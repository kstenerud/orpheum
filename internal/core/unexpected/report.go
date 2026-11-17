// SPDX-License-Identifier: AGPL-3.0-or-later

package unexpected

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// Alert is a kind of work past the setting's count of failures of unknown cause in an hour, or under
// it for less than an hour since (D509 §2, §3).
type Alert struct {
	Kind string
	// LastHour is how many of its failures the last hour had.
	LastHour int64
}

// Kind is one kind's failures of unknown cause, for the Needs attention page and the export
// (D509 §4, D511 §6).
type Kind struct {
	Kind string
	// LastHour and SinceStart are how many of its failures the last hour and this start had.
	LastHour, SinceStart int64
	// LastAt is when one last failed, at this start or an earlier one.
	LastAt time.Time
	// IsRaised is whether it raises the alert now.
	IsRaised bool
	// Failures are those kept, the newest first.
	Failures []KeptFailure
}

// KeptFailure is a failure of unknown cause as persistent.db keeps it.
type KeptFailure struct {
	Failure
	// Version is the server's version it last failed on.
	Version string
	// FirstAt and LastAt are when it first and last failed; Count is how many times.
	FirstAt, LastAt time.Time
	Count           int64
	// IsThisStart is whether it last failed at this start, so its subject is unavailable until the
	// next.
	IsThisStart bool
}

// Report is every failure of unknown cause kept, by kind, the kinds in name order.
type Report struct {
	Version string
	StartID string
	At      time.Time
	Kinds   []Kind
}

// Alerts are the kinds that raise the alert now: each past the setting's count in an hour that ended
// within the last hour (D509 §2), in name order.
func (recorder *Recorder) Alerts(ctx context.Context) ([]Alert, error) {
	threshold, err := recorder.threshold(ctx)
	if err != nil {
		return nil, err
	}
	now := recorder.clock.Now()
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	alerts := []Alert{}
	for kind, counted := range recorder.counts {
		if counted.isRaised(now, threshold) {
			alerts = append(alerts, Alert{Kind: kind, LastHour: counted.lastHour(now)})
		}
	}
	sort.Slice(alerts, func(i, j int) bool { return alerts[i].Kind < alerts[j].Kind })
	return alerts, nil
}

func (recorder *Recorder) threshold(ctx context.Context) (int64, error) {
	threshold, err := recorder.settings.Int(ctx, settings.UnknownFailuresAlertPerHour)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", settings.UnknownFailuresAlertPerHour, err)
	}
	return threshold, nil
}

// windowEnding is how many failures the hour that ends with minute had.
func (counted *kindCount) windowEnding(minute int64) int64 {
	var sum int64
	for at := minute - 59; at <= minute; at++ {
		sum += counted.minuteCount[at]
	}
	return sum
}

func (counted *kindCount) lastHour(now time.Time) int64 { return counted.windowEnding(unixMinute(now)) }

// isRaised is whether an hour ending in one of the last hour's minutes had more than threshold: a
// kind over raises the alert, and it clears once an hour has passed under (D509 §2).
func (counted *kindCount) isRaised(now time.Time, threshold int64) bool {
	minute := unixMinute(now)
	for ending := minute - 60; ending <= minute; ending++ {
		if counted.windowEnding(ending) > threshold {
			return true
		}
	}
	return false
}

// Report is every failure of unknown cause persistent.db keeps, with each kind's counts: the Needs
// attention page's and the admin's export (D511 §6). The kinds counted this start with none kept,
// their failures resolved since, are listed with their counts and no failures.
func (recorder *Recorder) Report(ctx context.Context) (Report, error) {
	threshold, err := recorder.threshold(ctx)
	if err != nil {
		return Report{}, err
	}
	rows, err := recorder.database.Read().ListUnknownFailures(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("read the unknown failures: %w", err)
	}
	now := recorder.clock.Now()
	byKind := map[string]*Kind{}
	kindOf := func(name string) *Kind {
		kind := byKind[name]
		if kind == nil {
			kind = &Kind{Kind: name, Failures: []KeptFailure{}}
			byKind[name] = kind
		}
		return kind
	}
	for _, row := range rows {
		kind := kindOf(row.Kind)
		lastAt := time.UnixMilli(row.LastFailedAt).UTC()
		kind.Failures = append(kind.Failures, KeptFailure{
			Failure: Failure{Kind: row.Kind, Subject: row.Subject, Error: row.ErrorText, Output: row.Output},
			Version: row.Version, FirstAt: time.UnixMilli(row.FirstFailedAt).UTC(), LastAt: lastAt,
			Count: row.FailureCount, IsThisStart: row.StartID == recorder.startID,
		})
		if lastAt.After(kind.LastAt) {
			kind.LastAt = lastAt
		}
	}
	recorder.mutex.Lock()
	for name, counted := range recorder.counts {
		kind := kindOf(name)
		kind.LastHour, kind.SinceStart = counted.lastHour(now), counted.sinceStart
		kind.IsRaised = counted.isRaised(now, threshold)
		if counted.lastAt.After(kind.LastAt) {
			kind.LastAt = counted.lastAt.UTC()
		}
	}
	recorder.mutex.Unlock()
	report := Report{Version: recorder.version, StartID: recorder.startID, At: now.UTC(), Kinds: []Kind{}}
	for _, kind := range byKind {
		report.Kinds = append(report.Kinds, *kind)
	}
	sort.Slice(report.Kinds, func(i, j int) bool { return report.Kinds[i].Kind < report.Kinds[j].Kind })
	return report, nil
}
