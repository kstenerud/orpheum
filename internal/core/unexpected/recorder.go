// SPDX-License-Identifier: AGPL-3.0-or-later

package unexpected

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Failure is one failure of unknown cause, with everything it said (D511 §2).
type Failure struct {
	// Kind is the kind of work that failed, as its owner names it: a job's kind ("fetch_metadata"),
	// or the work of a site that is not a job.
	Kind string
	// Subject is what the work was for, as its kind names it: a job's target ("item 12"), a file.
	Subject string
	// Error is the error, whole.
	Error string
	// Output is what the work said besides its error, a tool's standard error among it, whole; ""
	// when it said none.
	Output string
}

// Retry tries once more the failures an earlier start left of its kind, by what each was for: most
// clear the mark the kind keeps its failures by, so the ordinary sweep queues the work again. It is
// called once a start, from RetryEarlierStarts, before the runners start.
type Retry func(ctx context.Context, subjects []string) error

// Settings are what the recorder reads of the server's settings.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// Parts are what a Recorder is made of.
type Parts struct {
	Database *store.Database
	Settings Settings
	Clock    clock.Clock
	Logger   *slog.Logger
	// Version is the server's version, kept with each failure (D511 §2).
	Version string
	// StartID names this start of the server, at random: a failure kept under another was left by an
	// earlier start, and is tried once more at this one (D511 §4).
	StartID string
}

// Recorder keeps the failures of unknown cause, says what this start holds unavailable for one,
// counts them per kind, and says when a kind has had too many in an hour (D509, D511). Safe for
// concurrent use.
type Recorder struct {
	database *store.Database
	settings Settings
	clock    clock.Clock
	logger   *slog.Logger
	version  string
	startID  string

	mutex sync.Mutex
	// kept are the failures persistent.db holds, by kind and subject: a success of one not kept
	// writes nothing. Guarded by mutex.
	kept map[key]bool
	// unavailable are the failures recorded at this start, not tried again until the next (D511 §3).
	// Guarded by mutex.
	unavailable map[key]bool
	// earlier are the failures an earlier start left, by kind, for RetryEarlierStarts. Guarded by
	// mutex; nil once they were tried.
	earlier map[string][]string
	// retries are each kind's Retry, by kind. Guarded by mutex.
	retries map[string]Retry
	// counts are each kind's counts since this start (D509 §2). Guarded by mutex.
	counts map[string]*kindCount
}

type key struct{ kind, subject string }

// minutesKept is how many minutes of counts a kind keeps: an hour's window ending in each minute of
// the last hour, which is how long a kind stays raised after its last minute over (D509 §2).
const minutesKept = 120

// kindCount is one kind's failures since this start: in all, the last one's time, and how many in each
// of the last minutesKept minutes, by Unix minute.
type kindCount struct {
	sinceStart  int64
	lastAt      time.Time
	minuteCount map[int64]int64
}

// New returns the recorder, with the failures an earlier start left read from persistent.db.
func New(ctx context.Context, parts Parts) (*Recorder, error) {
	if parts.StartID == "" {
		return nil, errors.New("the recorder of unknown failures needs this start's name")
	}
	rows, err := parts.Database.Read().ListUnknownFailuresOfStartsBefore(ctx, parts.StartID)
	if err != nil {
		return nil, fmt.Errorf("read the unknown failures earlier starts left: %w", err)
	}
	recorder := &Recorder{
		database: parts.Database, settings: parts.Settings, clock: parts.Clock, logger: parts.Logger,
		version: parts.Version, startID: parts.StartID,
		kept: map[key]bool{}, unavailable: map[key]bool{}, earlier: map[string][]string{},
		retries: map[string]Retry{}, counts: map[string]*kindCount{},
	}
	for _, row := range rows {
		recorder.kept[key{row.Kind, row.Subject}] = true
		recorder.earlier[row.Kind] = append(recorder.earlier[row.Kind], row.Subject)
	}
	return recorder, nil
}

// Record keeps a failure of unknown cause with everything it said, logs it marked unexpected with its
// kind (D509 §1), counts it, and holds its subject unavailable until the next start (D511 §3). One
// kept before is kept again, its count raised (§4).
func (recorder *Recorder) Record(ctx context.Context, failure Failure) error {
	now := recorder.clock.Now()
	recorder.logger.ErrorContext(ctx, "work failed for an unknown cause", slogx.EventKey, "unknownFailure",
		"unexpected", true, "kind", failure.Kind, "subject", failure.Subject, "err", failure.Error,
		"output", failure.Output)
	recorder.mutex.Lock()
	recorder.unavailable[key{failure.Kind, failure.Subject}] = true
	recorder.count(failure.Kind, now)
	recorder.mutex.Unlock()
	err := recorder.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.RecordUnknownFailure(ctx, sqlcgen.RecordUnknownFailureParams{
			Kind: failure.Kind, Subject: failure.Subject, ErrorText: failure.Error, Output: failure.Output,
			Version: recorder.version, FailedAt: now.UnixMilli(), StartID: recorder.startID,
		})
	})
	if err != nil {
		return fmt.Errorf("keep the unknown failure of %s %s: %w", failure.Kind, failure.Subject, err)
	}
	recorder.mutex.Lock()
	recorder.kept[key{failure.Kind, failure.Subject}] = true
	recorder.mutex.Unlock()
	return nil
}

// count adds a failure of kind at now to its counts, and drops the minutes past keeping. The caller
// holds mutex.
func (recorder *Recorder) count(kind string, now time.Time) {
	counted := recorder.counts[kind]
	if counted == nil {
		counted = &kindCount{minuteCount: map[int64]int64{}}
		recorder.counts[kind] = counted
	}
	counted.sinceStart++
	counted.lastAt = now
	minute := unixMinute(now)
	counted.minuteCount[minute]++
	for kept := range counted.minuteCount {
		if kept <= minute-minutesKept {
			delete(counted.minuteCount, kept)
		}
	}
}

func unixMinute(at time.Time) int64 { return at.Unix() / 60 }

// IsUnavailable is whether this start holds the work of kind on subject unavailable for a failure of
// unknown cause: one recorded since the server started, not tried again until it starts again
// (D511 §3).
func (recorder *Recorder) IsUnavailable(kind, subject string) bool {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	return recorder.unavailable[key{kind, subject}]
}

// Resolved is the work of kind on subject done: a failure of unknown cause kept for it is taken off,
// as one no longer happening. One not kept writes nothing.
func (recorder *Recorder) Resolved(ctx context.Context, kind, subject string) error {
	recorder.mutex.Lock()
	isKept := recorder.kept[key{kind, subject}]
	recorder.mutex.Unlock()
	if !isKept {
		return nil
	}
	err := recorder.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.DeleteUnknownFailure(ctx, sqlcgen.DeleteUnknownFailureParams{Kind: kind, Subject: subject})
	})
	if err != nil {
		return fmt.Errorf("take off the unknown failure of %s %s: %w", kind, subject, err)
	}
	recorder.mutex.Lock()
	delete(recorder.kept, key{kind, subject})
	delete(recorder.unavailable, key{kind, subject})
	recorder.mutex.Unlock()
	return nil
}

// RegisterRetry has RetryEarlierStarts try once more, by retry, the failures of kind an earlier start
// left. A kind registers before the start's retry; a kind with none leaves its failures as they are,
// kept and shown, until it has one.
func (recorder *Recorder) RegisterRetry(kind string, retry Retry) {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	recorder.retries[kind] = retry
}

// RetryEarlierStarts tries once more each failure of unknown cause an earlier start left, by its kind's
// Retry (D511 §4): a fix arrives only with a new version, and so a start. It runs once a start; a
// second call does nothing. A failure tried that fails again is recorded again, its count raised. A
// kind's Retry that fails is logged, and the others are still tried.
func (recorder *Recorder) RetryEarlierStarts(ctx context.Context) {
	recorder.mutex.Lock()
	earlier, retries := recorder.earlier, recorder.retries
	recorder.earlier = nil
	recorder.mutex.Unlock()
	kinds := make([]string, 0, len(earlier))
	for kind := range earlier {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		retry := retries[kind]
		if retry == nil {
			continue
		}
		if err := retry(ctx, earlier[kind]); err != nil {
			recorder.logger.ErrorContext(ctx, "could not try again the unknown failures an earlier start left",
				"kind", kind, "failures", len(earlier[kind]), "err", err)
			continue
		}
		recorder.logger.InfoContext(ctx, "tried again the unknown failures an earlier start left", "kind", kind,
			"failures", len(earlier[kind]))
	}
}
