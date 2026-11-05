// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A repointed file keeps its modified time and is marked unconfirmed instead, so its film's found
// time does not move, and the path it was identified from moves with it, so identification does not
// take it for a file renamed (D432 §6, D208 §1). The next listing reads it again though its size and
// modified time are as stored, and the scan's read confirms it.
func TestRepointRoot_AMovedFileIsUnconfirmedAndKeepsItsIdentification(t *testing.T) {
	t.Parallel()
	world := newHarness(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("Arkon (1979)/Arkon.mkv", "arkon")}, true)
	ctx := t.Context()
	titleID := librarytest.Title(t, world.database, librarytest.TitleSpec{Library: world.libraryID})
	versionID := librarytest.Version(t, world.database, librarytest.VersionSpec{TitleID: titleID})
	arkon := world.liveFilesByPath()[root+"/Arkon (1979)/Arkon.mkv"]
	err := world.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return PutInVersion(ctx, queries, arkon.ID, LibraryKindMovie, versionID)
	})
	if err != nil {
		t.Fatal(err)
	}
	path := world.database.Path()
	found := storetest.QueryRaw(t, path, "SELECT found_at, found_file_modified_at FROM item WHERE id = "+itoa(titleID))
	nodes := &fakeNodes{id: world.nodeID, roots: []string{"/media"}}
	admin := NewAdmin(world.database, world.clock, nodes, nodes, retirementOf(&fakeWork{}))
	if _, err = admin.RepointRoot(ctx, Repoint{
		LibraryID: world.libraryID, NodeName: "local", Path: root, NewNodeName: "local", NewPath: "/media/moved",
	}); err != nil {
		t.Fatal(err)
	}
	const moved = "/media/moved/Arkon (1979)/Arkon.mkv"
	checks := []struct{ what, query, want string }{
		{"the place", "SELECT path, modified_at, is_unconfirmed FROM place", moved + "|1|1"},
		{"the path identified from", "SELECT identified_from_path FROM file_filing", moved},
		{
			"the film's found time", "SELECT found_at, found_file_modified_at FROM item WHERE id = " + itoa(titleID),
			found[0],
		},
	}
	for _, check := range checks {
		if got := storetest.QueryRaw(t, path, check.query); !slices.Equal(got, []string{check.want}) {
			t.Errorf("%s after the repoint = %v; want %q", check.what, got, check.want)
		}
	}
	world.rootPath = "/media/moved"
	taken := world.takeListing([]nodeproto.ScanRecord{
		listedDirectory("Arkon (1979)"), listedFile("Arkon (1979)/Arkon.mkv", int64(len("arkon")), 1),
	}, true)
	if !slices.ContainsFunc(taken.ToExamine, func(file nodeproto.ExamineFile) bool {
		return file.Path.Text == "Arkon (1979)/Arkon.mkv"
	}) {
		t.Errorf("to examine = %+v; want the unconfirmed file read again, though listed as stored", taken.ToExamine)
	}
	world.scan([]nodeproto.ScanRecord{observedRecord("Arkon (1979)/Arkon.mkv", "arkon")}, true)
	if got := storetest.QueryRaw(t, path, "SELECT is_unconfirmed FROM place"); !slices.Equal(got, []string{"0"}) {
		t.Errorf("the place after the scan read it = %v; want it confirmed", got)
	}
}
