// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A video a library's pass already filed that the node then cannot read, or skips, is filed so, with
// its reason, and no kind of library files it as a video any more (D163, D363 §6): the M2
// acceptance's mode-000 file, listed and filed unidentified before its read failed.
func TestIngest_AFiledVideoThatCannotBeReadIsErroredAndNoLongerAKindsVideo(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("locked.mkv", "locked")}, true)
	fileID := world.liveFilesByPath()[root+"/locked.mkv"].ID
	err := world.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return FileAs(t.Context(), queries, fileID, LibraryKindMovie, "unidentified", "no NFO file")
	})
	if err != nil {
		t.Fatal(err)
	}
	world.scan([]nodeproto.ScanRecord{errorRecord("locked.mkv", nodeproto.ScopeEntry)}, true)
	got := storetest.QueryRaw(t, world.database.Path(),
		"SELECT place.filing, place.filing_reason, coalesce(file_filing.filing, '') FROM place "+
			"LEFT JOIN file_filing ON file_filing.file_id = place.file_id WHERE place.id = "+itoa(fileID))
	if want := []string{"errored|permissionDenied|"}; !slices.Equal(got, want) {
		t.Errorf("the unreadable file = %v; want %v: errored with its reason, filed by no kind", got, want)
	}
}
