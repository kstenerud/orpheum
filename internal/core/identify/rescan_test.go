// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// rescanScans is the library admin as a rescan asks it: the scans it asked for, and whether a scan
// of a library is in progress, as the test sets it.
type rescanScans struct {
	libraries []int64
	works     []int64
	// readingAgain are the libraries and items whose scans read their files again.
	librariesReadAgain, worksReadAgain []int64
	scanning                           map[int64]bool
}

func (scans *rescanScans) Scan(_ context.Context, libraryID int64) (int, error) {
	scans.libraries = append(scans.libraries, libraryID)
	return 1, nil
}

func (scans *rescanScans) ScanReadingAgain(ctx context.Context, libraryID int64) (int, error) {
	scans.librariesReadAgain = append(scans.librariesReadAgain, libraryID)
	return scans.Scan(ctx, libraryID)
}

func (scans *rescanScans) ScanWork(_ context.Context, _, itemID int64, isReadingAgain bool) (int, error) {
	scans.works = append(scans.works, itemID)
	if isReadingAgain {
		scans.worksReadAgain = append(scans.worksReadAgain, itemID)
	}
	return 1, nil
}

func (scans *rescanScans) IsScanning(_ context.Context, libraryID int64) (bool, error) {
	return scans.scanning[libraryID], nil
}

// fetchQueue is the job queue as a rescan queues on it: one open fetch an item, as the queue keeps
// a target's (jobs: TestEnqueue_QueuesATargetOnceWhileItIsOpen).
type fetchQueue struct {
	open []jobs.Request
}

func (queue *fetchQueue) Enqueue(_ context.Context, job jobs.Request) (bool, error) {
	if slices.ContainsFunc(queue.open, func(open jobs.Request) bool { return open.ItemID == job.ItemID }) {
		return false, nil
	}
	queue.open = append(queue.open, job)
	return true, nil
}

// rescanning is a library's rescans, over the test's fakes of the scans and the queue.
type rescanning struct {
	lib     *testLibrary
	rescans *identify.Rescans
	matcher *identify.Matcher
	scans   *rescanScans
	queue   *fetchQueue
}

func (lib *testLibrary) rescanning() *rescanning {
	return lib.rescanningLogged(lib.logger)
}

// rescanningLogged is rescanning with the matcher logging to logger.
func (lib *testLibrary) rescanningLogged(logger *slog.Logger) *rescanning {
	matcher := identify.NewMatcher(lib.database, lib.cache, lib.pass, lib.clock, logger)
	scans, queue := &rescanScans{scanning: map[int64]bool{}}, &fetchQueue{}
	return &rescanning{
		lib: lib, matcher: matcher, scans: scans, queue: queue,
		rescans: identify.NewRescans(matcher, queue, scans),
	}
}

// run is the rescans' sweep, then each fetch it queued, as the fetch_metadata runner runs them, then
// the passes they left owed.
func (rescan *rescanning) run() {
	t := rescan.lib.t
	t.Helper()
	if _, err := rescan.rescans.Advance(t.Context()); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	for len(rescan.queue.open) > 0 {
		job := rescan.queue.open[0]
		rescan.queue.open = rescan.queue.open[1:]
		err := rescan.matcher.Handle(t.Context(), jobs.Job{Kind: job.Kind, Target: job.Target, ArgsJSON: job.ArgsJSON})
		if err != nil {
			t.Fatalf("the fetch of item %d: %v", job.ItemID, err)
		}
	}
	rescan.lib.runOwedPasses()
}

// library rescans the test's library, and fails unless it started.
func (rescan *rescanning) library() {
	t := rescan.lib.t
	t.Helper()
	isStarted, err := rescan.rescans.RescanLibrary(t.Context(), rescan.lib.libraryID, identify.Rescan)
	if err != nil || !isStarted {
		t.Fatalf("RescanLibrary = %v, %v; want it started", isStarted, err)
	}
}

// followed is the id at its provider a file's title follows, and its name.
func (lib *testLibrary) followed(rel string) (id, name string) {
	lib.t.Helper()
	file, found := lib.titlesByPath()[rel]
	if !found {
		lib.t.Fatalf("no file %s", rel)
	}
	title, err := lib.database.Read().GetWorkForMatch(lib.t.Context(), file.WorkID)
	if err != nil {
		lib.t.Fatal(err)
	}
	return ptr.ValueOrZero(title.FollowedSourceID), titleName(file)
}

// crowdAsBeforeTheFix is a library whose "Crowd (1928).mkv" was matched to TMDB's "The
// Crowd" of 1928 by a search that found only it, as the matcher matched staging's before the
// exact name won (ab718755); the search now finds the film named exactly too.
func crowdAsBeforeTheFix(t *testing.T) *testLibrary {
	t.Helper()
	lib := newTestLibrary(t, "movie", "tmdb")
	withArticle := lib.provider.Add(identify.Metadata{
		ID: "32", Name: "The Crowd", OriginalName: "The Crowd", Year: 1928,
	})
	plain := lib.provider.Add(identify.Metadata{ID: "31", Name: "Crowd", OriginalName: "Crowd", Year: 1928})
	lib.provider.Searches[identifytest.SearchKey("Crowd", 1928)] = []identify.Candidate{withArticle}
	lib.scan(videoRecord(crowdFile))
	lib.matchFileTitle(crowdFile)
	if id, name := lib.followed(crowdFile); id != "32" || name != "The Crowd" {
		t.Fatalf("before the rescan: %s %q; want The Crowd (32)", id, name)
	}
	lib.provider.Searches[identifytest.SearchKey("Crowd", 1928)] = []identify.Candidate{withArticle, plain}
	// Searched by the name its match stored, the film named so is all TMDB finds.
	lib.provider.Searches[identifytest.SearchKey("The Crowd", 1928)] = []identify.Candidate{withArticle}
	return lib
}

const crowdFile = "Crowd (1928)/Crowd (1928).mkv"

// D357 §2: a rescan scans its library first, matches nothing while the scan runs, and then matches
// each film again from the name and year its file gives, not the name its old match stored: the
// file's "Crowd", once matched to "The Crowd", is the film named exactly.
func TestRescan_ScansThenRematchesFromTheFilesNameAndYear(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	rescan := lib.rescanning()
	rescan.scans.scanning[lib.libraryID] = true
	rescan.library()
	if !slices.Equal(rescan.scans.libraries, []int64{lib.libraryID}) {
		t.Fatalf("scans asked for: %v; want the library's", rescan.scans.libraries)
	}
	calls := len(lib.provider.Calls)
	rescan.run()
	if len(lib.provider.Calls) != calls {
		t.Fatalf("matched while its scan ran: %v", lib.provider.Calls[calls:])
	}
	rescan.scans.scanning[lib.libraryID] = false
	rescan.run()
	if !slices.Contains(lib.provider.Calls[calls:], "search "+identifytest.SearchKey("Crowd", 1928)) {
		t.Errorf("searched %v; want the file's name and year", lib.provider.Calls[calls:])
	}
	if id, name := lib.followed(crowdFile); id != "31" || name != "Crowd" {
		t.Errorf("after the rescan: %s %q; want Crowd (31)", id, name)
	}
}

// D357 §3: a match a person chose is not searched again, and a field a person edited keeps its value
// over the new match.
func TestRescan_LeavesAPersonsChoiceAndEditedFieldsAlone(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	const chosenFile = "Metropolis (1927).mkv"
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1927)] = []identify.Candidate{
		lib.provider.Add(metropolis),
	}
	lib.provider.Add(identify.Metadata{ID: "950", Name: "Metropolis", Year: 1986})
	lib.scan(videoRecord(crowdFile), videoRecord(chosenFile))
	chosen := lib.titlesByPath()[chosenFile].WorkID
	if err := lib.chooser.Choose(t.Context(), chosen, "tmdb", "950"); err != nil {
		t.Fatal(err)
	}
	crowd := lib.titlesByPath()[crowdFile].WorkID
	person := storetest.InsertUser(t, lib.database, "alice", "admin")
	if err := lib.chooser.SetOverride(t.Context(), crowd, identify.FieldName, "Ours", person); err != nil {
		t.Fatal(err)
	}
	calls := len(lib.provider.Calls)
	rescan := lib.rescanning()
	rescan.library()
	rescan.run()
	if slices.Contains(lib.provider.Calls[calls:], "search "+identifytest.SearchKey("Metropolis", 1927)) {
		t.Errorf("searched for the film a person chose: %v", lib.provider.Calls[calls:])
	}
	if id, _ := lib.followed(chosenFile); id != "950" {
		t.Errorf("the chosen film follows %s; want 950, the person's choice", id)
	}
	if id, name := lib.followed(crowdFile); id != "31" || name != "Ours" {
		t.Errorf("the edited film follows %s, named %q; want 31, matched again, named as the person named it",
			id, name)
	}
}

// D357 §3: a match the matcher guessed and no one confirmed is matched again like any other, and
// one the rules now settle clearly is no longer marked a guess (D301).
func TestRescan_RematchesAnUnconfirmedGuess(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	const rel = "The General (2002).mkv"
	lesser := lib.provider.Add(identify.Metadata{ID: "2", Name: "The General", Year: 2002})
	lesser.Votes = 10
	popular := lib.provider.Add(identify.Metadata{ID: "3", Name: "The General", Year: 2002})
	popular.Votes = 900
	lib.provider.Searches[identifytest.SearchKey("The General", 2002)] = []identify.Candidate{lesser, popular}
	lib.scan(videoRecord(rel))
	lib.matchFileTitle(rel)
	titleID := lib.titlesByPath()[rel].WorkID
	if id, _ := lib.followed(rel); id != "3" || !lib.isGuessed(titleID) {
		t.Fatalf("before: follows %s, guessed %v; want the most voted, guessed", id, lib.isGuessed(titleID))
	}
	lib.provider.Searches[identifytest.SearchKey("The General", 2002)] = []identify.Candidate{lesser}
	rescan := lib.rescanning()
	rescan.library()
	rescan.run()
	if id, _ := lib.followed(rel); id != "2" {
		t.Errorf("after: follows %s; want 2, the search's one answer", id)
	}
	if lib.isGuessed(titleID) {
		t.Error("still marked a guess; want the guess cleared by a match no person confirmed")
	}
}

// D357 §4: a different answer replaces the old one without review: its ids replace the old match's,
// the title takes the new work's name, and an answer that is a guess is marked one.
func TestRescan_ADifferentMatchReplacesTheOld(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	titleID := lib.titlesByPath()[crowdFile].WorkID
	rescan := lib.rescanning()
	rescan.library()
	rescan.run()
	ids, err := lib.database.Read().ListItemProviderIDs(t.Context(), titleID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if id.ProviderID == "32" {
			t.Errorf("the old match's id is kept: %+v", ids)
		}
	}
	if lib.isGuessed(titleID) {
		t.Error("marked a guess; the film named exactly is a clear match")
	}
	// Two of the name in its year, now: the most voted is guessed, and marked so.
	other := lib.provider.Add(identify.Metadata{ID: "33", Name: "Crowd", OriginalName: "Crowd", Year: 1928})
	other.Votes = 900
	lib.provider.Searches[identifytest.SearchKey("Crowd", 1928)] = append(
		lib.provider.Searches[identifytest.SearchKey("Crowd", 1928)], other)
	rescan.library()
	rescan.run()
	if id, _ := lib.followed(crowdFile); id != "33" || !lib.isGuessed(titleID) {
		t.Errorf("follows %s, guessed %v; want 33, the most voted of the two, marked a guess", id,
			lib.isGuessed(titleID))
	}
}

// D357 §5: a second press while a library's rescan runs adds nothing, nor does a press on a film in
// it; once it ends, a press starts another.
func TestRescan_ASecondPressWhileOneRunsAddsNothing(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	rescan := lib.rescanning()
	rescan.scans.scanning[lib.libraryID] = true
	rescan.library()
	isStarted, err := rescan.rescans.RescanLibrary(t.Context(), lib.libraryID, identify.Rescan)
	if err != nil || isStarted {
		t.Errorf("a second press = %v, %v; want nothing started", isStarted, err)
	}
	titleID := lib.titlesByPath()[crowdFile].WorkID
	isStarted, err = rescan.rescans.RescanItem(t.Context(), titleID, identify.Rescan)
	if err != nil || isStarted {
		t.Errorf("a press on its film = %v, %v; want nothing started", isStarted, err)
	}
	if len(rescan.scans.libraries) != 1 || len(rescan.scans.works) != 0 {
		t.Errorf("scans asked for: %v and %v; want the first press's alone", rescan.scans.libraries,
			rescan.scans.works)
	}
	rescan.scans.scanning[lib.libraryID] = false
	rescan.run()
	rescan.library()
}

// D357 §1: "Rescan everything" rescans each library of films or of series, and no other one, whose
// videos are never matched.
func TestRescan_EveryLibraryOfFilmsOrSeriesAndNoOther(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	var want []int64
	want = append(want, lib.libraryID)
	for index, kind := range []string{"series", "other"} {
		source := "tmdb"
		if kind == "other" {
			source = "nfo"
		}
		libraryID := librarytest.LibraryOn(t, lib.database, lib.nodeID, librarytest.Library{
			Name: kind, Kind: kind, MetadataSource: source, Root: "/media/" + kind + string(rune('a'+index)),
		})
		if kind == "series" {
			want = append(want, libraryID)
		}
	}
	rescan := lib.rescanning()
	started, err := rescan.rescans.RescanEverything(t.Context(), identify.Rescan)
	if err != nil || started != len(want) || !slices.Equal(rescan.scans.libraries, want) {
		t.Errorf("RescanEverything = %d, %v, scanning %v; want %d started, scanning %v", started, err,
			rescan.scans.libraries, len(want), want)
	}
}

// D357 §1, §2: "Rescan this" scans the film's own files and matches it alone again.
func TestRescan_OneItemRescansItsFilesAndRematchesIt(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	const otherFile = "Metropolis (1927).mkv"
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1927)] = []identify.Candidate{
		lib.provider.Add(metropolis),
	}
	lib.scan(videoRecord(crowdFile), videoRecord(otherFile))
	lib.matchFileTitle(otherFile)
	titleID := lib.titlesByPath()[crowdFile].WorkID
	rescan := lib.rescanning()
	isStarted, err := rescan.rescans.RescanItem(t.Context(), titleID, identify.Rescan)
	if err != nil || !isStarted {
		t.Fatalf("RescanItem = %v, %v; want it started", isStarted, err)
	}
	if !slices.Equal(rescan.scans.works, []int64{titleID}) || len(rescan.scans.libraries) != 0 {
		t.Errorf("scans asked for: works %v, libraries %v; want the film's files alone", rescan.scans.works,
			rescan.scans.libraries)
	}
	calls := len(lib.provider.Calls)
	rescan.run()
	if id, _ := lib.followed(crowdFile); id != "31" {
		t.Errorf("the film follows %s; want 31", id)
	}
	if slices.Contains(lib.provider.Calls[calls:], "search "+identifytest.SearchKey("Metropolis", 1927)) {
		t.Errorf("searched for the other film too: %v", lib.provider.Calls[calls:])
	}
	seriesLib := orbitoramaLibrary(t)
	seriesLib.scanAll(orbitoramaFiles...)
	episode := seriesLib.titlesByPath()[orbitoramaFiles[0]].WorkID
	if _, err := seriesLib.rescanning().rescans.RescanItem(t.Context(), episode, identify.Rescan); err == nil {
		t.Error("an episode was rescanned alone; want it refused, its series is what is rescanned")
	}
}

// D357 §6: each match a rescan changes is logged with the item, its old match, its new one and
// what settled it, and each run's totals when it ends.
func TestRescan_LogsEachChangedMatchAndTheRunsTotals(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	var logged bytes.Buffer
	rescan := lib.rescanningLogged(slog.New(slog.NewTextHandler(&logged, nil)))
	rescan.library()
	rescan.run()
	titleID := lib.titlesByPath()[crowdFile].WorkID
	for _, want := range []string{
		slogx.EventKey + `=rescanRematched itemId=` + strconv.FormatInt(titleID, 10) +
			` before=32 beforeName="The Crowd" after=31 afterName=Crowd settledBy="the name and the year"` +
			` isGuess=false`,
		slogx.EventKey + `=rescanEnded libraryId=` + strconv.FormatInt(lib.libraryID, 10) + ` itemId=0 itemCount=1` +
			` changedCount=1 unchangedCount=0 leftAloneCount=0 unsettledCount=0 failedCount=0`,
	} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("the log lacks %s:\n%s", want, logged.String())
		}
	}
}

// D357 §1, §4: a series is matched again as the one thing identified, from its folder's name: a
// different answer replaces the old match, the old work's season listings with it, and its episodes
// follow the new work's (D209).
func TestRescan_ASeriesIsMatchedAgainAndItsEpisodesFollow(t *testing.T) {
	t.Parallel()
	lib := orbitoramaLibrary(t)
	remake := lib.provider.Add(identify.Metadata{ID: "700", Name: "Orbitorama", Year: 2023})
	lib.provider.Seasons["700/2"] = []identify.Episode{
		{ID: "7001", Season: 2, Number: 10, Name: "Not Ours", Year: 2023},
	}
	lib.provider.Searches[identifytest.SearchKey("Orbitorama", 0)] = []identify.Candidate{remake}
	lib.scanAll(orbitoramaFiles...)
	series := lib.onlySeries()
	if _, err := lib.matcher.Match(t.Context(), series.ID); err != nil {
		t.Fatal(err)
	}
	lib.runOwedPasses()
	if name := titleName(lib.titlesByPath()[orbitoramaFiles[0]]); name != "Not Ours" {
		t.Fatalf("before the rescan, S02E10 is %q; want the remake's episode", name)
	}
	original := identify.Candidate{ID: orbitorama.ID, Name: orbitorama.Name, Year: orbitorama.Year}
	lib.provider.Searches[identifytest.SearchKey("Orbitorama", 0)] = []identify.Candidate{remake, original}
	rescan := lib.rescanning()
	rescan.library()
	rescan.run()
	if followed := ptr.ValueOrZero(lib.onlySeries().FollowedSourceID); followed != orbitorama.ID {
		t.Errorf("the series follows %s; want %s, the oldest of the name, guessed", followed, orbitorama.ID)
	}
	if !lib.isGuessed(series.ID) {
		t.Error("the series' new match is not marked a guess; two of its name were found and it has no year")
	}
	checkOrbitorama(t, lib, "after the rescan")
}

// A rescan of a library holding an episode its name placed, as a guess, lists its series and queues
// its match: reading what the library's files say writes nothing, so the read completes, rather
// than the rescan waiting forever for the library to stop changing (staging's shows, 2026-11-01).
func TestRescan_ALibraryWithAnEpisodePlacedByItsNameIsMatchedAgain(t *testing.T) {
	t.Parallel()
	lib := placedByNameLibrary(t)
	rescan := lib.rescanning()
	rescan.library()
	queued, err := rescan.rescans.Advance(t.Context())
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if queued != 1 {
		t.Errorf("the rescan queued %d fetches; want its series' one", queued)
	}
}

// A rescan whose library changes each time its files are read waits for it, and its log names the
// write that keeps it changing: a stuck rescan said only that it waited, for hours. Here
// the film's sort key is taken away after each pass, so each read would write it again.
func TestRescan_ALibraryThatKeepsChangingIsLoggedWithTheWriteThatChangesIt(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	titleID := lib.titlesByPath()[crowdFile].WorkID
	takeAway := `DELETE FROM sort_key WHERE item_id = ` + strconv.FormatInt(titleID, 10)
	storetest.ExecRaw(t, lib.database.Path(), takeAway)
	lib.pass.AfterPass(func(context.Context, identify.Report) {
		storetest.ExecRaw(t, lib.database.Path(), takeAway) //nolint:contextcheck // it writes with the test's context
	})
	var logged bytes.Buffer
	rescan := lib.rescanningLogged(slog.New(slog.NewTextHandler(&logged, nil)))
	rescan.library()
	if _, err := rescan.rescans.Advance(t.Context()); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	for _, want := range []string{
		slogx.EventKey + `=rescanWaiting libraryId=` + strconv.FormatInt(lib.libraryID, 10),
		`the write its last read would make: name title ` + strconv.FormatInt(titleID, 10),
	} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("the log lacks %s:\n%s", want, logged.String())
		}
	}
}

// A library that keeps changing however often the sweep reads it is a bug, which waiting does not
// fix (the owner, 2026-11-13): its rescan waits some sweeps, then gives up, ending, with an error
// that names the write that kept changing the library and the item it was filing, and a rescan
// asked for after it starts again.
func TestRescan_ALibraryThatNeverStopsChangingGivesUpWithTheWriteThatChangesIt(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	titleID := lib.titlesByPath()[crowdFile].WorkID
	takeAway := `DELETE FROM sort_key WHERE item_id = ` + strconv.FormatInt(titleID, 10)
	storetest.ExecRaw(t, lib.database.Path(), takeAway)
	lib.pass.AfterPass(func(context.Context, identify.Report) {
		storetest.ExecRaw(t, lib.database.Path(), takeAway) //nolint:contextcheck // it writes with the test's context
	})
	var logged bytes.Buffer
	rescan := lib.rescanningLogged(slog.New(slog.NewTextHandler(&logged, nil)))
	rescan.library()
	gaveUp := slogx.EventKey + `=rescanGaveUp`
	sweeps := 0
	for ; sweeps < 20 && !strings.Contains(logged.String(), gaveUp); sweeps++ {
		if _, err := rescan.rescans.Advance(t.Context()); err != nil {
			t.Fatalf("Advance: %v", err)
		}
	}
	if sweeps < 2 || sweeps == 20 {
		t.Fatalf("the rescan gave up after %d sweeps; want after a few:\n%s", sweeps, logged.String())
	}
	var line string
	for candidate := range strings.Lines(logged.String()) {
		if strings.Contains(candidate, gaveUp) {
			line = candidate
		}
	}
	for _, want := range []string{
		`level=ERROR`, `libraryId=` + strconv.FormatInt(lib.libraryID, 10),
		`the write its last read would make: name title ` + strconv.FormatInt(titleID, 10),
	} {
		if !strings.Contains(line, want) {
			t.Errorf("the line it gave up with lacks %s: %s", want, line)
		}
	}
	progress, err := rescan.rescans.Progress(t.Context())
	if err != nil || len(progress) != 0 {
		t.Errorf("rescans after giving up: %+v, %v; want none", progress, err)
	}
	isStarted, err := rescan.rescans.RescanLibrary(t.Context(), lib.libraryID, identify.Rescan)
	if err != nil || !isStarted {
		t.Errorf("RescanLibrary after giving up = %t, %v; want a new rescan", isStarted, err)
	}
}

// D357 §2: a film whose search now settles on none is left as a first match leaves one, its old
// match forgotten: unidentified, not found, named from its file again.
func TestRescan_AFilmItsSearchNoLongerSettlesIsNamedFromItsFileAgain(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	titleID := lib.titlesByPath()[crowdFile].WorkID
	lib.provider.Searches[identifytest.SearchKey("Crowd", 1928)] = nil
	lib.provider.Searches[identifytest.SearchKey("The Crowd", 1928)] = nil
	rescan := lib.rescanning()
	rescan.library()
	rescan.run()
	file := lib.titlesByPath()[crowdFile]
	if id, name := lib.followed(crowdFile); id != "" || name != "Crowd" ||
		file.Filing != identify.FilingUnidentified {
		t.Errorf("after the rescan: follows %q, named %q, filed %s; want no match, named from its file, unidentified",
			id, name, file.Filing)
	}
	ids, err := lib.database.Read().ListItemProviderIDs(t.Context(), titleID)
	if err != nil || len(ids) != 0 {
		t.Errorf("ids kept: %+v, %v; want the old match's forgotten", ids, err)
	}
}

// D357 §4: what a person watched stays theirs across a changed match. Watching is kept by file
// (play_position, play_span and play_event name the file), and a person's hiding of a film's
// progress by its item (item_hidden); a rescan's new match keeps the film's title and its files in
// it, so its Continue Watching row is the same film's, under its new name.
func TestRescan_WhatAPersonWatchedSurvivesAChangedMatch(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	file := lib.titlesByPath()[crowdFile]
	person := storetest.InsertUser(t, lib.database, "alice", "user")
	storetest.ExecRaw(t, lib.database.Path(),
		`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at, position_read_at)
		 VALUES (`+strconv.FormatInt(person, 10)+`, `+strconv.FormatInt(file.ID, 10)+`, 600000, 1, 1, 1)`,
		`INSERT INTO play_span (user_id, file_id, from_position_ms, to_position_ms, started_at, ended_at)
		 VALUES (`+strconv.FormatInt(person, 10)+`, `+strconv.FormatInt(file.ID, 10)+`, 0, 600000, 1, 2)`)
	rescan := lib.rescanning()
	rescan.library()
	rescan.run()
	if id, _ := lib.followed(crowdFile); id != "31" {
		t.Fatalf("the film follows %s; want 31, its match changed", id)
	}
	watching, err := lib.database.Read().ListRecentPlayPositions(t.Context(), sqlcgen.ListRecentPlayPositionsParams{
		UserID: person, IsEveryLibrary: 1, LibraryIdsJSON: "[]", MaxRows: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(watching) != 1 || watching[0].WorkID != file.WorkID || watching[0].PositionMs != 600000 ||
		ptr.ValueOrZero(watching[0].Name) != "Crowd" {
		t.Errorf("what alice was watching: %+v; want the same film, ten minutes in, named Crowd now", watching)
	}
	spans, err := lib.database.Read().ListPlaySpansOfFile(t.Context(), sqlcgen.ListPlaySpansOfFileParams{
		UserID: person, FileID: file.ID,
	})
	if err != nil || len(spans) != 1 {
		t.Errorf("what alice watched of the file: %+v, %v; want the one span", spans, err)
	}
}

// imagesAskedAgain is the artwork service as a full rescan asks it: the items whose missing images it
// asked for again.
type imagesAskedAgain struct{ itemIDs []int64 }

func (asked *imagesAskedAgain) MarkImagesOwedAgain(_ context.Context, itemIDs ...int64) error {
	asked.itemIDs = append(asked.itemIDs, itemIDs...)
	return nil
}

// fullRescanning is the rescans of a library holding "Crowd (1928).mkv", logging to its buffer,
// with what asks again for its films' images.
func fullRescanning(t *testing.T) (*rescanning, *imagesAskedAgain, *bytes.Buffer) {
	t.Helper()
	lib := crowdAsBeforeTheFix(t)
	var logged bytes.Buffer
	rescan := lib.rescanningLogged(slog.New(slog.NewJSONHandler(&logged, nil)))
	images := &imagesAskedAgain{}
	rescan.matcher.SetImageRetrier(images)
	return rescan, images, &logged
}

// D392 §1, §2, §4, §6: a full rescan of a library has its scan read every file again, changed on disk
// or not, asks again for the images each film it matches again is missing, and matches it again as a
// rescan does; a press of either kind while it runs adds nothing.
func TestRescan_AFullRescanReadsItsFilesAgainAndAsksAgainForMissingImages(t *testing.T) {
	t.Parallel()
	rescan, images, logged := fullRescanning(t)
	lib := rescan.lib
	titleID := lib.titlesByPath()[crowdFile].WorkID
	rescan.scans.scanning[lib.libraryID] = true
	isStarted, err := rescan.rescans.RescanLibrary(t.Context(), lib.libraryID, identify.FullRescan)
	if err != nil || !isStarted || !slices.Equal(rescan.scans.librariesReadAgain, []int64{lib.libraryID}) {
		t.Fatalf("a full rescan = %v, %v, reading again %v; want it started, its library read again", isStarted,
			err, rescan.scans.librariesReadAgain)
	}
	for _, kind := range []identify.RescanKind{identify.FullRescan, identify.Rescan} {
		if isStarted, err := rescan.rescans.RescanLibrary(t.Context(), lib.libraryID, kind); err != nil || isStarted {
			t.Errorf("a second press while it runs = %v, %v; want nothing started", isStarted, err)
		}
	}
	rescan.scans.scanning[lib.libraryID] = false
	rescan.run()
	if !slices.Equal(images.itemIDs, []int64{titleID}) {
		t.Errorf("images asked for again of %v; want the film's, %d", images.itemIDs, titleID)
	}
	if id, _ := lib.followed(crowdFile); id != "31" {
		t.Errorf("the film follows %s after the full rescan; want 31, matched again as a rescan does", id)
	}
	if len(testutil.LinesWith(logged.String(), "rescanEnded")) == 0 || !strings.Contains(logged.String(),
		`"isFull":true`) {
		t.Errorf("the log does not say a full rescan ended:\n%s", logged.String())
	}
}

// D392 §1, §2, §4: a full rescan of one film has its scan read its files again and asks again for its
// missing images; a rescan of it does neither.
func TestRescan_AFullRescanOfOneFilmReadsItsFilesAgain(t *testing.T) {
	t.Parallel()
	rescan, images, _ := fullRescanning(t)
	lib := rescan.lib
	titleID := lib.titlesByPath()[crowdFile].WorkID
	if isStarted, err := rescan.rescans.RescanItem(t.Context(), titleID, identify.FullRescan); err != nil ||
		!isStarted || !slices.Equal(rescan.scans.worksReadAgain, []int64{titleID}) {
		t.Fatalf("a full rescan of the film = %v, %v, reading again %v; want its files read again", isStarted, err,
			rescan.scans.worksReadAgain)
	}
	rescan.run()
	if isStarted, err := rescan.rescans.RescanItem(t.Context(), titleID, identify.Rescan); err != nil || !isStarted {
		t.Fatalf("a rescan of the film = %v, %v; want it started", isStarted, err)
	}
	rescan.run()
	if !slices.Equal(images.itemIDs, []int64{titleID}) || len(rescan.scans.worksReadAgain) != 1 {
		t.Errorf("images asked for again of %v, works read again %v; want the full rescan's alone", images.itemIDs,
			rescan.scans.worksReadAgain)
	}
}

// D357: a fetch reads its item's rematch in cache.db only while a rescan runs, which a rematch is kept
// with: one planted behind the matcher's back while it counts none running is not read, and the fetch
// matches as an ordinary one.
func TestRescan_AFetchReadsNoRematchWhileNoRescanRuns(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	rescan := lib.rescanning()
	titleID := lib.titlesByPath()[crowdFile].WorkID
	fetch := func() {
		t.Helper()
		err := rescan.matcher.Handle(t.Context(),
			jobs.Job{Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: titleID}})
		if err != nil {
			t.Fatalf("the fetch: %v", err)
		}
	}
	fetch()
	err := lib.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		rescanID, err := queries.InsertRescan(t.Context(), cachegen.InsertRescanParams{
			LibraryID: lib.libraryID, RequestedAt: 1,
		})
		if err != nil {
			return fmt.Errorf("plant a rescan: %w", err)
		}
		_, err = queries.InsertRematch(t.Context(), cachegen.InsertRematchParams{
			ItemID: titleID, RescanID: rescanID, Name: "Crowd", Year: 1928, IdsJSON: "{}",
		})
		if err != nil {
			return fmt.Errorf("plant its rematch: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fetch()
	if _, err := lib.cache.Read().GetRematch(t.Context(), titleID); err != nil {
		t.Errorf("the planted rematch: %v; want it unread and so still kept", err)
	}
	if id, _ := lib.followed(crowdFile); id != "32" {
		t.Errorf("the film follows %s; want 32, matched as an ordinary fetch", id)
	}
}

// D357: a rescan a restart finds in cache.db is counted running, so the fetches it queued before the
// restart read their rematches and match again.
func TestRescan_ARescanFoundAtStartIsReadByItsFetches(t *testing.T) {
	t.Parallel()
	lib := crowdAsBeforeTheFix(t)
	before := lib.rescanning()
	before.library()
	if _, err := before.rescans.Advance(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(before.queue.open) == 0 {
		t.Fatal("the rescan queued no fetch")
	}
	restarted := lib.rescanning()
	for _, job := range before.queue.open {
		err := restarted.matcher.Handle(t.Context(),
			jobs.Job{Kind: job.Kind, Target: job.Target, ArgsJSON: job.ArgsJSON})
		if err != nil {
			t.Fatalf("the fetch of item %d: %v", job.ItemID, err)
		}
	}
	lib.runOwedPasses()
	if id, _ := lib.followed(crowdFile); id != "31" {
		t.Errorf("after the restart the film follows %s; want 31, matched again by the rescan", id)
	}
}
