// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The films whose own picture a device's player gave up as one it cannot play (D266) are kept by
// the core, so that its later playbacks of the film are not offered it: Firefox on a Mac gave up
// Flow's own picture anew at every playback, 1.5 to 5 s each (the owner's test, 2026-10-12). They
// are kept in the store by device (own_picture_given_up), so a restart of the core keeps them too
// (D493), until the device or the file goes. A session with no device, a Watch Together guest's,
// has no row to keep them by: its start's own word counts for that start alone.

// keepOwnPictureGivenUp keeps that deviceID's player gave fileID's own picture up; nothing for a
// session with no device, deviceID 0.
func (service *Service) keepOwnPictureGivenUp(ctx context.Context, deviceID, fileID int64) error {
	if deviceID == 0 {
		return nil
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.KeepOwnPictureGivenUp(ctx, sqlcgen.KeepOwnPictureGivenUpParams{
			DeviceID: deviceID, FileID: fileID, GivenUpAt: service.clock.Now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("keep that device %d gave file %d's own picture up: %w", deviceID, fileID, err)
	}
	return nil
}

// isOwnPictureUnplayable is whether the asking device's player gave fileID's own picture up as
// unplayable on it (D266): on an earlier playback, as its report said, or as this request says,
// which is kept first, so a page asking again at once is offered the twin (D323 §2).
func (service *Service) isOwnPictureUnplayable(ctx context.Context, request Request, fileID int64) (bool, error) {
	if request.HasGivenUpOwnPicture {
		return true, service.keepOwnPictureGivenUp(ctx, request.DeviceID, fileID)
	}
	if request.DeviceID == 0 {
		return false, nil
	}
	isGivenUp, err := service.database.Read().IsOwnPictureGivenUp(ctx, sqlcgen.IsOwnPictureGivenUpParams{
		DeviceID: request.DeviceID, FileID: fileID,
	})
	if err != nil {
		return false, fmt.Errorf("read whether device %d gave file %d's own picture up: %w",
			request.DeviceID, fileID, err)
	}
	return isGivenUp == 1, nil
}
