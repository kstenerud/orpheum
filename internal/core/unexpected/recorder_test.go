// SPDX-License-Identifier: AGPL-3.0-or-later

package unexpected_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/core/unexpected"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

var started = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)

type fixture struct {
	database *store.Database
	settings *settings.Service
	clock    *clocktest.Fake
	log      *bytes.Buffer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	database := storetest.OpenMigrated(t)
	return &fixture{
		database: database, settings: settingstest.Seeded(t, database), clock: clocktest.NewFake(started),
		log: &bytes.Buffer{},
	}
}

// start is a start of the server named startID, on the version given.
func (fixture *fixture) start(t *testing.T, startID, version string) *unexpected.Recorder {
	t.Helper()
	logger, err := slogx.New(fixture.log, slogx.FormatJSON, slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := unexpected.New(t.Context(), unexpected.Parts{
		Database: fixture.database, Settings: fixture.settings, Clock: fixture.clock, Logger: logger,
		Version: version, StartID: startID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return recorder
}

func record(t *testing.T, recorder *unexpected.Recorder, kind, subject, said string) {
	t.Helper()
	if err := recorder.Record(t.Context(), unexpected.Failure{
		Kind: kind, Subject: subject, Error: said, Output: "the tool's whole standard error\nline two",
	}); err != nil {
		t.Fatal(err)
	}
}

func report(t *testing.T, recorder *unexpected.Recorder) unexpected.Report {
	t.Helper()
	got, err := recorder.Report(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRecord_KeepsEverythingTheFailureSaidAndHoldsItsSubjectUnavailableThisStart(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	recorder := fixture.start(t, "aa", "v0.9.0")
	long := strings.Repeat("an error that goes on ", 500)
	record(t, recorder, "fetch_metadata", "item 12", long)
	if !recorder.IsUnavailable("fetch_metadata", "item 12") {
		t.Error("a subject whose work failed for an unknown cause is not unavailable this start")
	}
	if recorder.IsUnavailable("fetch_metadata", "item 13") || recorder.IsUnavailable("make_chunks", "item 12") {
		t.Error("another subject, or another kind's, is unavailable")
	}
	got := report(t, recorder)
	if len(got.Kinds) != 1 || len(got.Kinds[0].Failures) != 1 {
		t.Fatalf("report = %+v, want one kind with one failure", got)
	}
	kept := got.Kinds[0].Failures[0]
	if kept.Error != long || kept.Output != "the tool's whole standard error\nline two" || kept.Version != "v0.9.0" ||
		!kept.LastAt.Equal(started) || kept.Count != 1 || !kept.IsThisStart {
		t.Errorf("kept = %+v; want the error and output whole, the version, the time, a count of 1, this start", kept)
	}
	if got.Kinds[0].SinceStart != 1 || got.Kinds[0].LastHour != 1 {
		t.Errorf("counts = %d since start, %d last hour; want 1 and 1", got.Kinds[0].SinceStart, got.Kinds[0].LastHour)
	}
}

func TestRecord_LogsTheFailureMarkedUnexpectedWithItsKind(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	recorder := fixture.start(t, "aa", "v0.9.0")
	record(t, recorder, "fetch_metadata", "item 12", "the provider said what was not understood")
	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(fixture.log.Bytes()), &line); err != nil {
		t.Fatalf("the log is not one JSON line: %v\n%s", err, fixture.log)
	}
	isMarked, isBool := line["unexpected"].(bool)
	if !isBool || !isMarked || line["kind"] != "fetch_metadata" || line[slogx.EventKey] != "unknownFailure" ||
		line["level"] != "ERROR" || line["err"] != "the provider said what was not understood" {
		t.Errorf("logged %v; want it marked unexpected, with its kind, its error, at ERROR", line)
	}
}

func TestNew_AFailureOfAnEarlierStartIsTriedOnceByItsKindsRetryAndAvailableAgain(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	first := fixture.start(t, "aa", "v0.9.0")
	record(t, first, "fetch_metadata", "item 12", "first")
	record(t, first, "fetch_metadata", "item 14", "first")
	record(t, first, "make_chunks", "file 3", "first")

	second := fixture.start(t, "bb", "v0.9.1")
	if second.IsUnavailable("fetch_metadata", "item 12") {
		t.Error("a failure of an earlier start is still unavailable at the next")
	}
	var retried []string
	calls := 0
	second.RegisterRetry("fetch_metadata", func(_ context.Context, subjects []string) error {
		calls++
		retried = append(retried, subjects...)
		return nil
	})
	second.RetryEarlierStarts(t.Context())
	second.RetryEarlierStarts(t.Context())
	if calls != 1 || !slices.Equal(retried, []string{"item 12", "item 14"}) {
		t.Errorf("retried %v in %d calls; want items 12 and 14, once", retried, calls)
	}

	fixture.clock.Advance(time.Minute)
	record(t, second, "fetch_metadata", "item 12", "again")
	got := report(t, second)
	for _, kind := range got.Kinds {
		for _, kept := range kind.Failures {
			if kept.Subject == "item 12" && (kept.Count != 2 || kept.Error != "again" || kept.Version != "v0.9.1" ||
				!kept.FirstAt.Equal(started) || !kept.IsThisStart) {
				t.Errorf("item 12 again = %+v; want a count of 2, the newest error and version, the first time kept",
					kept)
			}
			if kept.Subject == "file 3" && kept.IsThisStart {
				t.Error("a failure no retry of this start tried says it is this start's")
			}
		}
	}
}

func TestRetryEarlierStarts_AKindsRetryThatFailsLeavesTheOthersTried(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	first := fixture.start(t, "aa", "v0.9.0")
	record(t, first, "a_kind", "one", "first")
	record(t, first, "b_kind", "two", "first")
	second := fixture.start(t, "bb", "v0.9.0")
	second.RegisterRetry("a_kind", func(context.Context, []string) error { return errors.New("the disk said no") })
	var retried []string
	second.RegisterRetry("b_kind", func(_ context.Context, subjects []string) error {
		retried = subjects
		return nil
	})
	second.RetryEarlierStarts(t.Context())
	if !slices.Equal(retried, []string{"two"}) {
		t.Errorf("b_kind retried %v; want [two] after a_kind's retry failed", retried)
	}
}

func TestResolved_TakesTheFailureOffAndItsSubjectIsAvailable(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	recorder := fixture.start(t, "aa", "v0.9.0")
	record(t, recorder, "fetch_metadata", "item 12", "first")
	if err := recorder.Resolved(t.Context(), "fetch_metadata", "item 12"); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Resolved(t.Context(), "fetch_metadata", "item 99"); err != nil {
		t.Fatal(err)
	}
	if recorder.IsUnavailable("fetch_metadata", "item 12") {
		t.Error("a resolved subject is still unavailable")
	}
	got := report(t, recorder)
	if len(got.Kinds) != 1 || len(got.Kinds[0].Failures) != 0 || got.Kinds[0].SinceStart != 1 {
		t.Errorf("report = %+v; want the kind with its count and no failure kept", got)
	}
	if again := fixture.start(t, "bb", "v0.9.0"); len(report(t, again).Kinds) != 0 {
		t.Error("the next start still finds the resolved failure")
	}
}

func TestAlerts_AKindPastTheSettingInAnHourRaisesItAndItClearsAfterAnHourUnder(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t)
	settingstest.Set(t, fixture.settings, settings.UnknownFailuresAlertPerHour, "3")
	recorder := fixture.start(t, "aa", "v0.9.0")
	alerts := func() []unexpected.Alert {
		t.Helper()
		got, err := recorder.Alerts(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	for range 3 {
		record(t, recorder, "fetch_metadata", "item 1", "again")
		fixture.clock.Advance(10 * time.Minute)
	}
	if got := alerts(); len(got) != 0 {
		t.Fatalf("alerts at the setting's count = %v; want none until one is past it", got)
	}
	record(t, recorder, "fetch_metadata", "item 1", "again")
	record(t, recorder, "make_chunks", "file 1", "once")
	got := alerts()
	if len(got) != 1 || got[0].Kind != "fetch_metadata" || got[0].LastHour != 4 {
		t.Fatalf("alerts past the count = %v; want fetch_metadata, 4 in the last hour", got)
	}
	// Forty minutes on, the first two are out of the hour and the kind is under, but not yet an hour under.
	fixture.clock.Advance(40 * time.Minute)
	if got := alerts(); len(got) != 1 || got[0].LastHour != 2 {
		t.Errorf("alerts under for less than an hour = %v; want it still raised, 2 in the last hour", got)
	}
	fixture.clock.Advance(61 * time.Minute)
	if got := alerts(); len(got) != 0 {
		t.Errorf("alerts after an hour under = %v; want none", got)
	}
	if kinds := report(t, recorder).Kinds; kinds[0].SinceStart != 4 || kinds[0].IsRaised {
		t.Errorf("report = %+v; want 4 since start, not raised", kinds[0])
	}
}
