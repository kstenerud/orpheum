// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// decomposedRoot is a root whose name is not in NFC, as macOS writes names.
const decomposedRoot = "/media/Filme\u0301s"

func TestRelativeToRoot_ComparesTheReportedBytesAndTakesARootOfSlash(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		reportedPath, rootPath, wantRelative string
		wantUnder                            bool
	}{
		{"/media/films/a.mkv", "/media/films", "a.mkv", true},
		{"/media/films/a.mkv", "/media/films/", "a.mkv", true},
		{"/a.mkv", "/", "a.mkv", true},
		{decomposedRoot + "/a.mkv", decomposedRoot, "a.mkv", true},
		{"/media/Films/a.mkv", decomposedRoot, "", false},
		{"/media/filmsx/a.mkv", "/media/films", "", false},
		{"/media/films", "/media/films", "", false},
	} {
		relative, under := RelativeToRoot([]byte(check.reportedPath), check.rootPath)
		if under != check.wantUnder || string(relative) != check.wantRelative {
			t.Errorf("%s under %s: %q, %v; want %q, %v", check.reportedPath, check.rootPath, relative, under,
				check.wantRelative, check.wantUnder)
		}
	}
	root, relative, found := RootHolding([]byte("/media/films/new/a.mkv"), []string{"/media/films", "/media/films/new"})
	if !found || root != "/media/films/new" || string(relative) != "a.mkv" {
		t.Errorf("nested roots: %s, %q, %v; want the deepest", root, relative, found)
	}
}

func TestIngest_UnderARootNotInNFCOrOfSlashAScanMovesRemovesKeepsAndAttaches(t *testing.T) {
	t.Parallel()
	for _, rootPath := range []string{decomposedRoot, "/"} {
		world := newHarnessAt(t, rootPath)
		world.scan([]nodeproto.ScanRecord{
			observedRecord("Locked/e.mkv", "e"), observedRecord("a.mkv", "a"), observedRecord("b.mkv", "b"),
			observedRecord("c.mkv", "c"), sidecarRecord("c.en.srt", nodeproto.SidecarSubtitle, "1\n"),
			observedRecord("d.mkv", "d"),
		}, true)
		textOf := func(relative string) string { return PathText([]byte(rootPath)) + "/" + relative }
		if rootPath == "/" {
			textOf = func(relative string) string { return "/" + relative }
		}
		before := world.liveFilesByPath()
		world.scan([]nodeproto.ScanRecord{
			errorRecord("Locked", nodeproto.ScopeSubtree), observedRecord("Moved/a.mkv", "a"),
			observedRecord("c.mkv", "c"), sidecarRecord("c.en.srt", nodeproto.SidecarSubtitle, "1\n"),
			observedRecord("d.mkv", "d"),
		}, true)
		after := world.liveFilesByPath()
		if moved, found := after[textOf("Moved/a.mkv")]; !found || moved.ID != before[textOf("a.mkv")].ID {
			t.Errorf("root %s: a.mkv moved is %+v, want its row to follow it", rootPath, moved)
		}
		if gone, found := after[textOf("b.mkv")]; found {
			t.Errorf("root %s: b.mkv is %+v, want it removed by the complete scan that did not see it", rootPath, gone)
		}
		if _, found := after[textOf("Locked/e.mkv")]; !found {
			t.Errorf("root %s: Locked/e.mkv was removed, want it kept where the scan could not read", rootPath)
		}
		rangeAfter, rangeBefore := PathRange(rootPath)
		rows, err := world.database.Read().ListLiveFilesForAttaching(t.Context(),
			sqlcgen.ListLiveFilesForAttachingParams{NodeID: world.nodeID, After: rangeAfter, Before: rangeBefore})
		if err != nil {
			t.Fatal(err)
		}
		subtitleIndex := slices.IndexFunc(rows, func(row sqlcgen.ListLiveFilesForAttachingRow) bool {
			return row.Path == textOf("c.en.srt")
		})
		if subtitleIndex < 0 || rows[subtitleIndex].AttachedToFile == nil ||
			*rows[subtitleIndex].AttachedToFile != after[textOf("c.mkv")].ID {
			t.Errorf("root %s: the files are %+v, want c.en.srt attached to c.mkv", rootPath, rows)
		}
	}
}
