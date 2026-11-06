// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// askedWithin is how lately a playback of chunks must have asked for a stream for it to need the
// stream's rate: a playing player asks every chunk, every few seconds, and a paused one with its
// buffer full asks nothing and needs nothing (as the chunks' inUseWithin; D286 §5,
// plans/upload-budget.md §1, §2).
const askedWithin = 30 * time.Second

// streamNeed is a stream's average rate, in kbit/s, as a playback last asked for it, and when.
type streamNeed struct {
	kbps int64
	at   time.Time
}

// Upload is what the server's players need of its upload, and what it sent them, over a span.
type Upload struct {
	// Playbacks is how many need anything: asked for chunks within askedWithin, or were sent bytes.
	Playbacks int
	// NeedKbps is what they need: each playback of chunks the rates of the picture and sound it last
	// asked for, and each other playback what it was sent over the span.
	NeedKbps int64
	// SentKbps is what the server sent all of them over the span.
	SentKbps int64
}

// Sent counts bytes of a film the server sent a token's playback.
func (service *Service) Sent(token string, bytes int64) {
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	if known, found := service.tokens.playbackByToken[token]; found {
		known.sentBytes += bytes
	}
}

// TakeUpload is what the server's players need now, and were sent over span, ending now; the bytes
// sent are counted afresh from now.
func (service *Service) TakeUpload(span time.Duration) Upload {
	now := service.clock.Now()
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	var upload Upload
	var sentBytes int64
	for _, playback := range service.tokens.playbackByToken {
		playback.measuredKbps = kbpsOver(playback.sentBytes, span)
		need := playback.picturesNeed.within(now) + playback.soundsNeed.within(now)
		if need == 0 {
			need = playback.measuredKbps
		}
		if need > 0 {
			upload.Playbacks++
			upload.NeedKbps += need
		}
		sentBytes += playback.sentBytes
		playback.sentBytes = 0
	}
	upload.SentKbps = kbpsOver(sentBytes, span)
	return upload
}

// within is the stream's rate if it was asked for within askedWithin of now, else zero.
func (need streamNeed) within(now time.Time) int64 {
	if need.at.IsZero() || now.Sub(need.at) > askedWithin {
		return 0
	}
	return need.kbps
}

// kbpsOver is bytes sent over span as a rate in kbit/s; zero over no span.
func kbpsOver(bytes int64, span time.Duration) int64 {
	if span <= 0 {
		return 0
	}
	return bytes * 8 / span.Milliseconds()
}

// WatchUpload logs, at each tick while anyone plays, what the server's players need of its upload
// beside what it sent them and the share of server_upload_mbps its films may use: whether the setting
// is right, and how near the server is to lowering its players' qualities (D286 §5), shows in the log
// before anything acts on it.
func (service *Service) WatchUpload(ctx context.Context, tick <-chan time.Time, logger *slog.Logger) {
	last := service.clock.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
		now := service.clock.Now()
		upload := service.TakeUpload(now.Sub(last))
		last = now
		if upload.Playbacks == 0 && upload.SentKbps == 0 {
			continue
		}
		uploadMbps, err := service.uploadMbps(ctx)
		if err != nil {
			logger.WarnContext(ctx, "could not read the server's upload", "err", err)
		}
		shareKbps := service.uploadTuningOf(ctx, logger).shareKbps(uploadMbps)
		logger.InfoContext(ctx, "the server's upload", slogx.EventKey, "uploadMeasured", "playbacks", upload.Playbacks,
			"needKbps", upload.NeedKbps, "sentKbps", upload.SentKbps, "uploadMbps", uploadMbps,
			"filmsShareKbps", shareKbps, "isOverShare", shareKbps > 0 && upload.NeedKbps > shareKbps)
	}
}

// uploadMbps is server_upload_mbps, zero while it is unanswered.
func (service *Service) uploadMbps(ctx context.Context) (int64, error) {
	mbps, err := service.settings.Int(ctx, settings.ServerUploadMbps)
	if errors.Is(err, settings.ErrUnanswered) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("the server's upload: %w", err)
	}
	return mbps, nil
}

// UploadFullReason is the reason a member sent away for the server's upload is told: the host's
// connection cannot carry everyone (D286 §5).
const UploadFullReason = "uploadFull"

// SendAway sends away the Watch Together member who joined most recently of those with a player,
// never a host, saying whom, and whether there was anyone: together.Hub.SendAwayNewest.
type SendAway func() (sessionID, memberID int64, isSent bool)

// SendAwayBy is the last resort when the upload cannot carry every player at its lowest quality
// (D286 §5). Set once, before the server serves.
func (service *Service) SendAwayBy(sendAway SendAway) { service.sendAway = sendAway }

// lastResort is when the need has been over the share with every quality at its lowest since, and
// when a member was last sent away for it.
type lastResort struct {
	overSince, sentAwayAt time.Time
}

// isDue is whether a member is to be sent away now: the need over the share at every lowest quality
// for the tuning's sendAwayAfter, and its sentAwaySettles since the last one sent away (D286 §5).
func (resort *lastResort) isDue(isOverAtLowest bool, now time.Time, tuning uploadTuning) bool {
	if !isOverAtLowest {
		resort.overSince = time.Time{}
		return false
	}
	if resort.overSince.IsZero() {
		resort.overSince = now
	}
	return now.Sub(resort.overSince) >= tuning.sendAwayAfter &&
		(resort.sentAwayAt.IsZero() || now.Sub(resort.sentAwayAt) >= tuning.sentAwaySettles())
}

// KeepUnderShare looks at each tick at what the server's players need of its upload, and lowers or
// raises their qualities to keep it under the films' share of server_upload_mbps (D286 §5), telling
// each player with its next chunk (CeilingOf). Unanswered, it lifts every ceiling. Each change is
// logged, and the need still over the share with every quality at its lowest, once each time it
// comes to be; staying so for upload_send_away_after_seconds, the newest member is sent away
// (SendAwayBy). Its tuning, and the playbacks' timing (playbackTiming), are read at each look.
func (service *Service) KeepUnderShare(ctx context.Context, tick <-chan time.Time, logger *slog.Logger) {
	control := &uploadControl{}
	crowded := &crowding{}
	resort := &lastResort{}
	wasOverAtLowest := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
		service.refreshTiming(ctx, logger)
		uploadMbps, err := service.uploadMbps(ctx)
		if err != nil {
			logger.WarnContext(ctx, "could not read the server's upload", "err", err)
			continue
		}
		tuning := service.uploadTuningOf(ctx, logger)
		shareKbps := service.crowdedShare(ctx, crowded, tuning.shareKbps(uploadMbps), tuning, logger)
		changes, isOverAtLowest := service.lookAtUpload(control, shareKbps, tuning)
		for _, change := range changes {
			logger.InfoContext(ctx, "a playback's quality was kept within the server's upload",
				append(change.logAttributes(), slogx.EventKey, "qualityKeptWithinUpload", "shareKbps", shareKbps)...)
		}
		if isOverAtLowest && !wasOverAtLowest {
			logger.WarnContext(ctx, "the server's upload cannot carry its players at their lowest qualities",
				"shareKbps", shareKbps)
		}
		wasOverAtLowest = isOverAtLowest
		if resort.isDue(isOverAtLowest, service.clock.Now(), tuning) && service.sendAway != nil {
			if sessionID, memberID, isSent := service.sendAway(); isSent {
				resort.sentAwayAt = service.clock.Now()
				logger.WarnContext(ctx, "sent away the newest member: the server's upload cannot carry everyone",
					"sessionId", sessionID, "memberId", memberID, "shareKbps", shareKbps)
			}
		}
	}
}

// crowdedShare is the films' share at this look, settingShareKbps lowered while players say the
// house's upload is crowded and grown back once they stop, kept for the starts; each change logged.
func (service *Service) crowdedShare(
	ctx context.Context, crowded *crowding, settingShareKbps int64, tuning uploadTuning, logger *slog.Logger,
) int64 {
	if settingShareKbps == 0 {
		service.crowdedShareKbps.Store(0)
		return 0
	}
	now := service.clock.Now()
	isCrowded := service.isCrowded(settingShareKbps, now, tuning)
	share, isChanged := crowded.look(settingShareKbps, isCrowded, now, tuning)
	service.crowdedShareKbps.Store(crowded.shareKbps)
	if isChanged {
		logger.InfoContext(ctx, "the films' share of the server's upload changed with the house's other traffic",
			"shareKbps", share, "settingShareKbps", settingShareKbps, "isCrowded", isCrowded)
	}
	return share
}

// ceilingChange is a playback whose ceiling a look changed.
type ceilingChange struct {
	playbackID, fileID, fromKbps, toKbps, needKbps int64
}

func (change ceilingChange) logAttributes() []any {
	return []any{
		"playbackId", change.playbackID, "fileId", change.fileID, "fromCeilingKbps", change.fromKbps,
		"toCeilingKbps", change.toKbps, "needKbps", change.needKbps,
	}
}

// lookAtUpload is one look (uploadControl.look) at the playbacks now, under shareKbps, its ceilings
// kept: every ceiling lifted with no share. It says each change, and whether the need is over the
// share with every quality at its lowest.
func (service *Service) lookAtUpload(
	control *uploadControl, shareKbps int64, tuning uploadTuning,
) ([]ceilingChange, bool) {
	now := service.clock.Now()
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	var playbacks []lowerable
	var otherKbps int64
	for token, playback := range service.tokens.playbackByToken {
		loads := slices.IndexFunc(playback.ladder, func(rung chunks.Rung) bool {
			return rung.StreamName == playback.pictureStream
		})
		if shareKbps == 0 || loads < 0 || playback.picturesNeed.within(now) == 0 {
			if need := playback.picturesNeed.within(now) + playback.soundsNeed.within(now); need > 0 {
				otherKbps += need
			} else {
				otherKbps += playback.measuredKbps
			}
			continue
		}
		ceiling := slices.IndexFunc(playback.ladder, func(rung chunks.Rung) bool {
			return rung.AverageKbps == playback.ceilingKbps
		})
		if ceiling < 0 {
			ceiling = len(playback.ladder) - 1
		}
		playbacks = append(playbacks, lowerable{
			token: token, fileID: playback.fileID, startedAt: playback.startedAt, loweredAt: playback.loweredAt,
			rungs: playback.ladder, loads: loads, ceiling: ceiling,
		})
	}
	if shareKbps == 0 {
		return service.liftCeilingsLocked(), false
	}
	before := make([]int64, len(playbacks))
	for index, playback := range playbacks {
		before[index] = playback.ceilingKbps()
	}
	_, isOverAtLowest := control.look(playbacks, otherKbps, shareKbps, now, tuning)
	var changes []ceilingChange
	for index, playback := range playbacks {
		if playback.ceilingKbps() == before[index] {
			continue
		}
		known := service.tokens.playbackByToken[playback.token]
		known.ceilingKbps, known.loweredAt = playback.ceilingKbps(), playback.loweredAt
		changes = append(changes, ceilingChange{
			playbackID: known.playbackID, fileID: known.fileID, fromKbps: before[index],
			toKbps: known.ceilingKbps, needKbps: playback.needKbps(),
		})
	}
	return changes, isOverAtLowest
}

// liftCeilingsLocked lifts every playback's ceiling, saying each it lifted; tokens.mutex is held.
func (service *Service) liftCeilingsLocked() []ceilingChange {
	var changes []ceilingChange
	for _, playback := range service.tokens.playbackByToken {
		if playback.ceilingKbps != 0 {
			changes = append(changes, ceilingChange{
				playbackID: playback.playbackID, fileID: playback.fileID, fromKbps: playback.ceilingKbps,
			})
			playback.ceilingKbps = 0
		}
	}
	return changes
}

// GroupPlayers is how many devices play in the Watch Together session a user hosts, zero for none:
// together.Hub.PlayersHostedBy.
type GroupPlayers func(hostUserID int64) int

// GroupPlayersBy is how a start in a group learns how many players share the upload with it (D286
// §5). Set once, before the server serves.
func (service *Service) GroupPlayersBy(players GroupPlayers) { service.groupPlayers = players }

// startAtMostKbps is the most a playback of fileID starting now for holder may take of the films'
// share of server_upload_mbps (D286 §5): what is left of it, and in a Watch Together group of more
// than one player, at most an even part of what the playbacks of other films leave, so every player
// a film put on starts for at once starts within its part, whichever asks first; at least 1 kbit/s, so
// a start past it takes the lowest quality; zero, no bound, while the setting is unanswered.
func (service *Service) startAtMostKbps(ctx context.Context, holder Holder, fileID int64) (int64, error) {
	uploadMbps, err := service.uploadMbps(ctx)
	if err != nil || uploadMbps == 0 {
		return 0, err
	}
	sharePercent, err := service.settings.Int(ctx, settings.UploadFilmsSharePercent)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", settings.UploadFilmsSharePercent, err)
	}
	players := 0
	if service.groupPlayers != nil {
		players = service.groupPlayers(cmp.Or(holder.HostUserID, holder.UserID))
	}
	now := service.clock.Now()
	service.tokens.mutex.Lock()
	defer service.tokens.mutex.Unlock()
	share := service.filmsShareKbps(uploadTuning{filmsSharePercent: sharePercent}.shareKbps(uploadMbps))
	left, othersLeave := share, share
	for _, playback := range service.tokens.playbackByToken {
		need := playback.picturesNeed.within(now) + playback.soundsNeed.within(now)
		if need == 0 {
			need = playback.measuredKbps
		}
		left -= need
		if playback.fileID != fileID {
			othersLeave -= need
		}
	}
	if players > 1 {
		left = min(left, othersLeave/int64(players))
	}
	return max(left, 1), nil
}

// keepStartsNeed counts a playback starting on its ladder at offer as needing its rate from now,
// before its player asks for anything, so the starts of a group's players, a moment apart, each see
// the others' (the owner's test, 2026-10-12: every device started at 1.7 to 1.8 Mbit/s, together
// past the share); and, where the start was held lower to keep within the upload, at that ceiling.
// A playback not on its ladder, a nil offer, is counted by what it is sent.
func (service *Service) keepStartsNeed(playback *tokenPlayback, offer *chunks.Offer) {
	if offer == nil {
		return
	}
	now := service.clock.Now()
	playback.picturesNeed, playback.pictureStream = streamNeed{kbps: offer.AverageKbps, at: now}, offer.StreamName
	if offer.IsHeldByUpload {
		playback.ceilingKbps, playback.loweredAt = offer.AverageKbps, now
	}
}
