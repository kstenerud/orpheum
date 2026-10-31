// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// D357 §2: a rescan of one film scans the folder its files are in as a manual scan scans: a file
// there unchanged on disk is listed and not read again, a new one is read, and nothing outside the
// folder is looked at.
func TestRescan_AFileUnchangedOnDiskIsNotProbedAgain(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan([]nodeproto.ScanRecord{
		observedRecord("Arkon (1979)/Arkon.mkv", "arkon"), observedRecord("Metropolis (1995)/Metropolis.mkv", "heat"),
	}, true)
	ctx := t.Context()
	films, err := bench.database.Read().GetLiveLibraryByName(ctx, "Films")
	if err != nil {
		t.Fatal(err)
	}
	// The film the pass filed the file in, written here as identification writes it.
	titleID := librarytest.Title(t, bench.database, librarytest.TitleSpec{Library: films.ID})
	versionID := librarytest.Version(t, bench.database, librarytest.VersionSpec{TitleID: titleID})
	arkon := bench.liveFilesByPath()[root+"/Arkon (1979)/Arkon.mkv"]
	err = bench.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return PutInVersion(ctx, queries, arkon.ID, LibraryKindMovies, versionID)
	})
	if err != nil {
		t.Fatal(err)
	}
	nodes := &fakeNodes{id: bench.nodeID, roots: []string{root}}
	admin := NewAdmin(bench.database, bench.clock, nodes, bench.scans, retirementOf(&fakeWork{}))
	asked, err := admin.ScanWork(ctx, films.ID, titleID)
	if err != nil || asked != 1 {
		t.Fatalf("ScanWork = %d, %v; want the one folder asked for", asked, err)
	}
	bench.listedIn("Arkon (1979)", listedDirectory("Arkon (1979)"),
		listedFile("Arkon (1979)/Arkon.mkv", int64(len("arkon")), 1), listedFile("Arkon (1979)/Extended.mkv", 8, 2))
	_, files := bench.examine()
	if len(files) != 1 || files[0].Path.Text != "Arkon (1979)/Extended.mkv" {
		t.Errorf("the node was asked to read %+v; want the new file alone, not the one unchanged on disk", files)
	}
}
