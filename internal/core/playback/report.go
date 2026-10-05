// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// EventKind is a change of play state (D165): what happened, not a heartbeat.
type EventKind string

// The kinds of change, as play_event's CHECK names them.
const (
	EventStarted   EventKind = "started"
	EventPaused    EventKind = "paused"
	EventSeeked    EventKind = "seeked"
	EventCompleted EventKind = "completed"
	EventAbandoned EventKind = "abandoned"
)

// Report is what a player tells the server about one playback (cut-a §5, D165): where it is,
// what changed since its last report, and the spans of watching those changes closed. Every time
// in it is the device's monotonic clock, in milliseconds, which the server anchors to its own
// when the report arrives.
type Report struct {
	// ReportSeq numbers the playback's reports from 1; one sent again is taken once.
	ReportSeq int64
	// SentMonoMs is the device's monotonic clock when it sent the report.
	SentMonoMs int64
	// PositionMs is where the playback is in the file, sampled at PositionReadMonoMs.
	PositionMs         int64
	PositionReadMonoMs int64
	Events             []Event
	Spans              []Span
	// Stalls are where the player stalled since its last report (D247 §2): a statement about its
	// link, not about what was watched, so they are not kept in the play log.
	Stalls []Stall
	// LinkKbps is the link's speed as the player measured it, zero when it has not (D247 §1).
	LinkKbps int64
}

// Stall is one stall of a player: a wait for data, while it played, where it was.
type Stall struct {
	PositionMs int64
}

// Answer is what a report is answered with: the admin's stop, and a lower quality to move to.
type Answer struct {
	// StopMessage is the admin's message when they stopped the playback, which then ends (D243 §9).
	StopMessage *string
	// LowerQuality is a quality to move to, once the player has stalled again after the stall that
	// started it (D247 §2); nil until then, and told once.
	LowerQuality *chunks.Offer
}

// Chunks offers a stalled playback a lower quality, and starts making it: chunks.Service.
type Chunks interface {
	Stalled(ctx context.Context, playback chunks.Playback, positionMs, linkKbps int64) (chunks.Offer, error)
}

// Event is one change of play state, at a position in the file.
type Event struct {
	Kind           EventKind
	PositionMs     int64
	HappenedMonoMs int64
}

// Span is a run of watching that a change closed: from one position to another, between two
// moments. An unattended span played after nobody answered "still watching?" (D188).
type Span struct {
	FromPositionMs int64
	ToPositionMs   int64
	StartedMonoMs  int64
	EndedMonoMs    int64
	IsUnattended   bool
}

// ErrBadReport is a report that cannot be true: a time after the report was sent, a position
// before the start.
var ErrBadReport = errors.New("the report cannot be true")

// serverTimeMs turns a device's monotonic reading into the server's time: the report was sent at
// sentMonoMs by the device's clock and arrived now by the server's (D165), so a reading is that
// far before now.
func serverTimeMs(nowMs, sentMonoMs, monoMs int64) (int64, error) {
	if monoMs > sentMonoMs || monoMs < 0 {
		return 0, fmt.Errorf("%w: a moment at %d ms is after the report was sent at %d ms",
			ErrBadReport, monoMs, sentMonoMs)
	}
	return nowMs - (sentMonoMs - monoMs), nil
}

// Report takes a player's report on the playback a media token refers to, from the device the
// session was signed in on (deviceID 0 for none). The events and spans are appended to the log;
// the position moves only when the watching it describes is later than the watching that set
// the stored one, so two devices reporting out of order never rewind it (D217 §7). A report sent
// again changes nothing. A playback the admin stopped is told so in the answer, its message
// returned, and ends: its token is forgotten once its report is kept (D243 §9). A Watch Together
// viewer's report keeps nothing in the log (D243 §2). Its stalls start a lower quality, and from the
// second the answer offers it (D247 §2).
func (service *Service) Report(
	ctx context.Context, token string, holder Holder, deviceID int64, report Report,
) (Answer, error) {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return Answer{}, err
	}
	now := service.clock.Now()
	playback, isNew, found := service.tokens.takeReport(token, holder, report.ReportSeq, now, idle)
	if !found {
		return Answer{}, ErrNoToken
	}
	if isNew {
		// A Watch Together viewer is nobody's: nothing it watches is recorded (D243 §2).
		if holder.UserID != 0 {
			keepErr := service.keepReport(ctx, playback, holder.UserID, deviceID, now.UnixMilli(), report)
			if keepErr != nil {
				service.tokens.returnReport(token, report.ReportSeq, playback.lastReportSeq)
				return Answer{}, keepErr
			}
		}
		service.tokens.keepPosition(token, report.PositionMs)
	}
	answer := Answer{StopMessage: playback.stopMessage}
	if playback.stopMessage != nil {
		service.tokens.end(token)
		return answer, nil
	}
	if isNew && len(report.Stalls) > 0 {
		offer, isOffered, err := service.takeStalls(ctx, token, playback, report)
		if err != nil {
			return Answer{}, err
		}
		if isOffered {
			answer.LowerQuality = &offer
		}
	}
	return answer, nil
}

// takeStalls counts a report's stalls on the playback (D247 §2): the first starts a lower quality
// where the player was, on the link it measured; from the second, the quality is offered, once, and
// isOffered says so. A film that cannot offer one, with no grid yet or nothing lower, is offered
// none; its first stall is asked about again at the next.
func (service *Service) takeStalls(
	ctx context.Context, token string, playback tokenPlayback, report Report,
) (offer chunks.Offer, isOffered bool, err error) {
	stalls := service.tokens.countStalls(token, len(report.Stalls))
	if service.chunks == nil {
		return chunks.Offer{}, false, nil
	}
	offer, isStarted := service.tokens.offerOf(token)
	if !isStarted {
		offer, err = service.chunks.Stalled(ctx, chunksPlaybackOf(playback), report.Stalls[0].PositionMs,
			report.LinkKbps)
		if errors.Is(err, chunks.ErrNoGrid) || errors.Is(err, chunks.ErrCannotBeCut) ||
			errors.Is(err, chunks.ErrNoQualityOffered) {
			service.tokens.forgetStalls(token)
			return chunks.Offer{}, false, nil
		}
		if err != nil {
			return chunks.Offer{}, false, err
		}
		service.tokens.keepOffer(token, offer)
	}
	isOffered = stalls >= stallsBeforeOffering && service.tokens.tellOffer(token)
	return offer, isOffered, nil
}

// stallsBeforeOffering is how many stalls a playback has before it is offered a lower quality: the
// first starts it, the second moves the player to it (D247 §2, the owner's design).
const stallsBeforeOffering = 2

// nowPlayingHeardWithin is how lately a playback must have been heard from, by a report or a byte
// request, to be playing now: three of the player's ten-second reports (D243 §9), so one lost
// report does not drop it from the list. Tied to the player's interval, not an opinion.
const nowPlayingHeardWithin = 30 * time.Second

// NowPlaying is every playback heard from lately, oldest first (cut-a §5).
func (service *Service) NowPlaying() []NowPlaying {
	return service.tokens.nowPlaying(service.clock.Now().Add(-nowPlayingHeardWithin))
}

// Stop stops a playback now playing, with a message its player shows: told in the answer to its
// next report, within a report interval (D243 §9). ErrNoToken when there is none.
func (service *Service) Stop(playbackID int64, message string) error {
	if !service.tokens.stop(playbackID, message) {
		return ErrNoToken
	}
	return nil
}

// keepReport writes a new report: its events and spans to the log, and the position through the guard.
func (service *Service) keepReport(
	ctx context.Context, playback tokenPlayback, userID, deviceID, nowMs int64, report Report,
) error {
	watchedAt, err := serverTimeMs(nowMs, report.SentMonoMs, report.PositionReadMonoMs)
	if err != nil {
		return err
	}
	if report.PositionMs < 0 {
		return fmt.Errorf("%w: a position before the start", ErrBadReport)
	}
	log := playLog{userID: userID, fileID: playback.fileID, nowMs: nowMs, sentMonoMs: report.SentMonoMs}
	if deviceID != 0 {
		log.deviceID = &deviceID
	}
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, event := range report.Events {
			if err := log.appendEvent(ctx, queries, event); err != nil {
				return err
			}
		}
		for _, span := range report.Spans {
			if err := log.appendSpan(ctx, queries, span); err != nil {
				return err
			}
		}
		err := queries.MovePlayPosition(ctx, sqlcgen.MovePlayPositionParams{
			UserID: userID, FileID: playback.fileID, PositionMs: report.PositionMs, ReportSeq: report.ReportSeq,
			DeviceID: log.deviceID, UpdatedAt: nowMs, PositionReadAt: watchedAt,
		})
		if err != nil {
			return err
		}
		// A reported playback has started its file, which plays again with what it used (D230 §2).
		return service.preferences.KeepUsed(ctx, queries, userID, playback.fileID, playback.used.subtitle,
			playback.used.audio)
	})
}

// playLog appends one report's events and spans, at the server's time of each.
type playLog struct {
	userID, fileID    int64
	deviceID          *int64
	nowMs, sentMonoMs int64
}

func (log playLog) appendEvent(ctx context.Context, queries *sqlcgen.Queries, event Event) error {
	happenedAt, err := serverTimeMs(log.nowMs, log.sentMonoMs, event.HappenedMonoMs)
	if err != nil {
		return err
	}
	mono := event.HappenedMonoMs
	err = queries.InsertPlayEvent(ctx, sqlcgen.InsertPlayEventParams{
		UserID: log.userID, FileID: log.fileID, DeviceID: log.deviceID, Kind: string(event.Kind),
		PositionMs: event.PositionMs, HappenedAt: happenedAt, HappenedMonoMs: &mono,
	})
	if err != nil {
		return fmt.Errorf("log a %s event: %w", event.Kind, err)
	}
	return nil
}

func (log playLog) appendSpan(ctx context.Context, queries *sqlcgen.Queries, span Span) error {
	started, err := serverTimeMs(log.nowMs, log.sentMonoMs, span.StartedMonoMs)
	if err != nil {
		return err
	}
	ended, err := serverTimeMs(log.nowMs, log.sentMonoMs, span.EndedMonoMs)
	if err != nil {
		return err
	}
	if ended < started {
		return fmt.Errorf("%w: a span that ends before it starts", ErrBadReport)
	}
	var unattended int64
	if span.IsUnattended {
		unattended = 1
	}
	err = queries.InsertPlaySpan(ctx, sqlcgen.InsertPlaySpanParams{
		UserID: log.userID, FileID: log.fileID, DeviceID: log.deviceID, FromPositionMs: span.FromPositionMs,
		ToPositionMs: span.ToPositionMs, StartedAt: started, EndedAt: ended, IsUnattended: unattended,
	})
	if err != nil {
		return fmt.Errorf("log a span: %w", err)
	}
	return nil
}
