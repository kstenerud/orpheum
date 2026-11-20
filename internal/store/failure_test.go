// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	sqlite3 "modernc.org/sqlite/lib"

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

// standInError stands in for the driver's error, which cannot be made outside it, with the code a
// filesystem gives that no test here can mount (a mergerfs pool mounted cache.files=off).
type standInError struct{ code int }

func (failure standInError) Error() string { return fmt.Sprintf("disk I/O error (%d)", failure.code) }
func (failure standInError) Code() int     { return failure.code }

func TestOpen_SaysPlainlyAFilesystemThatRefusesTheSharedMemoryMap(t *testing.T) {
	t.Parallel()
	refused := fmt.Errorf("database %q: read PRAGMA journal_mode: %w", "persistent.db",
		standInError{code: sqlite3.SQLITE_IOERR_SHMMAP})
	explained := explainOpenFailure(refused)
	for _, says := range []string{
		"the data folder's filesystem refuses the shared memory map", "mergerfs pool mounted cache.files=off",
		"PATH_TO_APPDATA", "cache.files=partial", refused.Error(),
	} {
		if !strings.Contains(explained.Error(), says) {
			t.Errorf("a refused shared memory map: %q; want it to say %q", explained, says)
		}
	}
	if !errors.Is(explained, refused) {
		t.Errorf("a refused shared memory map: %v; want the driver's error kept wrapped", explained)
	}
	// Another I/O error stays as it is.
	writeFailed := error(standInError{code: sqlite3.SQLITE_IOERR_WRITE})
	if got := explainOpenFailure(writeFailed); got.Error() != writeFailed.Error() {
		t.Errorf("another I/O error: %v; want it as it is", got)
	}
	// The driver's own error is matched by its code too: a file that is no database is refused with
	// SQLITE_NOTADB, read by the same method, and said as it is.
	path := filepath.Join(t.TempDir(), "persistent.db")
	if err := os.WriteFile(path, bytes.Repeat([]byte("not a database "), 512), 0o600); err != nil {
		t.Fatal(err)
	}
	_, notADatabase := Open(t.Context(), path, testOpenOptions())
	failure, isCoded := errors.AsType[codedFailure](notADatabase)
	if !isCoded || failure.Code()&0xff != sqlite3.SQLITE_NOTADB {
		t.Fatalf("Open(a file that is no database): %v; want the driver's SQLITE_NOTADB", notADatabase)
	}
	if strings.Contains(notADatabase.Error(), sharedMemoryRefusedAdvice) {
		t.Errorf("Open(a file that is no database): %v; want no word of the shared memory map", notADatabase)
	}
}
