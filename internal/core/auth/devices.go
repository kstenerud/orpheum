// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ErrNoDevice is a device that is not there, or not the person's.
var ErrNoDevice = errors.New("no such device")

// Device is one of a person's devices, as their list shows it (D218 §4).
type Device struct {
	ID       int64
	Name     string
	Platform string
	AddedAt  time.Time
	LastUsed time.Time
	// IsSignedIn is whether any of its sessions is still live.
	IsSignedIn bool
}

// Devices lists a person's devices, most recently used first.
func (service *Service) Devices(ctx context.Context, userID int64) ([]Device, error) {
	rows, err := service.database.Read().ListLiveDevicesOf(ctx, sqlcgen.ListLiveDevicesOfParams{
		UserID: &userID, Now: new(service.clock.Now().UnixMilli()),
	})
	if err != nil {
		return nil, fmt.Errorf("list the devices of user %d: %w", userID, err)
	}
	devices := make([]Device, 0, len(rows))
	for _, row := range rows {
		devices = append(devices, Device{
			ID: row.ID, Name: row.Name, Platform: row.Platform, AddedAt: time.UnixMilli(row.FirstSeenAt),
			LastUsed: time.UnixMilli(row.LastUsedAt), IsSignedIn: row.IsSignedIn == 1,
		})
	}
	return devices, nil
}

// RenameDevice renames one of a person's devices.
func (service *Service) RenameDevice(ctx context.Context, userID, deviceID int64, name string) error {
	return service.onOwnDevice(ctx, userID, deviceID, func(queries *sqlcgen.Queries) error {
		return queries.RenameDevice(ctx, sqlcgen.RenameDeviceParams{Name: name, ID: deviceID})
	})
}

// RemoveDevice revokes one of a person's devices (D218 §4): it leaves their list, and its
// sessions end at their next request.
func (service *Service) RemoveDevice(ctx context.Context, userID, deviceID int64) error {
	now := service.clock.Now().UnixMilli()
	return service.onOwnDevice(ctx, userID, deviceID, func(queries *sqlcgen.Queries) error {
		if err := queries.RevokeSessionsOfDevice(ctx, sqlcgen.RevokeSessionsOfDeviceParams{
			RevokedAt: &now, DeviceID: &deviceID,
		}); err != nil {
			return fmt.Errorf("end the device's sessions: %w", err)
		}
		return queries.RemoveDevice(ctx, sqlcgen.RemoveDeviceParams{RemovedAt: &now, ID: deviceID})
	})
}

// SignOutElsewhere ends every session of the person's but those of the device keepDeviceID: the
// "sign out everywhere" D218 §5 keeps separate from a password reset. The devices stay listed.
func (service *Service) SignOutElsewhere(ctx context.Context, userID, keepDeviceID int64) error {
	var keep *int64
	if keepDeviceID != 0 {
		keep = &keepDeviceID
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.RevokeSessionsOfUserExceptDevice(ctx, sqlcgen.RevokeSessionsOfUserExceptDeviceParams{
			RevokedAt: new(service.clock.Now().UnixMilli()), UserID: &userID, KeepDeviceID: keep,
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
		owner, err := store.One(queries.GetLiveDeviceOwner(ctx, deviceID))
		if errors.Is(err, store.ErrNotFound) || (err == nil && (owner == nil || *owner != userID)) {
			return ErrNoDevice
		}
		if err != nil {
			return fmt.Errorf("read device %d: %w", deviceID, err)
		}
		return change(queries)
	})
}

// DeviceOf is one of a person's devices: the one a session is on, for showing who is signed in
// where.
func (service *Service) DeviceOf(ctx context.Context, userID, deviceID int64) (Device, error) {
	known, err := store.One(service.database.Read().GetLiveDeviceOf(ctx, sqlcgen.GetLiveDeviceOfParams{
		ID: deviceID, UserID: &userID,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return Device{}, ErrNoDevice
	}
	if err != nil {
		return Device{}, fmt.Errorf("read device %d: %w", deviceID, err)
	}
	return Device{ID: known.ID, Name: known.Name, Platform: known.Platform, IsSignedIn: true}, nil
}
