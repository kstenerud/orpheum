// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"

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
// again changes nothing.
func (service *Service) Report(
	ctx context.Context, token string, userID int64, sessionKey string, deviceID int64, report Report,
) error {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return err
	}
	now := service.clock.Now()
	playback, isNew, found := service.tokens.takeReport(token, userID, sessionKey, report.ReportSeq, now, idle)
	if !found {
		return ErrNoToken
	}
	if !isNew {
		return nil
	}
	if err := service.keepReport(ctx, playback, userID, deviceID, now.UnixMilli(), report); err != nil {
		service.tokens.returnReport(token, report.ReportSeq, playback.lastReportSeq)
		return err
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
		return queries.MovePlayPosition(ctx, sqlcgen.MovePlayPositionParams{
			UserID: userID, FileID: playback.fileID, PositionMs: report.PositionMs, ReportSeq: report.ReportSeq,
			DeviceID: log.deviceID, UpdatedAt: nowMs, PositionReadAt: watchedAt,
		})
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
