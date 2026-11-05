// SPDX-License-Identifier: AGPL-3.0-or-later

package assembly

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func TestKeepSortAndSearchKeysCurrent_KeysEveryNameAtStartAndOnEveryChange(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	config := settingstest.Seeded(t, database)
	storetest.ExecRaw(t, database.Path(),
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movie', 'nfo', 'manual', 0, 1)`)
	titleID := librarytest.Title(t, database, librarytest.TitleSpec{ID: 10, Library: 1})
	librarytest.Version(t, database, librarytest.VersionSpec{TitleID: titleID})
	storetest.ExecRaw(t, database.Path(),
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
	rows, err := cache.Read().ListJobs(t.Context(), cachegen.ListJobsParams{MaxRows: 10})
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

// The queue the server runs folds a player's index read into the sweep's queued full read, which
// becomes the index read the player waits on (readthrough.ArgsMerge, D269 §5).
func TestJobArgsMerges_APlayersIndexReadFoldsIntoTheSweepsQueuedFullRead(t *testing.T) {
	t.Parallel()
	cache := storetest.OpenCache(t)
	now := clocktest.NewFake(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	queue := jobs.NewQueue(jobs.QueueParts{
		Cache: cache, Clock: now, Settings: settingstest.Seeded(t, storetest.OpenMigrated(t)),
		Random: jobqueuetest.Unjittered(), Logger: slog.New(slog.DiscardHandler), ArgsMerges: jobArgsMerges(),
	})
	for _, request := range []jobs.Request{
		{
			Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 4}, Priority: jobs.PriorityBackground,
			Reason: "sweep",
		},
		{
			Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 4}, Priority: jobs.PriorityPlayNow, Reason: "play",
			ArgsJSON: `{"isFromIndexOnly":true}`,
		},
	} {
		if _, err := queue.Enqueue(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := cache.Read().ListJobs(t.Context(), cachegen.ListJobsParams{MaxRows: 10})
	if err != nil || len(listed) != 1 || listed[0].ArgsJSON != `{"isFromIndexOnly":true}` ||
		listed[0].Priority != jobs.PriorityPlayNow {
		t.Errorf("the jobs are %+v (%v); want the one, an index read at a play's priority", listed, err)
	}
}
