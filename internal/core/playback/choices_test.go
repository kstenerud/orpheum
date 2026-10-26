// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A subtitle offset is kept only for a file its own transaction finds playable: one a rescan
// removed in that transaction, before the offset is kept, gets none.
func TestKeepSubtitleOffset_RefusesAFileItsTransactionRemoved(t *testing.T) {
	t.Parallel()
	database := storetest.OpenMigrated(t)
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	service := &Service{database: database, clock: fakeClock, preferences: preferences.NewService(database, fakeClock)}
	userID := storetest.InsertUser(t, database, "alice", "admin")
	nodeID, libraryID := librarytest.NodeWithLibrary(t, database,
		librarytest.Library{Name: "Films", Kind: "mixed", MetadataSource: "nfo", Root: "/media"})
	librarytest.File(t, database, librarytest.FileSpec{
		ID: 2, LibraryID: libraryID, NodeID: nodeID, Path: "/media/Film.mkv", SizeBytes: 10,
		PayloadHash: []byte{0x0c, 0x0d}, Filing: "listed",
	})
	librarytest.Probe(t, database, librarytest.ProbeSpec{FileID: 2})
	const fileID, offsetMs = 2, 500
	removedAt := int64(2)
	err := database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		removal := sqlcgen.TombstoneFileParams{RemovedAt: &removedAt, ID: fileID}
		if err := queries.TombstoneFile(t.Context(), removal); err != nil {
			return err
		}
		return service.keepSubtitleOffset(t.Context(), queries, userID, fileID, offsetMs)
	})
	if !errors.Is(err, library.ErrNoFile) {
		t.Errorf("keeping an offset for a file removed in the same transaction: %v, want library.ErrNoFile", err)
	}
	// That transaction rolled back, so the file is live again, and an offset for it is kept.
	if err = service.SetSubtitleOffset(t.Context(), userID, fileID, offsetMs); err != nil {
		t.Fatal(err)
	}
	prefs, err := service.preferences.ForFile(t.Context(), database.Read(), userID, fileID, preferences.Tracks{})
	if err != nil || prefs.SubtitleOffsetMs != offsetMs {
		t.Errorf("the live file's offset = %+v, %v; want %d", prefs, err, offsetMs)
	}
}
