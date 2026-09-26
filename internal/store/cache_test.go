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
			Kind: "fetch", Args: "{}", RunnableAt: 1, CreatedAt: 1,
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

func TestOpenCache_RefusesLibraryDB(t *testing.T) {
	t.Parallel()
	library := openMigrated(t)
	_, err := OpenCache(t.Context(), library.Path(), testOpenOptions())
	if !errors.Is(err, ErrNotOurs) {
		t.Errorf("OpenCache on library.db = %v, want ErrNotOurs, so it is never discarded", err)
	}
}

func TestCacheVersion_IsDeclaredByTheSchema(t *testing.T) {
	t.Parallel()
	if version, err := CacheVersion(); err != nil || version < 1 {
		t.Errorf("CacheVersion() = %d, %v", version, err)
	}
}
