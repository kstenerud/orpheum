// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// Carrying a choice to a file reads the file's own choice in the transaction that carries it, so
// an own choice kept earlier in that transaction stands; read from the reader pool, it would not be
// seen yet, and the carried choice would replace it.
func TestCarryChoice_SeesAnOwnChoiceKeptEarlierInItsTransaction(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	service := &Service{database: database, clock: clocktest.NewFake(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))}
	userID := storetest.InsertUser(t, database, "alice", "admin")
	const fileID = 6
	french := &SubtitleTrack{Language: "fr"}
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		if err := service.keepChoice(t.Context(), queries, userID, fileID, &SubtitleChoice{IsOff: true}); err != nil {
			return err
		}
		return service.carryChoice(t.Context(), queries, userID, fileID, french)
	})
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := service.FilePrefs(t.Context(), userID, fileID)
	if err != nil {
		t.Fatal(err)
	}
	if choice := prefs.SubtitleChoice; choice == nil || !choice.IsOff || choice.IsCarriedFromWork {
		t.Errorf("the file's choice = %+v, want its own choice, off, standing", choice)
	}
}

// A subtitle offset is kept only for a file its own transaction finds playable: one a rescan
// removed in that transaction, before the offset is kept, gets none.
func TestKeepSubtitleOffset_RefusesAFileItsTransactionRemoved(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	service := &Service{database: database, clock: clocktest.NewFake(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))}
	userID := storetest.InsertUser(t, database, "alice", "admin")
	storetest.ExecRaw(t, database.Path(),
		"INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)",
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'mixed', 'nfo', 'manual', 0, 1)`,
		`INSERT INTO file (id, library_id, node_id, path, reported_path_bytes, size_bytes, payload_hash, filing,
		 probe_json, first_seen_at, last_seen_at)
		 VALUES (2, 1, 1, '/media/Film.mkv', CAST('/media/Film.mkv' AS BLOB), 10, X'0c0d', 'listed', '{}', 1, 1)`)
	const fileID, offsetMs = 2, 500
	removedAt := int64(2)
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		removal := sqlcgen.TombstoneFileParams{RemovedAt: &removedAt, ID: fileID}
		if err := queries.TombstoneFile(t.Context(), removal); err != nil {
			return err
		}
		return service.keepSubtitleOffset(t.Context(), queries, userID, fileID, offsetMs)
	})
	if !errors.Is(err, ErrNoFile) {
		t.Errorf("keeping an offset for a file removed in the same transaction: %v, want ErrNoFile", err)
	}
	// That transaction rolled back, so the file is live again, and an offset for it is kept.
	if err := service.SetSubtitleOffset(t.Context(), userID, fileID, offsetMs); err != nil {
		t.Fatal(err)
	}
	if prefs, err := service.FilePrefs(t.Context(), userID, fileID); err != nil || prefs.SubtitleOffsetMs != offsetMs {
		t.Errorf("the live file's offset = %+v, %v; want %d", prefs, err, offsetMs)
	}
}
