// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// D392 §2: a full rescan of one film reads its own files again, changed on disk or not, with what is
// new in their folder, and not another file there stored as listed; a full rescan of its library
// reads every file again.
func TestRescan_AFullRescanReadsItsFilesAgainChangedOnDiskOrNot(t *testing.T) {
	t.Parallel()
	bench := newScanBench(t)
	bench.scan([]nodeproto.ScanRecord{
		observedRecord("Arkon (1979)/Arkon.mkv", "arkon"), observedRecord("Arkon (1979)/Trailer.mkv", "trailer"),
		observedRecord("Metropolis (1927)/Metropolis.mkv", "metropolis"),
	}, true)
	ctx := t.Context()
	films, err := bench.database.Read().GetLiveLibraryByName(ctx, "Films")
	if err != nil {
		t.Fatal(err)
	}
	titleID := librarytest.Title(t, bench.database, librarytest.TitleSpec{Library: films.ID})
	versionID := librarytest.Version(t, bench.database, librarytest.VersionSpec{TitleID: titleID})
	arkon := bench.liveFilesByPath()[root+"/Arkon (1979)/Arkon.mkv"]
	err = bench.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return PutInVersion(ctx, queries, arkon.ID, LibraryKindMovie, versionID)
	})
	if err != nil {
		t.Fatal(err)
	}
	nodes := &fakeNodes{id: bench.nodeID, roots: []string{root}}
	admin := NewAdmin(bench.database, bench.clock, nodes, bench.scans, retirementOf(&fakeWork{}))
	if asked, err := admin.ScanWork(ctx, films.ID, titleID); err != nil || asked != 1 {
		t.Fatalf("ScanWork = %d, %v; want the one folder asked for", asked, err)
	}
	folder := []nodeproto.ScanRecord{
		listedDirectory("Arkon (1979)"), listedFile("Arkon (1979)/Arkon.mkv", int64(len("arkon")), 1),
		listedFile("Arkon (1979)/Extended.mkv", 8, 2), listedFile("Arkon (1979)/Trailer.mkv", int64(len("trailer")), 1),
	}
	bench.listedIn("Arkon (1979)", folder...)
	_, files := bench.examine()
	want := []string{"Arkon (1979)/Arkon.mkv", "Arkon (1979)/Extended.mkv"}
	if got := examinedPaths(files); !slices.Equal(got, want) {
		t.Errorf("a full rescan of the film read %v; want %v: its own file again and the new one", got, want)
	}
	bench.examineAs(observedRecord("Arkon (1979)/Arkon.mkv", "arkon"),
		observedRecord("Arkon (1979)/Extended.mkv", "extended"))
	if asked, err := admin.ScanReadingAgain(ctx, films.ID); err != nil || asked != 1 {
		t.Fatalf("ScanReadingAgain = %d, %v; want the root asked for", asked, err)
	}
	bench.list(append(folder, listedDirectory("Metropolis (1927)"),
		listedFile("Metropolis (1927)/Metropolis.mkv", 10, 1))...)
	if scan, isScanning := bench.inProgressOf(); !isScanning || scan.ListedCount != 4 || scan.ToExamineCount != 4 {
		t.Errorf("a full rescan of the library is %+v (%v); want its four files listed and every one read", scan,
			isScanning)
	}
}

// examinedPaths are the paths of the files a batch reads, sorted.
func examinedPaths(files []nodeproto.ExamineFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path.Text)
	}
	slices.Sort(paths)
	return paths
}
