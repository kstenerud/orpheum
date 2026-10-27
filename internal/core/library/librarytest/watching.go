// SPDX-License-Identifier: AGPL-3.0-or-later

package librarytest

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// startingFinishedAfterPercent is finished_after_percent's starting value, which a test's seeded
// settings hold: a position past it of a known length finishes the file.
const startingFinishedAfterPercent = 90

// WatchSpec is where a person stands in a file, as their player's report left it: the position, and
// the moment of watching it is for.
type WatchSpec struct {
	UserID     int64 // defaults to 1
	FileID     int64 // required
	PositionMs int64
	ReadAt     int64 // the position's moment of watching, its position_read_at
}

// Watch puts a person at a position in a file as a play report of it leaves them (D370 §1): the
// position, whatever was stored, and, once it is past finished_after_percent's starting 90% of the
// file's known length, the titles the file is filed as watched from ReadAt, through the store's own
// MarkWatchedByPlay. A later position short of it leaves them watched, as a rewatch does.
func Watch(t *testing.T, database *store.Database, spec WatchSpec) {
	t.Helper()
	userID := defaultInt(spec.UserID, 1)
	storetest.ExecRaw(t, database.Path(), fmt.Sprintf(
		`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at, position_read_at)
		 VALUES (%d, %d, %d, 1, %d, %d)
		 ON CONFLICT (user_id, file_id) DO UPDATE SET position_ms = excluded.position_ms,
		   position_read_at = excluded.position_read_at`,
		userID, spec.FileID, spec.PositionMs, spec.ReadAt, spec.ReadAt))
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		durationMs, err := store.OrNotFound(queries.GetFileDuration(t.Context(), spec.FileID))
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		if durationMs <= 0 || spec.PositionMs*100 <= startingFinishedAfterPercent*durationMs {
			return nil // unknown length, or not finished
		}
		return queries.MarkWatchedByPlay(t.Context(), sqlcgen.MarkWatchedByPlayParams{
			UserID: userID, WatchedAt: spec.ReadAt, FileID: spec.FileID,
		})
	})
	if err != nil {
		t.Fatalf("watch file %d: %v", spec.FileID, err)
	}
}
