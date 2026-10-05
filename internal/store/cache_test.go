// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

func openCacheAt(t *testing.T, path string) *Cache {
	t.Helper()
	cache, err := OpenCache(t.Context(), path, testOpenOptions())
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return cache
}

func TestOpenCache_BuildsAnEmptyFileAndKeepsWhatItHolds(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cache.db")
	cache, err := OpenCache(t.Context(), path, testOpenOptions())
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	err = cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		_, insertErr := queries.InsertJob(t.Context(), cachegen.InsertJobParams{
			Kind: "fetch_metadata", ArgsJSON: "{}", RunnableAt: 1, CreatedAt: 1,
		})
		return insertErr //nolint:wrapcheck // the test reads it as is
	})
	if err != nil {
		t.Fatalf("InsertJob: %v", err)
	}
	if closeErr := cache.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	reopened := openCacheAt(t, path)
	jobs, err := reopened.Read().ListJobs(t.Context(), 10)
	if err != nil || len(jobs) != 1 {
		t.Errorf("after reopening: %d jobs, %v; want the one written", len(jobs), err)
	}
}

func TestOpenCache_ReportsAnotherVersionAsOutdated(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cache.db")
	cache := openCacheAt(t, path)
	execRaw(t, cache.database, "PRAGMA user_version = 999")
	_, err := OpenCache(t.Context(), path, testOpenOptions())
	if !errors.Is(err, ErrCacheOutdated) {
		t.Errorf("OpenCache over version 999 = %v, want ErrCacheOutdated", err)
	}
}

func TestOpenCache_RefusesPersistentDB(t *testing.T) {
	t.Parallel()
	library := openMigrated(t)
	_, err := OpenCache(t.Context(), library.Path(), testOpenOptions())
	if !errors.Is(err, ErrNotOurs) {
		t.Errorf("OpenCache on persistent.db = %v, want ErrNotOurs, so it is never discarded", err)
	}
}

func TestCacheVersion_IsDeclaredByTheSchema(t *testing.T) {
	t.Parallel()
	if version, err := CacheVersion(); err != nil || version < 1 {
		t.Errorf("CacheVersion() = %d, %v", version, err)
	}
}

func TestOpenCache_HoldsNoSubtitleTextWhichPersistentDBKeeps(t *testing.T) {
	t.Parallel()
	// Making a track's text again reads a whole video: it is kept in persistent.db, and cache.db,
	// which a release may discard, holds none (the architecture map's F1, D231 section 3).
	const named = "SELECT name FROM sqlite_master WHERE name = 'subtitle_text'"
	cache := openCacheAt(t, filepath.Join(t.TempDir(), "cache.db"))
	if rows := queryRows(t, cache.database, named, 1); len(rows) != 0 {
		t.Errorf("cache.db has %v, want no subtitle_text", rows)
	}
	library := openMigrated(t)
	if rows := queryRows(t, library, named, 1); len(rows) != 1 {
		t.Errorf("persistent.db has %v, want subtitle_text", rows)
	}
}

// cache.db keeps each job given to a node by its attempt token, so the node's word resolves it after
// a restart, and forgets it once resolved (D234 §7).
func TestOpenCache_KeepsTheAttemptsGivenToNodesUntilForgotten(t *testing.T) {
	t.Parallel()
	cache := openCacheAt(t, filepath.Join(t.TempDir(), "cache.db"))
	err := cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		return queries.InsertNodeAttempt(t.Context(), cachegen.InsertNodeAttemptParams{
			AttemptToken: "a7", JobID: 7, JobKind: "prepare", NodeID: 1, WorkJSON: `{"type":"prepare"}`, SentAt: 5,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := cache.Read().ListNodeAttempts(t.Context())
	if err != nil || len(kept) != 1 || kept[0].AttemptToken != "a7" || kept[0].JobID != 7 || kept[0].NodeID != 1 {
		t.Fatalf("cache.db keeps %+v, %v; want job 7's attempt on node 1", kept, err)
	}
	err = cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		return queries.DeleteNodeAttempt(t.Context(), "a7")
	})
	if kept, listErr := cache.Read().ListNodeAttempts(t.Context()); err != nil || listErr != nil || len(kept) != 0 {
		t.Errorf("after forgetting it cache.db keeps %+v, %v %v; want nothing", kept, err, listErr)
	}
}

// The job table takes the kinds that work on the whole server, a backup and a certificate's
// renewal, naming no target (D243 sections 6, 11); and a scan keeps how many files it needs read
// and whether another is queued after it (D236), as the queries name them.
func TestOpenCache_TakesTheWholeServerJobsAndAScansCounts(t *testing.T) {
	t.Parallel()
	cache := openCacheAt(t, filepath.Join(t.TempDir(), "cache.db"))
	err := cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		for _, kind := range []string{"backup", "renew_certificate"} {
			if _, insertErr := queries.InsertJob(t.Context(), cachegen.InsertJobParams{
				Kind: kind, ArgsJSON: "{}", RunnableAt: 1, CreatedAt: 1,
			}); insertErr != nil {
				return insertErr //nolint:wrapcheck // the test reads it as is
			}
		}
		scanID, insertErr := queries.InsertScan(t.Context(), cachegen.InsertScanParams{
			NodeID: 1, RootPath: "/media/films", StartedAt: 1,
		})
		if insertErr != nil {
			return insertErr //nolint:wrapcheck // the test reads it as is
		}
		if listErr := queries.SetScanListed(t.Context(), cachegen.SetScanListedParams{
			ListedAt: new(int64(2)), ListedCount: 10, ToExamineCount: 3, ID: scanID,
		}); listErr != nil {
			return listErr //nolint:wrapcheck // the test reads it as is
		}
		_, againErr := queries.SetScanAgain(t.Context(), cachegen.SetScanAgainParams{NodeID: 1, RootPath: "/media/films"})
		return againErr //nolint:wrapcheck // the test reads it as is
	})
	if err != nil {
		t.Fatalf("a backup's and a renewal's jobs, and a scan's counts: %v", err)
	}
	scans, err := cache.Read().ListScans(t.Context())
	if err != nil || len(scans) != 1 || scans[0].ToExamineCount != 3 || scans[0].IsAnotherScanQueued != 1 {
		t.Errorf("the scan = %+v (%v); want 3 to read and another queued", scans, err)
	}
}
