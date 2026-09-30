// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
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

func TestRunners_PreparesRunAsManyAtOnceAsANodeHasSlots(t *testing.T) {
	t.Parallel()
	for _, runner := range (jobServices{}).runners() {
		want := 1
		if runner.kind == jobs.KindPrepare {
			// A second person's copy is not held behind the first's.
			want = nodeproto.EncodeSlotCount + nodeproto.RemuxSlotCount
		}
		if runner.concurrency != want {
			t.Errorf("%s jobs run %d at once, want %d (cut-a §5.3)", runner.kind, runner.concurrency, want)
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
