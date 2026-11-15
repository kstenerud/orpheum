// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// deviceSettingsNow are the settings the core keeps for the device asking (D493): how far it moves
// its sound from its picture, and its data caps.
type deviceSettingsNow struct {
	soundOffsetMs int64
	dataCaps      auth.DataCaps
}

// deviceSettings are a person's device's settings; none when there is no device, as for a Watch
// Together viewer, or none the person has.
func (service *Service) deviceSettings(ctx context.Context, userID, deviceID int64) (deviceSettingsNow, error) {
	if deviceID == 0 {
		return deviceSettingsNow{}, nil
	}
	device, err := store.OrNotFound(service.database.Read().GetLiveDeviceOfUser(ctx, sqlcgen.GetLiveDeviceOfUserParams{
		ID: deviceID, UserID: &userID,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return deviceSettingsNow{}, nil
	}
	if err != nil {
		return deviceSettingsNow{}, fmt.Errorf("read device %d: %w", deviceID, err)
	}
	return deviceSettingsNow{
		soundOffsetMs: device.SoundOffsetMs,
		dataCaps:      auth.DataCapsOf(device.DataCapWifiKbps, device.DataCapMobileKbps, device.DataCapUnknownKbps),
	}, nil
}

// onDevice is what a start takes from the device it is for, read from the core's own record of it
// (D493): its sound offset, the data cap of the kind of network the start says, which chooses the
// delivery the request prefers, and whether it gave the film's own picture up (D266).
func (service *Service) onDevice(
	ctx context.Context, userID int64, request *Request, probe nodeproto.Probe, fileID int64,
) (Started, bool, error) {
	device, err := service.deviceSettings(ctx, userID, request.DeviceID)
	if err != nil {
		return Started{}, false, err
	}
	started := Started{
		SoundOffsetMs: device.soundOffsetMs, DataCapKbps: device.dataCaps.Of(request.NetworkKind),
		FileBitRateBps: probe.Format.BitRateBps,
	}
	request.PreferredDelivery = deliveryUnderCap(request.PreferredDelivery, started.DataCapKbps,
		probe.Format.BitRateBps/1000)
	isOwnPictureUnplayable, err := service.isOwnPictureUnplayable(ctx, *request, fileID)
	if err != nil {
		return Started{}, false, err
	}
	return started, isOwnPictureUnplayable, nil
}
