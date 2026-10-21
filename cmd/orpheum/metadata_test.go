// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

func TestKeepSortAndSearchKeysCurrent_KeysEveryNameAtStartAndOnEveryChange(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	config := settingstest.Seeded(t, database)
	storetest.ExecRaw(t, database.Path(),
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movies', 'nfo', 'manual', 0, 1)`,
		"INSERT INTO item (id, kind, library_id, added_at) VALUES (10, 'title', 1, 1)",
		"INSERT INTO title (id, kind, identification_state) VALUES (10, 'movie', 'identified')",
		"INSERT INTO version (title_id, created_at) VALUES (10, 1)",
		"INSERT INTO item_text (item_id, language, is_original, name) VALUES (10, 'und', 1, 'Metropolis')")
	pass := identify.NewPass(database, clock.System{}, slog.New(slog.DiscardHandler), nil, config)
	if err := keepSortAndSearchKeysCurrent(t.Context(), pass, config, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	missing := func(locale string) int64 {
		t.Helper()
		count, err := database.Read().CountItemsMissingSortKey(t.Context(), locale)
		if err != nil {
			t.Fatal(err)
		}
		return count
	}
	if missing("en") != 0 {
		t.Error("at start, a name was left with no sort key in the sort locale")
	}
	swedish := map[string]json.RawMessage{string(settings.SortLocale): json.RawMessage(`"sv"`)}
	err := config.Set(t.Context(), swedish, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if missing("sv") != 0 {
		t.Error("after the locale changed, a name had no sort key in it; the change must not wait for a scan")
	}
}

// unreadableSettings is a database that cannot be read.
type unreadableSettings struct{}

func (unreadableSettings) Duration(context.Context, settings.Key) (time.Duration, error) {
	return 0, errors.New("database is locked")
}

// A loop's wait is its setting, read afresh; when the setting cannot be read, the loop goes on at
// the registry's starting value, not a copy of it.
func TestLengthOrStarting_IsTheSettingOrElseItsStartingValue(t *testing.T) {
	t.Parallel()
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	settingstest.Set(t, config, settings.ReadThroughSweepEveryHours, "3")
	logger := slog.New(slog.DiscardHandler)
	if got := lengthOrStarting(t.Context(), config, settings.ReadThroughSweepEveryHours, logger); got != 3*time.Hour {
		t.Errorf("read_through_sweep_every_hours at 3: a wait of %v, want 3h", got)
	}
	for _, check := range []struct {
		key  settings.Key
		want time.Duration
	}{
		{settings.MetadataRefreshCheckEveryHours, 24 * time.Hour},
		{settings.ArtworkServedAtFlushEverySeconds, time.Hour},
	} {
		if got := lengthOrStarting(t.Context(), unreadableSettings{}, check.key, logger); got != check.want {
			t.Errorf("%s unreadable: a wait of %v, want its starting value, %v", check.key, got, check.want)
		}
	}
}

// The jobs no one waits for are looked for at most half the shortest play priority an admin may set
// apart, so a job keeps play priority at most half as long again as its setting says (D234 §3).
func TestDemoteUnwantedEvery_IsAtMostHalfTheShortestPlayPriorityKept(t *testing.T) {
	t.Parallel()
	kept, isDefined := settings.Lookup(settings.PlayPriorityKeptSeconds)
	if !isDefined {
		t.Fatal("play_priority_kept_seconds is not a setting")
	}
	if shortest := time.Duration(kept.Minimum) * kept.Unit; demoteUnwantedEvery > shortest/2 {
		t.Errorf("demoteUnwantedEvery is %v, against play_priority_kept_seconds' shortest, %v; want at most "+
			"half of it", demoteUnwantedEvery, shortest)
	}
}

// The sweeps that derive first matches, requested refreshes, rechecks, the identification passes and
// the reconsidering of ambiguous matches run as derived_work_sweep_every_seconds says, read afresh
// (D231 §3); the reconsidering runs before the first matches, which queue what it returns to them.
func TestSweeps_TheDerivedWorkSweepsRunAsTheirSettingSays(t *testing.T) {
	t.Parallel()
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	settingstest.Set(t, config, settings.DerivedWorkSweepEverySeconds, "420")
	services := jobServices{settings: config}
	derived := map[string]bool{
		sweepFirstMatches: true, sweepRefreshRequests: true, sweepRechecks: true, sweepPasses: true,
		sweepOwedPasses: true, sweepReconsidered: true,
	}
	var order []string
	for _, sweep := range services.sweeps(slog.New(slog.DiscardHandler)) {
		order = append(order, sweep.Name)
		if derived[sweep.Name] {
			delete(derived, sweep.Name)
			if every := sweep.Every(t.Context()); every != 7*time.Minute {
				t.Errorf("the %s sweep runs every %v, want 7m as its setting says", sweep.Name, every)
			}
		}
	}
	if len(derived) != 0 {
		t.Errorf("no sweep named %v", derived)
	}
	if slices.Index(order, sweepReconsidered) > slices.Index(order, sweepFirstMatches) {
		t.Errorf("the sweeps run in the order %v; want ambiguous matches reconsidered before the first matches",
			order)
	}
}

// Every kind runs one at a time: a handler gives its work to a node and answers at once, and the
// core holds each node to its slots and devices (D234 §7, D240 §5), so a second person's prepare is
// not held behind the first's.
func TestRunners_EveryKindRunsOneAtATime(t *testing.T) {
	t.Parallel()
	for _, runner := range (jobServices{}).runners(nil) {
		if runner.concurrency != 1 {
			t.Errorf("%s jobs run %d at once, want 1", runner.kind, runner.concurrency)
		}
	}
}

func TestRunners_EveryKindsWorkIsDerivedByASweepOrAskedForByAPlayAndItsOwnFailureIsAFact(t *testing.T) {
	t.Parallel()
	services := jobServices{}
	sweepNames := map[string]bool{}
	for _, sweep := range services.sweeps(slog.New(slog.DiscardHandler)) {
		if sweep.Derive == nil || sweep.Every == nil || sweepNames[sweep.Name] {
			t.Errorf("sweep %q is incomplete or named twice", sweep.Name)
		}
		sweepNames[sweep.Name] = true
	}
	run := map[jobs.Kind]bool{}
	for _, runner := range services.runners(nil) {
		run[runner.kind] = true
		checkDerivation(t, runner.kind, runner.derivedBy, sweepNames)
		// A measurement that gave up keeps nothing: only FFmpeg's word on the file is the file's own
		// failure, which its node's answer keeps, so the next sweep asks again (D244 §1). A backup and
		// the certificate's renewal have no item: a failure is the job's own, which the job view shows
		// (D243 §6, §11).
		isKeptByItsItem := !slices.Contains(
			[]jobs.Kind{jobs.KindReadThrough, jobs.KindBackup, jobs.KindRenewCertificate}, runner.kind)
		if isGivenUpKept := runner.recordFailure != nil; isGivenUpKept != isKeptByItsItem {
			t.Errorf("%s keeps a job that gave up as its item's own failure: %v (D231 §4, §10; D244 §1)",
				runner.kind, isGivenUpKept)
		}
	}
	for _, kind := range jobs.Kinds() {
		if !run[kind] {
			t.Errorf("no runner works %s", kind)
		}
	}
	for _, name := range sweepsAfterAPass() {
		if !sweepNames[name] || name == sweepPasses {
			t.Errorf("a pass asks for %q, which is no sweep, or the pass's own", name)
		}
	}
}

// checkDerivation fails unless a kind's work is derived by sweeps that exist, or asked for by a play or
// a scan's request.
func checkDerivation(t *testing.T, kind jobs.Kind, derivedBy []string, sweepNames map[string]bool) {
	t.Helper()
	if len(derivedBy) == 0 {
		t.Errorf("%s's work is derived by nothing: a job lost with cache.db would be lost for good (D231 §3)", kind)
	}
	if slices.Equal(derivedBy, derivedByAPlay()) || slices.Equal(derivedBy, derivedByAScanRequest()) {
		return
	}
	for _, name := range derivedBy {
		if !sweepNames[name] {
			t.Errorf("%s's work is derived by %q, which is no sweep", kind, name)
		}
	}
}

func TestScenario_S71_APersonsRefreshBehindARollingOneRunsAtTheirPriorityAndAsksForTheImagesAgain(t *testing.T) {
	t.Parallel()
	cache := storetest.OpenCache(t)
	queue := jobs.NewQueue(jobs.QueueParts{
		Cache: cache, Clock: clocktest.NewFake(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)),
		Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)), Random: jobqueuetest.Unjittered(),
		Logger: slog.New(slog.DiscardHandler), ArgsMerges: jobArgsMerges(),
	})
	enqueue := func(itemID int64, reason string, shouldRetryImages bool, priority int64) {
		t.Helper()
		job, err := identify.RefreshJob(itemID, reason, time.Time{}, shouldRetryImages, priority)
		if err == nil {
			_, err = queue.Enqueue(t.Context(), job)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	enqueue(1, "its tmdb data is due to be fetched again", false, -10)
	enqueue(1, "a person asked for it to be refreshed", true, 20)
	firstMatch := jobs.Request{Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: 2}}
	if _, err := queue.Enqueue(t.Context(), firstMatch); err != nil {
		t.Fatal(err)
	}
	enqueue(2, "a person asked for it to be refreshed", true, 20)
	rows, err := cache.Read().ListJobs(t.Context(), 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("%d jobs (%v) after two requests for each of two items, want one each", len(rows), err)
	}
	wantArgs := map[int64]string{
		1: `{"isRefresh":true,"shouldRetryImages":true}`,
		2: "{}", // a match and a refresh do different things; the match is kept (identify.FetchArgsMerge)
	}
	for _, row := range rows {
		if itemID := *row.ItemID; row.Priority != 20 || row.ArgsJSON != wantArgs[itemID] {
			t.Errorf("item %d's job: priority %d, arguments %s; want the person's priority, arguments %s", itemID,
				row.Priority, row.ArgsJSON, wantArgs[itemID])
		}
	}
}

// The job view hears from the exchanges where the node holding a job's work last said it is: here
// its status, which lists the attempt paused (D240 §6).
func TestReportedProgressOf_IsWhereTheNodeHoldingAJobsWorkLastSaidItIs(t *testing.T) {
	t.Parallel()
	cache := storetest.OpenCache(t)
	err := cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		return queries.InsertNodeAttempt(t.Context(), cachegen.InsertNodeAttemptParams{
			AttemptToken: "attempt-7", JobID: 7, JobKind: string(jobs.KindReadThrough), NodeID: 1,
			WorkJSON: `{"type":"read_through"}`, SentAt: 1,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	exchanges := nodes.NewExchanges(nodes.ExchangeParts{
		Clock: clocktest.NewFake(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)), Random: rand.Reader,
		Logger: slog.New(slog.DiscardHandler), Cache: cache, MaxSilence: time.Minute,
		Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)),
	})
	if err := exchanges.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	words := reportedProgressOf(exchanges)
	if word, isSaid := words(7); isSaid {
		t.Errorf("before its node spoke of job 7 its word is %q, want none", word)
	}
	exchanges.Reconcile(t.Context(), 1,
		[]nodeproto.HeldAttempt{{JobID: 7, AttemptToken: "attempt-7", State: nodeproto.HeldPaused}}, nil)
	if word, isSaid := words(7); !isSaid || word != "paused" {
		t.Errorf("once its node's status listed job 7 paused, its word is %q (%v), want paused", word, isSaid)
	}
}

// toldSender is a node that records what it is told, and has no work sent to it.
type toldSender struct{ told chan nodeproto.Command }

func (sender toldSender) Enqueue(_ int64, command nodeproto.Command) { sender.told <- command }

func (toldSender) Send(context.Context, int64, nodeproto.Command) error { return nil }

func (toldSender) ParkUnlessConnected(context.Context, int64) error { return nil }

func (toldSender) Withdraw(int64, string) bool { return false }

func (toldSender) StartOf(int64) string { return "" }

func (toldSender) Unanswered(_ context.Context, _ int64, err error) error { return err }

func (toldSender) Unpark(context.Context, jobs.Destination) {}

func (toldSender) Resources(context.Context, int64) (nodes.Resources, bool, error) {
	return nodes.Resources{}, false, nil
}

// A play's job no one waits for any more falls to background priority at the next look, and the
// node holding one given to it is told (D234 §3, S87).
func TestDemoteUnwanted_APlaysJobNoOneWaitsForFallsAndItsNodeIsTold(t *testing.T) {
	t.Parallel()
	cache := storetest.OpenCache(t)
	now := clocktest.NewFake(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC))
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	logger := slog.New(slog.DiscardHandler)
	queue := jobs.NewQueue(jobs.QueueParts{
		Cache: cache, Clock: now, Settings: config, Random: jobqueuetest.Unjittered(), Logger: logger,
	})
	if _, err := queue.Enqueue(t.Context(), jobs.Request{
		Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 4}, Priority: jobs.PriorityPlayNow, Reason: "test",
	}); err != nil {
		t.Fatal(err)
	}
	runner := queue.NewRunner(jobs.KindReadThrough, func(context.Context, jobs.Job) error {
		return jobs.Assign(now.Now().Add(time.Hour))
	})
	if err := runner.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	listed, err := cache.Read().ListJobs(t.Context(), 1)
	if err != nil || len(listed) != 1 {
		t.Fatalf("the jobs are %+v (%v), want the one measurement", listed, err)
	}
	job := listed[0].ID
	err = cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		return queries.InsertNodeAttempt(t.Context(), cachegen.InsertNodeAttemptParams{
			AttemptToken: "attempt", JobID: job, JobKind: string(jobs.KindReadThrough), NodeID: 1,
			WorkJSON: `{"type":"read_through"}`, SentAt: 1,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	told := toldSender{told: make(chan nodeproto.Command, 1)}
	exchanges := nodes.NewExchanges(nodes.ExchangeParts{
		Sender: told, Rooms: told, Clock: now, Random: rand.Reader, Logger: logger, Cache: cache, Settings: config,
		MaxSilence: time.Hour,
	})
	if err = exchanges.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	now.Advance(11 * time.Minute)
	tick := make(chan time.Time, 1)
	tick <- now.Now()
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		demoteUnwanted(ctx, queue, exchanges, tick, logger)
	}()
	command := testutil.Receive(t, told.told, "the node told")
	stop()
	testutil.Receive(t, done, "the loop's end")
	if command.Type != nodeproto.CommandReprioritize || command.JobID != job ||
		command.Priority != jobs.PriorityShownInterest {
		t.Errorf("the node was told %+v, want job %d at the shown-interest priority", command, job)
	}
	// Someone wanted it, so it stays a rung above the background (D324).
	again, err := cache.Read().ListJobs(t.Context(), 1)
	if err != nil || len(again) != 1 || again[0].Priority != jobs.PriorityShownInterest {
		t.Errorf("the job is %+v (%v), want it at the shown-interest priority", again, err)
	}
}

func TestSweeps_TheScanScheduleIsASweepCheckedEveryQuarterHour(t *testing.T) {
	t.Parallel()
	for _, sweep := range (jobServices{}).sweeps(slog.New(slog.DiscardHandler)) {
		if sweep.Name == sweepScheduledScans {
			if every := sweep.Every(t.Context()); every != 15*time.Minute {
				t.Errorf("the scan schedule is checked every %s, want 15m", every)
			}
			return
		}
	}
	t.Error("no sweep asks for the scans libraries are due (D272 §1)")
}

// brokenInvariants is a database whose data breaks the invariants it names.
type brokenInvariants []store.Violation

func (broken brokenInvariants) CheckInvariants(context.Context) ([]store.Violation, error) {
	return broken, nil
}

// The database's invariants are checked by a sweep, at start and daily, and each one the data
// breaks is logged as an error with the rows it fails at (D303, M7): the owner's NAS gathered 80,246
// empty titles under one series that nothing reported.
func TestSweeps_TheDatabasesInvariantsAreCheckedDailyAndEachBrokenOneLoggedAsAnError(t *testing.T) {
	t.Parallel()
	var logged bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logged, nil))
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	services := jobServices{settings: config, invariants: brokenInvariants{
		{Invariant: "every live title has a version", Rows: []string{"607", "608"}},
	}}
	for _, sweep := range services.sweeps(logger) {
		if sweep.Name != sweepInvariants {
			continue
		}
		if every := sweep.Every(t.Context()); every != 24*time.Hour {
			t.Errorf("the invariants are checked every %s, want daily", every)
		}
		settingstest.Set(t, config, settings.InvariantsCheckEveryHours, "6")
		if every := sweep.Every(t.Context()); every != 6*time.Hour {
			t.Errorf("with invariants_check_every_hours at 6, the invariants are checked every %s", every)
		}
		if _, err := sweep.Derive(t.Context()); err != nil {
			t.Fatal(err)
		}
		if line := logged.String(); !strings.Contains(line, "level=ERROR") ||
			!strings.Contains(line, "every live title has a version") || !strings.Contains(line, "607") {
			t.Errorf("logged %q; want an error naming the invariant and its rows", line)
		}
		return
	}
	t.Error("no sweep checks the database's invariants (D303, M7)")
}

// The scheduled backup's sweep looks every five minutes, so an hourly backup comes within five of
// its hour (D305 §2).
func TestSweeps_TheScheduledBackupIsCheckedEveryFiveMinutes(t *testing.T) {
	t.Parallel()
	for _, sweep := range (jobServices{}).sweeps(slog.New(slog.DiscardHandler)) {
		if sweep.Name == sweepScheduledBackup {
			if every := sweep.Every(t.Context()); every != 5*time.Minute {
				t.Errorf("the scheduled backup is checked every %s, want 5m", every)
			}
			return
		}
	}
	t.Error("no sweep asks for the scheduled backup (D305 §2)")
}

// countingPurger counts the purges of ended credentials asked of it.
type countingPurger struct{ purges int }

func (purger *countingPurger) PurgeEndedCredentials(context.Context) (int, error) {
	purger.purges++
	return 0, nil
}

// The credentials ended past their keeping are purged by a sweep, at start and daily (D304 §6).
func TestSweeps_EndedCredentialsArePurgedDaily(t *testing.T) {
	t.Parallel()
	purger := &countingPurger{}
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	for _, sweep := range (jobServices{credentials: purger, settings: config}).sweeps(slog.New(slog.DiscardHandler)) {
		if sweep.Name != sweepEndedCredentials {
			continue
		}
		if every := sweep.Every(t.Context()); every != 24*time.Hour {
			t.Errorf("ended credentials are purged every %s, want daily", every)
		}
		settingstest.Set(t, config, settings.RetentionSweepEveryHours, "12")
		if every := sweep.Every(t.Context()); every != 12*time.Hour {
			t.Errorf("with retention_sweep_every_hours at 12, ended credentials are purged every %s", every)
		}
		if _, err := sweep.Derive(t.Context()); err != nil || purger.purges != 1 {
			t.Errorf("the sweep purged %d times (%v); want once", purger.purges, err)
		}
		return
	}
	t.Error("no sweep purges ended credentials (D304 §6)")
}

// countingFilePurger counts the purges of removed files asked of it, and what each was asked.
type countingFilePurger struct {
	purges        int
	removedBefore time.Time
	forgetCount   int
}

func (purger *countingFilePurger) PurgeRemovedFiles(
	_ context.Context, removedBefore time.Time, forgets ...library.PurgedFilesForget,
) (int, error) {
	purger.purges++
	purger.removedBefore, purger.forgetCount = removedBefore, len(forgets)
	return 0, nil
}

// The files removed past their keeping are purged by a sweep, at start and daily, of those removed
// more than removed_files_kept_days ago, with what playback, preferences and subtitles hold of them
// dealt with first (D304 §6, D306 §8).
func TestSweeps_RemovedFilesArePurgedDaily(t *testing.T) {
	t.Parallel()
	purger := &countingFilePurger{}
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	now := time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)
	if _, err := purgeRemovedFiles(purger, config, clocktest.NewFake(now))(t.Context()); err != nil ||
		purger.purges != 1 || purger.forgetCount != 3 {
		t.Fatalf("the sweep purged %d times with %d forgets (%v); want once with 3", purger.purges,
			purger.forgetCount, err)
	}
	if want := now.AddDate(0, 0, -90); !purger.removedBefore.Equal(want) {
		t.Errorf("the sweep purged the files removed before %s; want %s, 90 days, the default",
			purger.removedBefore, want)
	}
	for _, sweep := range (jobServices{removedFiles: purger, settings: config}).sweeps(slog.New(slog.DiscardHandler)) {
		if sweep.Name != sweepRemovedFiles {
			continue
		}
		if every := sweep.Every(t.Context()); every != 24*time.Hour {
			t.Errorf("removed files are purged every %s, want daily", every)
		}
		return
	}
	t.Error("no sweep purges removed files (D306 §8)")
}
