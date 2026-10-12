// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// What the server's upload is counted by (D286 §5, plans/upload-budget.md §1, §2).
const (
	// How much of server_upload_mbps its films may use: the rest is for voice, playlists, pictures
	// and the bursts a player loading ahead sends above its quality's average.
	filmsShareOfUploadPercent = 85
	// How lately a playback of chunks must have asked for a stream for it to need the stream's rate:
	// a playing player asks every chunk, every few seconds, and a paused one with its buffer full
	// asks nothing and needs nothing (as the chunks' inUseWithin).
	askedWithin = 30 * time.Second
)

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
		need := playback.picturesNeed.within(now) + playback.soundsNeed.within(now)
		if need == 0 {
			need = kbpsOver(playback.sentBytes, span)
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
		shareKbps := uploadMbps * 1000 * filmsShareOfUploadPercent / 100
		logger.InfoContext(ctx, "the server's upload", "playbacks", upload.Playbacks,
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
		return 0, fmt.Errorf("read %s: %w", settings.ServerUploadMbps, err)
	}
	return mbps, nil
}
