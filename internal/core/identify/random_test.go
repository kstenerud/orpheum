// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/querytally"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A pass over libraries of names of every shape, a parse failing on some as it may, renamed, removed,
// added to and renumbered from scan to scan, writes nothing when run again, leaves the database's
// invariants holding, and makes no more titles over the library's life, tombstones and all, than
// files at paths it was shown: a fault that made one each pass grows with the passes (D303,
// M10; the owner's NAS, 2026-10-16, where a special made a new title each pass, 80,246 in all).
// Its seeds are fixed, so a failure is the same failure every run.
func TestPass_OverRandomLibrariesAgainWritesNothingAndMakesNoTitleEachPass(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"movies", "shows", "mixed"} {
		for seed := range uint64(4) {
			t.Run(fmt.Sprintf("%s/%d", kind, seed), func(t *testing.T) {
				t.Parallel()
				//nolint:gosec // G404: fixed seeds, so a failure repeats; nothing here is a secret
				randomLibraryWalk(t, kind, rand.New(rand.NewPCG(seed, uint64(len(kind)))))
			})
		}
	}
}

// randomFile is a video of a random library: the path its payload was first seen at, which its
// identity hashes, and where it is now.
type randomFile struct {
	origin, path string
}

// randomLibraryWalk scans a random library through several changes, checking each pass.
func randomLibraryWalk(t *testing.T, kind string, random *rand.Rand) {
	t.Helper()
	lib := newTestLibrary(t, kind, []string{"nfo", "nfo_then_tmdb"}[random.IntN(2)])
	var files []randomFile
	nfos := map[string]string{} // body by path
	pathsShown := map[string]bool{}
	placesShown := map[string]bool{} // each file at each path it was shown at
	add := func() {
		video, nfo := randomPath(random)
		if pathsShown[video] || slices.ContainsFunc(files, func(file randomFile) bool { return file.path == video }) {
			return
		}
		files = append(files, randomFile{origin: video, path: video})
		if nfo != "" {
			nfos[nfoPathOf(video)] = nfo
		}
	}
	for range 12 {
		add()
	}
	const steps = 5
	for step := range steps {
		records := make([]nodeproto.ScanRecord, 0, len(files)+len(nfos))
		for _, file := range files {
			records = append(records, videoMoved(file.origin, file.path))
			pathsShown[file.path] = true
			placesShown[file.origin+"\x00"+file.path] = true
		}
		for _, nfoPath := range slices.Sorted(maps.Keys(nfos)) {
			records = append(records, nfoRecord(nfoPath, nfos[nfoPath]))
		}
		lib.scan(records...)
		checkAgainAfterScan(t, lib, step, filePaths(files), len(placesShown))
		changeRandomly(random, &files, nfos, add)
	}
}

// checkAgainAfterScan runs a library's pass again and checks it wrote nothing, that the invariants
// hold, and that no more titles were made than places files were shown at.
func checkAgainAfterScan(t *testing.T, lib *testLibrary, step int, paths []string, placesShown int) {
	t.Helper()
	before, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	tablesBefore := dumpIdentified(t, lib)
	tallied, tally := querytally.New(t.Context())
	if _, err = lib.pass.IdentifyLibrary(tallied, lib.libraryID); err != nil {
		t.Fatal(err)
	}
	after, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("step %d: a second pass over %v changed %d rows; want none, by %s:\n%s", step,
			paths, after-before, tally, differences(tablesBefore, dumpIdentified(t, lib)))
	}
	violations, err := lib.database.CheckInvariants(t.Context())
	if err != nil || len(violations) > 0 {
		t.Errorf("step %d: invariants %v, %v, over %v", step, violations, err, paths)
	}
	made := storetest.QueryRaw(t, lib.database.Path(), "SELECT count(*) FROM item WHERE kind = 'title'")
	if count, _ := strconv.Atoi(made[0]); count > placesShown { //nolint:errcheck // a count
		t.Errorf("step %d: %d titles made over %d places files were shown at; a title is made for a "+
			"file at a path at most once", step, count, placesShown)
	}
}

// changeRandomly renames a file or two, removes one, adds one or two and renumbers an NFO.
func changeRandomly(random *rand.Rand, files *[]randomFile, nfos map[string]string, add func()) {
	for range random.IntN(3) {
		if len(*files) == 0 {
			break
		}
		index := random.IntN(len(*files))
		renamed, _ := randomPath(random)
		(*files)[index].path = renamed
	}
	if len(*files) > 0 && random.IntN(2) == 0 {
		index := random.IntN(len(*files))
		delete(nfos, nfoPathOf((*files)[index].path))
		*files = slices.Delete(*files, index, index+1)
	}
	for range random.IntN(3) {
		add()
	}
	for _, nfoPath := range slices.Sorted(maps.Keys(nfos)) {
		if random.IntN(4) == 0 {
			nfos[nfoPath] = randomNFO(random)
		}
	}
}

// randomPath is a video path of one of the shapes a library holds, well named or not, and the NFO
// beside it, or "".
func randomPath(random *rand.Rand) (video, nfo string) {
	names := []string{"Metropolis", "Wing", "Greed", "Fièvre", "Variety", "狂った一頁", "Nosferatu", "the-unknown"}
	shows := []string{"Quillwick", "Test Show", "Brackenmere", "Farwander"}
	name, show := names[random.IntN(len(names))], shows[random.IntN(len(shows))]
	year, season, episode := 1970+random.IntN(50), 1+random.IntN(4), 1+random.IntN(9)
	shapes := []string{
		fmt.Sprintf("%s (%d)/%s (%d).mkv", name, year, name, year),
		fmt.Sprintf("%s (%d).mkv", name, year),
		name + ".mkv",
		fmt.Sprintf("%s (%d)/Featurettes/Making Of %d.mkv", name, year, episode),
		fmt.Sprintf("%s (%d)/%s (%d)-trailer.mkv", name, year, name, year),
		fmt.Sprintf("xq_%d-final.mkv", random.IntN(1000)),
		fmt.Sprintf("%s 1080p x264 %d.mkv", name, random.IntN(100)),
		fmt.Sprintf("%s/Season %02d/%s - S%02dE%02d.mkv", show, season, show, season, episode),
		fmt.Sprintf("%s/%s - %dx%d - Its Name.avi", show, show, season, episode),
		fmt.Sprintf("%s/%s - %02d - Its Name.mkv", show, show, episode),
		fmt.Sprintf("%s/Specials/Thing %d.mkv", show, episode),
		fmt.Sprintf("%s/Extras/Bloopers %d.mkv", show, episode),
		fmt.Sprintf("%s/random clip %d.mkv", show, random.IntN(100)),
		fmt.Sprintf("%s/%s.S%02dE%02d.720p/x.mkv", show, show, season, episode),
	}
	video = shapes[random.IntN(len(shapes))]
	if random.IntN(3) == 0 {
		nfo = randomNFO(random)
	}
	return video, nfo
}

// randomNFO is a film's or an episode's NFO, numbered at random.
func randomNFO(random *rand.Rand) string {
	if random.IntN(2) == 0 {
		return fmt.Sprintf(`<movie><title>Film %d</title><year>%d</year></movie>`, random.IntN(5), 1990+random.IntN(5))
	}
	return fmt.Sprintf(
		`<episodedetails><title>Episode</title><season>%d</season><episode>%d</episode></episodedetails>`,
		1+random.IntN(2), 1+random.IntN(5))
}

// nfoPathOf is the NFO named for a video.
func nfoPathOf(video string) string {
	return strings.TrimSuffix(video, path.Ext(video)) + ".nfo"
}

func filePaths(files []randomFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.path)
	}
	return paths
}

// dumpIdentified is every table a pass writes, as text, by query.
func dumpIdentified(t *testing.T, lib *testLibrary) map[string][]string {
	t.Helper()
	dump := map[string][]string{}
	for _, query := range identifiedTables {
		dump[query] = storetest.QueryRaw(t, lib.database.Path(), query)
	}
	return dump
}

// differences names the rows of each table that one dump has and the other does not.
func differences(before, after map[string][]string) string {
	var said strings.Builder
	for _, query := range identifiedTables {
		for _, row := range before[query] {
			if !slices.Contains(after[query], row) {
				fmt.Fprintf(&said, "  %s\n    - %s\n", query, row)
			}
		}
		for _, row := range after[query] {
			if !slices.Contains(before[query], row) {
				fmt.Fprintf(&said, "  %s\n    + %s\n", query, row)
			}
		}
	}
	return said.String()
}
