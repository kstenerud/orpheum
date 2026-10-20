// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"time"
)

// How the server tells that other traffic in the house, a video call say, crowds the upload its
// films share, from the link speeds its players report, and how far it then lowers the films' share:
// uploadTuning's settings (plans/upload-budget.md §4c; the owner, 2026-10-12: "likely the clients
// could just as quickly report back about how quickly they're receiving the bytes").

// linkReportedWithin is how lately a player must have reported its link for the report to count:
// three of its ten-second reports.
const linkReportedWithin = 30 * time.Second

// linkReport is the link speed a playback's player last reported, and when.
type linkReport struct {
	kbps int64
	at   time.Time
}

// isSlowFor is whether the report, fresh at now, says a link too slow for a need of needKbps: under
// slowUnderPercent of it.
func (report linkReport) isSlowFor(needKbps int64, now time.Time, slowUnderPercent int64) bool {
	return !report.at.IsZero() && now.Sub(report.at) <= linkReportedWithin && needKbps > 0 &&
		report.kbps*100 < needKbps*slowUnderPercent
}

// crowding is the films' share as other traffic in the house leaves it: lowered while players say
// the house's upload is crowded, and grown back once they stop.
type crowding struct {
	// shareKbps is the films' share now, zero while it is the setting's whole share.
	shareKbps                    int64
	lastCrowdedAt, lastLoweredAt time.Time
	lastRaisedAt                 time.Time
}

// look is the films' share at now: settingShareKbps, the setting's, lowered by the tuning's
// crowdedKeepsPercent at most every crowdedLowerEvery while isCrowded, down to crowdedFloorPercent of
// it, and grown back by uncrowdedGrowsPercent every uncrowdedRaiseEvery once uncrowdedFor has passed
// without a sign of it. It says whether it changed.
func (crowded *crowding) look(
	settingShareKbps int64, isCrowded bool, now time.Time, tuning uploadTuning,
) (int64, bool) {
	current := crowded.shareKbps
	if current == 0 || current > settingShareKbps {
		current = settingShareKbps
	}
	if isCrowded {
		crowded.lastCrowdedAt = now
		if !crowded.lastLoweredAt.IsZero() && now.Sub(crowded.lastLoweredAt) < tuning.crowdedLowerEvery {
			return current, false
		}
		lowered := max(current*tuning.crowdedKeepsPercent/100, settingShareKbps*tuning.crowdedFloorPercent/100)
		crowded.lastLoweredAt = now
		return crowded.keep(lowered, current, settingShareKbps)
	}
	isQuiet := crowded.lastCrowdedAt.IsZero() || now.Sub(crowded.lastCrowdedAt) >= tuning.uncrowdedFor
	isDue := crowded.lastRaisedAt.IsZero() || now.Sub(crowded.lastRaisedAt) >= tuning.uncrowdedRaiseEvery
	if current == settingShareKbps || !isQuiet || !isDue {
		return current, false
	}
	crowded.lastRaisedAt = now
	grown := current * (100 + tuning.uncrowdedGrowsPercent) / 100
	return crowded.keep(min(settingShareKbps, grown), current, settingShareKbps)
}

// keep keeps share as the films' share, zero when it is the setting's whole, and says it and whether
// it differs from current.
func (crowded *crowding) keep(share, current, settingShareKbps int64) (int64, bool) {
	crowded.shareKbps = share
	if share >= settingShareKbps {
		crowded.shareKbps = 0
	}
	return share, share != current
}

// keepLink keeps the link speed a token's player reported at now; a report with none keeps nothing.
func (service *Service) keepLink(token string, kbps int64, now time.Time) {
	if kbps <= 0 {
		return
	}
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	if known, found := service.tokens.playbackByToken[token]; found {
		known.link = linkReport{kbps: kbps, at: now}
	}
}

// isCrowded is whether the house's upload looks crowded at now: the tuning's slowPlayersAtLeast
// players report links too slow for what they need while all of them need no more than
// settingShareKbps, so something other than the films fills it.
func (service *Service) isCrowded(settingShareKbps int64, now time.Time, tuning uploadTuning) bool {
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	slow := 0
	var total int64
	for _, playback := range service.tokens.playbackByToken {
		need := playback.picturesNeed.within(now) + playback.soundsNeed.within(now)
		total += need
		if playback.link.isSlowFor(need, now, tuning.slowUnderPercent) {
			slow++
		}
	}
	return slow >= tuning.slowPlayersAtLeast && total <= settingShareKbps
}

// filmsShareKbps is the films' share now: the setting's, settingShareKbps, or less while the house's
// upload is crowded.
func (service *Service) filmsShareKbps(settingShareKbps int64) int64 {
	if crowded := service.crowdedShareKbps.Load(); crowded > 0 && crowded < settingShareKbps {
		return crowded
	}
	return settingShareKbps
}
