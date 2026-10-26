// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// The sweeps that derive first matches, requested refreshes, rechecks, the identification passes,
// the reconsidering of ambiguous matches and what should have been fetched (D335) run as
// derived_work_sweep_every_seconds says, read afresh
// (D231 §3); the reconsidering runs before the first matches, which queue what it returns to them.
func TestSweeps_TheDerivedWorkSweepsRunAsTheirSettingSays(t *testing.T) {
	t.Parallel()
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	settingstest.Set(t, config, settings.DerivedWorkSweepEverySeconds, "420")
	services := Services{settings: config}
	derived := map[string]bool{
		sweepFirstMatches: true, sweepRefreshRequests: true, sweepRechecks: true, sweepPasses: true,
		sweepOwedPasses: true, sweepReconsidered: true, sweepMissing: true,
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
// core holds each node to its slots and devices (D234 §7, D240 §5), so a second person's run is
// not held behind the first's.
func TestRunners_EveryKindRunsOneAtATime(t *testing.T) {
	t.Parallel()
	for _, runner := range (&Services{}).runners(nil) {
		if runner.concurrency != 1 {
			t.Errorf("%s jobs run %d at once, want 1", runner.kind, runner.concurrency)
		}
	}
}

func TestRunners_EveryKindsWorkIsDerivedByASweepOrAskedForByAPlayAndItsOwnFailureIsAFact(t *testing.T) {
	t.Parallel()
	services := Services{}
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

func TestSweeps_TheScanScheduleIsASweepCheckedEveryQuarterHour(t *testing.T) {
	t.Parallel()
	for _, sweep := range (&Services{}).sweeps(slog.New(slog.DiscardHandler)) {
		if sweep.Name == sweepScheduledScans {
			if every := sweep.Every(t.Context()); every != 15*time.Minute {
				t.Errorf("the scan schedule is checked every %s, want 15m", every)
			}
			return
		}
	}
	t.Error("no sweep asks for the scans libraries are due (D272 §1)")
}

// An unavailable root is scanned again every unavailable_root_rescan_seconds, by a sweep of its own:
// the scan schedule's quarter hour was too long for a NAS back in a minute, and a minute left a film
// 45 s from resuming (staging, 2026-10-21).
func TestSweeps_UnavailableRootsAreScannedAgainEveryRescanInterval(t *testing.T) {
	t.Parallel()
	services := Services{settings: settingstest.Seeded(t, storetest.OpenMigrated(t))}
	for _, sweep := range services.sweeps(slog.New(slog.DiscardHandler)) {
		if sweep.Name == sweepUnavailableRoots {
			if every := sweep.Every(t.Context()); every != 10*time.Second {
				t.Errorf("the unavailable roots are scanned again every %s, want 10s", every)
			}
			return
		}
	}
	t.Error("no sweep scans the unavailable roots again (D233 §10)")
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
	services := Services{settings: config, invariants: brokenInvariants{
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
	for _, sweep := range (&Services{}).sweeps(slog.New(slog.DiscardHandler)) {
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
	for _, sweep := range (&Services{credentials: purger, settings: config}).sweeps(slog.New(slog.DiscardHandler)) {
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
	for _, sweep := range (&Services{removedFiles: purger, settings: config}).sweeps(slog.New(slog.DiscardHandler)) {
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
