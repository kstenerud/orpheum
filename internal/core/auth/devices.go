// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrNoDevice is a device that is not there, or not the person's.
var ErrNoDevice = errors.New("no such device")

// The ways a device makes sound, as a person answers (D222): its own speakers, a soundbar or
// receiver, or headphones.
const (
	AudioOutputBuiltInSpeakers = "built_in_speakers"
	AudioOutputReceiver        = "receiver"
	AudioOutputHeadphones      = "headphones"
)

// AudioOutputs are every way a device makes sound: the one list, which the API documents.
func AudioOutputs() []string {
	return []string{AudioOutputBuiltInSpeakers, AudioOutputReceiver, AudioOutputHeadphones}
}

// Device is one of a person's devices, as their list shows it (D218 §4).
type Device struct {
	ID       int64
	Name     string
	Platform string
	// AudioOutput is how it makes sound (D222): built_in_speakers, receiver or headphones, or empty
	// until it is asked, the first time it plays.
	AudioOutput string
	AddedAt     time.Time
	LastUsedAt  time.Time
	// IsSignedIn is whether any of its sessions is still live.
	IsSignedIn bool
}

// Devices lists a person's devices, most recently used first.
func (service *Service) Devices(ctx context.Context, userID int64) ([]Device, error) {
	rows, err := service.database.Read().ListLiveDevicesOfUser(ctx, sqlcgen.ListLiveDevicesOfUserParams{
		UserID: &userID, Now: new(service.clock.Now().UnixMilli()),
	})
	if err != nil {
		return nil, fmt.Errorf("list the devices of user %d: %w", userID, err)
	}
	devices := make([]Device, 0, len(rows))
	for _, row := range rows {
		lastUsedAt := row.LastUsedAt
		if remembered, ok := service.uses.latestOfDevice(row.ID); ok && remembered > lastUsedAt {
			lastUsedAt = remembered // a use not yet written (FlushSessionUses)
		}
		devices = append(devices, Device{
			ID: row.ID, Name: row.Name, Platform: row.Platform, AudioOutput: ptr.ValueOrZero(row.AudioOutput),
			AddedAt:    time.UnixMilli(row.AddedAt),
			LastUsedAt: time.UnixMilli(lastUsedAt), IsSignedIn: row.IsSignedIn == 1,
		})
	}
	slices.SortStableFunc(devices, func(a, b Device) int {
		return cmp.Or(b.LastUsedAt.Compare(a.LastUsedAt), cmp.Compare(b.ID, a.ID))
	})
	return devices, nil
}

// RenameDevice renames one of a person's devices.
func (service *Service) RenameDevice(ctx context.Context, userID, deviceID int64, name string) error {
	return service.onOwnDevice(ctx, userID, deviceID, func(queries *sqlcgen.Queries) error {
		return queries.RenameDevice(ctx, sqlcgen.RenameDeviceParams{Name: textindex.Line(name), ID: deviceID})
	})
}

// SetAudioOutput keeps how one of a person's devices makes sound (D222).
func (service *Service) SetAudioOutput(ctx context.Context, userID, deviceID int64, output string) error {
	return service.onOwnDevice(ctx, userID, deviceID, func(queries *sqlcgen.Queries) error {
		return queries.SetDeviceAudioOutput(ctx, sqlcgen.SetDeviceAudioOutputParams{AudioOutput: &output, ID: deviceID})
	})
}

// RemoveDevice revokes one of a person's devices (D218 §4): it leaves their list, and its
// sessions end at their next request.
func (service *Service) RemoveDevice(ctx context.Context, userID, deviceID int64) error {
	now := service.clock.Now().UnixMilli()
	return service.onOwnDevice(ctx, userID, deviceID, func(queries *sqlcgen.Queries) error {
		if err := queries.RevokeCredentialsOfDevice(ctx, sqlcgen.RevokeCredentialsOfDeviceParams{
			RevokedAt: &now, DeviceID: &deviceID,
		}); err != nil {
			return fmt.Errorf("end the device's sessions: %w", err)
		}
		return queries.RemoveDevice(ctx, sqlcgen.RemoveDeviceParams{RemovedAt: &now, ID: deviceID})
	})
}

// SignOutElsewhere ends every session of the person's but keepToken's, the one asking: the "sign out
// everywhere" D218 §5 keeps separate from a password reset. The devices stay listed. It kept every
// session of the asking device, so a session that had resumed this device stayed (the second
// audit's K1).
func (service *Service) SignOutElsewhere(ctx context.Context, userID int64, keepToken string) error {
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.RevokeSessionsOfUserExceptSession(ctx, sqlcgen.RevokeSessionsOfUserExceptSessionParams{
			RevokedAt: new(service.clock.Now().UnixMilli()), UserID: &userID, KeepSecretSha256: tokenHash(keepToken),
		})
	})
	if err != nil {
		return fmt.Errorf("sign user %d out elsewhere: %w", userID, err)
	}
	return nil
}

// onOwnDevice runs change on a device of the person's, or answers ErrNoDevice.
func (service *Service) onOwnDevice(
	ctx context.Context, userID, deviceID int64, change func(*sqlcgen.Queries) error,
) error {
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if _, err := liveDevice(ctx, queries, userID, deviceID); err != nil {
			return err
		}
		return change(queries)
	})
}

// liveDevice is one of a person's devices that has not been removed, or ErrNoDevice: a device
// that is someone else's is, to them, no device at all.
func liveDevice(
	ctx context.Context, queries *sqlcgen.Queries, userID, deviceID int64,
) (sqlcgen.GetLiveDeviceOfUserRow, error) {
	known, err := store.OrNotFound(queries.GetLiveDeviceOfUser(ctx, sqlcgen.GetLiveDeviceOfUserParams{
		ID: deviceID, UserID: &userID,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return known, ErrNoDevice
	}
	if err != nil {
		return known, fmt.Errorf("look up device %d: %w", deviceID, err)
	}
	return known, nil
}

// DeviceOf is one of a person's devices: the one a session is on, for showing who is signed in
// where.
func (service *Service) DeviceOf(ctx context.Context, userID, deviceID int64) (Device, error) {
	known, err := liveDevice(ctx, service.database.Read(), userID, deviceID)
	if err != nil {
		return Device{}, err
	}
	return Device{
		ID: known.ID, Name: known.Name, Platform: known.Platform, AudioOutput: ptr.ValueOrZero(known.AudioOutput),
		IsSignedIn: true,
	}, nil
}
