// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

func TestIsStorageFailure_IsADiskWithNoRoomOrFailingNotAWriteRefusedForWhatItSays(t *testing.T) {
	t.Parallel()
	cache := openCacheAt(t, filepath.Join(t.TempDir(), "cache.db"))
	ctx := t.Context()
	// A database held to the pages it has fails as a full disk does: SQLITE_FULL.
	if _, err := cache.database.writer.ExecContext(ctx, "PRAGMA max_page_count = 1"); err != nil {
		t.Fatal(err)
	}
	var full error
	for index := range 1000 {
		full = cache.Write(ctx, func(queries *cachegen.Queries) error {
			_, insertErr := queries.InsertJob(ctx, cachegen.InsertJobParams{
				Kind: "fetch_metadata", ItemID: new(int64(index + 1)), ArgsJSON: "{}", RunnableAt: 1, CreatedAt: 1,
			})
			if insertErr != nil {
				return fmt.Errorf("insert a job: %w", insertErr)
			}
			return nil
		})
		if full != nil {
			break
		}
	}
	if !IsStorageFailure(full) {
		t.Errorf("a write to a full database: %v; want a storage failure", full)
	}
	refused := cache.Write(ctx, func(queries *cachegen.Queries) error {
		_, insertErr := queries.InsertJob(ctx, cachegen.InsertJobParams{Kind: "not a kind", ArgsJSON: "{}"})
		if insertErr != nil {
			return fmt.Errorf("insert a job: %w", insertErr)
		}
		return nil
	})
	if refused == nil || IsStorageFailure(refused) {
		t.Errorf("a write its CHECK refused: %v; want an error that is not the storage failing", refused)
	}
	for _, failing := range []error{syscall.ENOSPC, syscall.EIO, fmt.Errorf("write: %w", syscall.EDQUOT)} {
		if !IsStorageFailure(failing) {
			t.Errorf("%v: want a storage failure", failing)
		}
	}
	if IsStorageFailure(nil) || IsStorageFailure(errors.New("no such item")) {
		t.Error("nil or an ordinary error was taken for the storage failing")
	}
}
