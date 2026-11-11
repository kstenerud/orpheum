// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// alsoOver makes another library of kind over folder on the test library's node, and returns it.
func (lib *testLibrary) alsoOver(name, kind, folder string) int64 {
	lib.t.Helper()
	return librarytest.LibraryOn(lib.t, lib.database, lib.nodeID,
		librarytest.Library{Name: name, Kind: kind, MetadataSource: "nfo", Root: folder})
}

// passOf runs libraryID's identification pass.
func (lib *testLibrary) passOf(libraryID int64) {
	lib.t.Helper()
	if _, err := lib.pass.IdentifyLibrary(lib.t.Context(), libraryID); err != nil {
		lib.t.Fatal(err)
	}
}

// filings are how each kind of library filed the live videos: kind, filing, the title's kind and its
// name, one per filing.
func (lib *testLibrary) filings() []string {
	lib.t.Helper()
	return storetest.QueryRaw(lib.t, lib.database.Path(),
		`SELECT file_filing.library_kind, file_filing.filing, work.kind, coalesce(item_text.name, '')
		 FROM file_filing JOIN place ON place.id = file_filing.file_id AND place.removed_at IS NULL
		 LEFT JOIN version ON version.id = file_filing.version_id
		 LEFT JOIN work ON work.id = version.work_id
		 LEFT JOIN item_text ON item_text.item_id = work.id AND item_text.is_original = 1
		 ORDER BY place.path, file_filing.library_kind`)
}

const metropolisNFO = `<movie><title>Metropolis</title><year>1927</year></movie>`

// D363 §5: a file two libraries of the same kind cover is filed once, by the library whose folder is
// outermost, and both list that one title; a library of another kind over it files it as its own
// item, so a film is also an other video.
func TestPass_OneKindOfLibraryFilesAFileOnceAndAnotherKindGivesItsOwnItem(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo")
	inner := lib.alsoOver("Metropolis alone", "movie", libraryRoot+"/Metropolis (1927)")
	other := lib.alsoOver("Everything", "other", libraryRoot)
	lib.scan(nfoRecord("Metropolis (1927)/Metropolis (1927).nfo", metropolisNFO),
		videoRecord("Metropolis (1927)/Metropolis (1927).mkv"))
	lib.passOf(inner)
	lib.passOf(other)
	want := []string{"movie|listed|movie|Metropolis", "other|listed|other|Metropolis"}
	if got := lib.filings(); !slices.Equal(got, want) {
		t.Errorf("the filings = %v; want the film once and the other video once: %v", got, want)
	}
	titles := storetest.QueryRaw(t, lib.database.Path(),
		"SELECT count(*) FROM work WHERE kind = 'movie'")
	if !slices.Equal(titles, []string{"1"}) {
		t.Errorf("films = %v; want one, which both films libraries list", titles)
	}
	for _, libraryID := range []int64{lib.libraryID, inner} {
		rows, err := lib.database.Read().ListFileWorks(t.Context(), libraryID)
		if err != nil || len(rows) != 1 || titleName(rows[0]) != "Metropolis" {
			t.Errorf("library %d lists %+v, %v; want Metropolis", libraryID, rows, err)
		}
	}
}

// D363 §2, §8, §9: a file at two places whose NFO files disagree is named by the one beside the place
// first found; the doctor lists the file at both places, and the item whose NFO files disagree.
func TestPass_TheNFOBesideTheFirstPlaceSpeaksForAFileAtTwo(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "nfo")
	first, copied := "A/Metropolis (1927).mkv", "Z/Metropolis (1927).mkv" // the copy read after, by path too
	lib.scan(nfoRecord("A/Metropolis (1927).nfo", metropolisNFO), videoRecord(first))
	lib.clock.Advance(time.Minute)
	all := librarytest.Scan(nodeproto.ScanBegin{RootPath: lib.rootPath}, []nodeproto.ScanRecord{
		nfoRecord("A/Metropolis (1927).nfo", metropolisNFO), videoRecord(first),
		nfoRecord("Z/Metropolis (1927).nfo", `<movie><title>Metropolis Wave</title><year>1927</year></movie>`),
		videoMoved(first, copied),
	}, true)
	if err := lib.ingester.IngestScan(t.Context(), lib.nodeID, lib.rootPath, all); err != nil {
		t.Fatal(err)
	}
	paths := lib.livePlaces()
	firstID, copyID := paths[libraryRoot+"/"+first], paths[libraryRoot+"/"+copied]
	storetest.ExecRaw(t, lib.database.Path(),
		"DELETE FROM file_filing WHERE file_id = "+strconv.FormatInt(copyID, 10),
		"UPDATE place SET file_id = "+strconv.FormatInt(firstID, 10)+" WHERE id = "+strconv.FormatInt(copyID, 10),
		"DELETE FROM stream WHERE video_file = "+strconv.FormatInt(copyID, 10),
		"DELETE FROM file_probe WHERE file_id = "+strconv.FormatInt(copyID, 10),
		"DELETE FROM whole_file_hash WHERE file_id = "+strconv.FormatInt(copyID, 10),
		"DELETE FROM file WHERE id = "+strconv.FormatInt(copyID, 10))
	lib.passOf(lib.libraryID)
	if got, want := lib.filings(), []string{"movie|listed|movie|Metropolis"}; !slices.Equal(got, want) {
		t.Errorf("the filings = %v; want the file named by the NFO beside its first place: %v", got, want)
	}
	admin := library.NewAdmin(lib.database, lib.clock, nil, nil, library.Retirement{Work: noWork{}})
	atPlaces, isMore, err := admin.FilesAtSeveralPlaces(t.Context())
	wantPaths := []string{libraryRoot + "/" + first, libraryRoot + "/" + copied}
	if err != nil || isMore || len(atPlaces) != 1 || !slices.Equal(atPlaces[0].Paths, wantPaths) {
		t.Errorf("the files at several places = %+v, %v, %v; want the file at %v", atPlaces, isMore, err, wantPaths)
	}
	disagreeing, err := admin.ItemsWhoseNFOsDisagree(t.Context())
	if err != nil || len(disagreeing) != 1 || disagreeing[0].Name != "Metropolis" ||
		!slices.Equal(disagreeing[0].Paths, wantPaths) {
		t.Errorf("the items whose NFO files disagree = %+v, %v; want Metropolis, at %v", disagreeing, err, wantPaths)
	}
}

// livePlaces are the live places' ids by path.
func (lib *testLibrary) livePlaces() map[string]int64 {
	lib.t.Helper()
	rows, err := lib.database.Read().ListLiveFilesByNode(lib.t.Context(), lib.nodeID)
	if err != nil {
		lib.t.Fatal(err)
	}
	byPath := map[string]int64{}
	for _, row := range rows {
		byPath[row.Path] = row.ID
	}
	return byPath
}
