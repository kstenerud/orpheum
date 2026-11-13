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
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/querytally"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// A pass over libraries of names of every shape, a parse failing on some as it may, renamed, removed,
// added to and renumbered from scan to scan, writes nothing when run again, leaves the database's
// invariants holding, and makes no more titles over the library's life, tombstones and all, than
// files at paths it was shown: a fault that made one each pass grows with the passes (D303,
// M10; the owner's NAS, 2026-10-16, where a special made a new title each pass, 80,246 in all).
// Filed with each unit its own batch, the library is the same as filed whole (D304 §2).
// A library that asks TMDB has its films and series matched to responses that credit people in
// common, naming and picturing them differently and fetched at differing times, and still settles: a
// person is named by the newest (staging, 2026-11-13, where two films traded a picture every pass).
// Its seeds are fixed, so a failure is the same failure every run.
func TestPass_OverRandomLibrariesAgainWritesNothingAndMakesNoTitleEachPass(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"movie", "series", "other"} {
		for seed := range uint64(4) {
			t.Run(fmt.Sprintf("%s/%d", kind, seed), func(t *testing.T) {
				t.Parallel()
				//nolint:gosec // G404: fixed seeds, so a failure repeats; nothing here is a secret
				randomLibraryWalk(t, kind, rand.New(rand.NewPCG(seed, uint64(len(kind)))),
					rand.New(rand.NewPCG(seed, uint64(len(kind))+100))) //nolint:gosec // as above
			})
		}
	}
}

// randomFile is a video of a random library: the path its payload was first seen at, which its
// identity hashes, and where it is now.
type randomFile struct {
	origin, path string
}

// randomLibraryWalk scans a random library through several changes, checking each pass; one that
// asks TMDB is answered by responses drawn from answers.
func randomLibraryWalk(t *testing.T, kind string, random, answers *rand.Rand) {
	t.Helper()
	source := []string{"nfo", "nfo_then_tmdb"}[random.IntN(2)]
	if kind == "other" {
		source = "nfo" // an other library reads NFO files alone (D349 section 10)
	}
	// The same library twice: one filed whole, one with each unit its own batch (D304 §2).
	lib, batched := newTestLibrary(t, kind, source), newTestLibrary(t, kind, source)
	identify.BatchRows(batched.pass, 1)
	asksTMDB := source == "nfo_then_tmdb"
	if asksTMDB {
		answerWithPeopleInCommon(answers, kind, lib, batched)
	}
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
		awaiting, batchedAwaiting := lib.scan(records...).AwaitingMatch, batched.scan(records...).AwaitingMatch
		if asksTMDB {
			lib.matchAwaiting(awaiting)
			batched.matchAwaiting(batchedAwaiting)
		}
		checkAgainAfterScan(t, lib, step, filePaths(files), len(placesShown))
		checkAgainAfterScan(t, batched, step, filePaths(files), len(placesShown))
		if whole, inBatches := dumpIdentified(t, lib), dumpIdentified(t, batched); !maps.EqualFunc(
			whole, inBatches, slices.Equal) {
			t.Errorf("step %d: filed in batches, the library differs from filed whole:\n%s", step,
				differences(whole, inBatches))
		}
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
	made := storetest.QueryRaw(t, lib.database.Path(),
		"SELECT count(*) FROM item WHERE kind IN ('movie', 'episode', 'extra', 'other')")
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

// The names of a random library's films and series, and the people their TMDB responses credit.
var (
	randomFilmNames = []string{
		"Metropolis", "Arkon", "Greed", "Fièvre", "Théo", "狂った一頁", "Nosferatu", "The Unknown",
	}
	randomShowNames = []string{"Grey Wolf", "Test Show", "Grey Comet", "Skyreach"}
	randomPeople    = []string{"Oren Talisk", "Ana Kessel", "Wend Halvors", "Ilse Marrow", "Tobin Achter"}
)

// The numbers a random library's paths and NFOs are drawn from.
const (
	randomNFOFilms   = 5    // an NFO's film is "Film <n>", n below this
	randomFirstYear  = 1970 // a path's year is this or one of the years after it
	randomPathYears  = 50   // how many years a path's may be
	randomNFOYear    = 1990 // an NFO's year is this or one of the 4 after
	randomShowSeason = 4    // a path's season is at most this
	randomEpisodes   = 9    // a path's episode is at most this
)

// randomPath is a video path of one of the shapes a library holds, well named or not, and the NFO
// beside it, or "".
func randomPath(random *rand.Rand) (video, nfo string) {
	names, shows := randomFilmNames, randomShowNames
	name, show := names[random.IntN(len(names))], shows[random.IntN(len(shows))]
	year := randomFirstYear + random.IntN(randomPathYears)
	season, episode := 1+random.IntN(randomShowSeason), 1+random.IntN(randomEpisodes)
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
		return fmt.Sprintf(`<movie><title>Film %d</title><year>%d</year></movie>`,
			random.IntN(randomNFOFilms), randomNFOYear+random.IntN(5))
	}
	return fmt.Sprintf(
		`<episodedetails><title>Episode</title><season>%d</season><episode>%d</episode></episodedetails>`,
		1+random.IntN(2), 1+random.IntN(5))
}

// answerWithPeopleInCommon makes each library's TMDB answer every name its films, or its series, can
// be searched by, each response crediting some of the same people as it spells and pictures them:
// one person is named and pictured differently from response to response.
func answerWithPeopleInCommon(answers *rand.Rand, kind string, libs ...*testLibrary) {
	if kind == "series" {
		answerSeries(answers, libs)
		return
	}
	years := []int{0}
	for year := randomFirstYear; year < randomFirstYear+randomPathYears; year++ {
		years = append(years, year)
	}
	films := slices.Clone(randomFilmNames)
	for film := range randomNFOFilms {
		films = append(films, fmt.Sprintf("Film %d", film))
	}
	nextID := 5000
	for _, name := range films {
		for _, year := range years {
			nextID++
			answerWith(libs, identifytest.SearchKey(name, year), identify.Metadata{
				ID: strconv.Itoa(nextID), Name: name, Year: year, Credits: creditsInCommon(answers),
			}, nil)
		}
	}
}

// answerSeries is answerWithPeopleInCommon for a library of series: each show a path names, with
// every season and episode a path may.
func answerSeries(answers *rand.Rand, libs []*testLibrary) {
	for index, show := range randomShowNames {
		id := strconv.Itoa(6000 + index)
		seasons := map[string][]identify.Episode{}
		numbers := make([]int, 0, randomShowSeason)
		for season := 1; season <= randomShowSeason; season++ {
			numbers = append(numbers, season)
			listing := fmt.Sprintf("%s/%d", id, season)
			for episode := 1; episode <= randomEpisodes; episode++ {
				seasons[listing] = append(seasons[listing], identify.Episode{
					ID: fmt.Sprintf("%s%02d%02d", id, season, episode), Season: season, Number: episode,
					Name: "Its Name",
				})
			}
		}
		answerWith(libs, identifytest.SearchKey(show, 0), identify.Metadata{
			ID: id, Name: show, SeasonNumbers: numbers, Credits: creditsInCommon(answers),
		}, seasons)
	}
}

// answerWith makes each library's TMDB answer a search by key with metadata, and its seasons.
func answerWith(libs []*testLibrary, key string, metadata identify.Metadata, seasons map[string][]identify.Episode) {
	for _, lib := range libs {
		lib.provider.Searches[key] = []identify.Candidate{lib.provider.Add(metadata)}
		maps.Copy(lib.provider.Seasons, seasons)
	}
}

// creditsInCommon is one response's credits: some of randomPeople, each spelled and pictured as
// this response has them, or not pictured at all.
func creditsInCommon(answers *rand.Rand) []identify.Credit {
	var credits []identify.Credit
	for index, name := range randomPeople {
		if answers.IntN(2) == 0 {
			continue
		}
		if answers.IntN(3) == 0 {
			name += "e" // as this response spells them
		}
		picture := fmt.Sprintf("/person-%d-%d.jpg", index, answers.IntN(3))
		if answers.IntN(4) == 0 {
			picture = "" // this response pictures them not at all
		}
		credits = append(credits, identify.Credit{
			Role: identify.RoleCast, Name: name, PersonID: strconv.Itoa(100 + index), Character: "Someone",
			PictureRef: picture,
		})
	}
	return credits
}

// matchAwaiting matches what a pass left awaiting its match, now and then a minute or two apart, so
// the responses crediting people in common are of differing ages, some the same, and files them.
func (lib *testLibrary) matchAwaiting(awaiting []int64) {
	lib.t.Helper()
	for index, itemID := range awaiting {
		if _, err := lib.matcher.Match(lib.t.Context(), itemID); err != nil {
			lib.t.Fatalf("Match(%d): %v", itemID, err)
		}
		lib.clock.Advance(time.Duration(index%3) * time.Minute)
	}
	lib.rowsAPassWrites()
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
