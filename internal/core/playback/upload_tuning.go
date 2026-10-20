// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// uploadTuning is how the server keeps its films within its upload, read from its settings at each
// look (the upload_* settings: D286 §4, §5; plans/upload-budget.md §4c; the constants sweep,
// 2026-10-20). One controller: its values are tuned together.
type uploadTuning struct {
	// filmsSharePercent is how much of server_upload_mbps the films may use.
	filmsSharePercent int64
	// A player is slow when its link carries less than slowUnderPercent of what its quality needs, and
	// the house's upload crowded when slowPlayersAtLeast are slow at once.
	slowUnderPercent   int64
	slowPlayersAtLeast int
	// Each lowering for a crowded upload keeps crowdedKeepsPercent of the films' share, at most every
	// crowdedLowerEvery, down to crowdedFloorPercent of the whole share; uncrowdedFor without a sign of
	// crowding, it grows uncrowdedGrowsPercent every uncrowdedRaiseEvery.
	crowdedKeepsPercent   int64
	crowdedLowerEvery     time.Duration
	crowdedFloorPercent   int64
	uncrowdedFor          time.Duration
	uncrowdedGrowsPercent int64
	uncrowdedRaiseEvery   time.Duration
	// A lowered playback is raised a quality once the need has stayed under raiseUnderPercent of the
	// films' share for raiseRoom.
	raiseUnderPercent int64
	raiseRoom         time.Duration
	// sendAwayAfter is how long the need stays over the share with every quality at its lowest before
	// the newest Watch Together member is sent away.
	sendAwayAfter time.Duration
}

// sentAwaySettles is how long after one member is sent away before another may be: what the one sent
// away asked for counts for askedWithin after (D286 §5).
func (tuning uploadTuning) sentAwaySettles() time.Duration { return askedWithin + tuning.sendAwayAfter }

// shareKbps is the films' share of an upload of uploadMbps, in kbit/s.
func (tuning uploadTuning) shareKbps(uploadMbps int64) int64 {
	return uploadMbps * 1000 * tuning.filmsSharePercent / 100
}

// startingUploadTuning is every value at its setting's starting value.
func startingUploadTuning() uploadTuning {
	return uploadTuning{
		filmsSharePercent:     settings.StartingInt(settings.UploadFilmsSharePercent),
		slowUnderPercent:      settings.StartingInt(settings.UploadCrowdedSlowUnderPercent),
		slowPlayersAtLeast:    int(settings.StartingInt(settings.UploadCrowdedSlowPlayersAtLeast)),
		crowdedKeepsPercent:   settings.StartingInt(settings.UploadCrowdedKeepsPercent),
		crowdedLowerEvery:     settings.StartingDuration(settings.UploadCrowdedLowerEverySeconds),
		crowdedFloorPercent:   settings.StartingInt(settings.UploadCrowdedFloorPercent),
		uncrowdedFor:          settings.StartingDuration(settings.UploadUncrowdedForSeconds),
		uncrowdedGrowsPercent: settings.StartingInt(settings.UploadUncrowdedGrowsPercent),
		uncrowdedRaiseEvery:   settings.StartingDuration(settings.UploadUncrowdedRaiseEverySeconds),
		raiseUnderPercent:     settings.StartingInt(settings.UploadRaiseUnderPercent),
		raiseRoom:             settings.StartingDuration(settings.UploadRaiseRoomSeconds),
		sendAwayAfter:         settings.StartingDuration(settings.UploadSendAwayAfterSeconds),
	}
}

// uploadTuningOf reads the tuning; a value that cannot be read is its starting value, said in the log.
func (service *Service) uploadTuningOf(ctx context.Context, logger *slog.Logger) uploadTuning {
	tuning := startingUploadTuning()
	integer := func(key settings.Key, into *int64) {
		if value, err := service.settings.Int(ctx, key); err == nil {
			*into = value
		} else {
			logger.WarnContext(ctx, "could not read a setting; going on at its starting value", "key", string(key),
				"err", err)
		}
	}
	length := func(key settings.Key, into *time.Duration) {
		if value, err := service.settings.Duration(ctx, key); err == nil {
			*into = value
		} else {
			logger.WarnContext(ctx, "could not read a setting; going on at its starting value", "key", string(key),
				"err", err)
		}
	}
	slowPlayers := int64(tuning.slowPlayersAtLeast)
	integer(settings.UploadFilmsSharePercent, &tuning.filmsSharePercent)
	integer(settings.UploadCrowdedSlowUnderPercent, &tuning.slowUnderPercent)
	integer(settings.UploadCrowdedSlowPlayersAtLeast, &slowPlayers)
	integer(settings.UploadCrowdedKeepsPercent, &tuning.crowdedKeepsPercent)
	length(settings.UploadCrowdedLowerEverySeconds, &tuning.crowdedLowerEvery)
	integer(settings.UploadCrowdedFloorPercent, &tuning.crowdedFloorPercent)
	length(settings.UploadUncrowdedForSeconds, &tuning.uncrowdedFor)
	integer(settings.UploadUncrowdedGrowsPercent, &tuning.uncrowdedGrowsPercent)
	length(settings.UploadUncrowdedRaiseEverySeconds, &tuning.uncrowdedRaiseEvery)
	integer(settings.UploadRaiseUnderPercent, &tuning.raiseUnderPercent)
	length(settings.UploadRaiseRoomSeconds, &tuning.raiseRoom)
	length(settings.UploadSendAwayAfterSeconds, &tuning.sendAwayAfter)
	tuning.slowPlayersAtLeast = int(slowPlayers)
	return tuning
}
