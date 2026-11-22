// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
)

// D535 §4 (amending D357 §1): "Rescan everything" rescans every library, whatever it holds, other
// videos' too.
func TestRescan_EveryLibraryWhateverItHolds(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
	want := []int64{lib.libraryID}
	for index, kind := range []string{"series", "other"} {
		source := "tmdb"
		if kind == "other" {
			source = "nfo"
		}
		want = append(want, librarytest.LibraryOn(t, lib.database, lib.nodeID, librarytest.Library{
			Name: kind, Kind: kind, MetadataSource: source, Root: "/media/" + kind + string(rune('a'+index)),
		}))
	}
	rescan := lib.rescanning()
	started, err := rescan.rescans.RescanEverything(t.Context(), false)
	if err != nil || started != len(want) || !slices.Equal(rescan.scans.libraries, want) {
		t.Errorf("RescanEverything = %d, %v, scanning %v; want %d started, scanning %v", started, err,
			rescan.scans.libraries, len(want), want)
	}
}

// D535 §4: a library of other videos is rescanned, fully too: its files scanned and read again, and,
// its scan over, the rescan ends with nothing matched, other videos never are.
func TestRescan_ALibraryOfOtherVideosIsScannedAndNothingMatched(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "other", "nfo")
	lib.scan(videoRecord("a.mp4"))
	rescan := lib.rescanning()
	isStarted, err := rescan.rescans.RescanLibrary(t.Context(), lib.libraryID, false)
	if err != nil || !isStarted || !slices.Equal(rescan.scans.libraries, []int64{lib.libraryID}) {
		t.Fatalf("RescanLibrary = %v, %v, reading again %v; want the library's files read again", isStarted, err,
			rescan.scans.libraries)
	}
	calls := len(lib.provider.Calls)
	rescan.run()
	rescan.ended(lib.libraryID)
	if len(lib.provider.Calls) != calls {
		t.Errorf("rescanning other videos asked the provider %v; want nothing matched", lib.provider.Calls[calls:])
	}
}

// D535 §3: an episode and an other video are rescanned alone, fully too: their files scanned and read
// again, and nothing matched, an episode being matched through its series; an extra is refused,
// rescanned with what it is an extra of.
func TestRescan_AnEpisodeOrOtherVideoIsScannedAloneAndAnExtraIsRefused(t *testing.T) {
	t.Parallel()
	seriesLib := orbitoramaLibrary(t)
	seriesLib.scanAll(orbitoramaFiles...)
	episode := seriesLib.titlesByPath()[orbitoramaFiles[0]].WorkID
	others := newTestLibrary(t, "other", "nfo")
	others.scan(videoRecord("a.mp4"))
	other := others.titlesByPath()["a.mp4"].WorkID
	for _, rescanned := range []struct {
		lib    *testLibrary
		itemID int64
	}{{seriesLib, episode}, {others, other}} {
		rescan := rescanned.lib.rescanning()
		isStarted, err := rescan.rescans.RescanItem(t.Context(), rescanned.itemID, false)
		if err != nil || !isStarted || !slices.Equal(rescan.scans.works, []int64{rescanned.itemID}) {
			t.Fatalf("RescanItem(%d) = %v, %v, reading again %v; want its own files read again", rescanned.itemID,
				isStarted, err, rescan.scans.works)
		}
		calls := len(rescanned.lib.provider.Calls)
		rescan.run()
		rescan.ended(rescanned.lib.libraryID)
		if len(rescanned.lib.provider.Calls) != calls {
			t.Errorf("rescanning item %d asked the provider %v; want nothing matched", rescanned.itemID,
				rescanned.lib.provider.Calls[calls:])
		}
	}
	films := newTestLibrary(t, "movie", "nfo")
	const making = "Metropolis (1927)/Featurettes/Making Metropolis.mkv"
	films.scan(videoRecord("Metropolis (1927)/Metropolis (1927).mkv"), videoRecord(making))
	extra := films.titlesByPath()[making].WorkID
	_, err := films.rescanning().rescans.RescanItem(t.Context(), extra, false)
	if !errors.Is(err, identify.ErrNotRescannable) {
		t.Errorf("RescanItem of an extra = %v; want it refused, %v", err, identify.ErrNotRescannable)
	}
}

// ended fails unless no rescan of the library is left in progress.
func (rescan *rescanning) ended(libraryID int64) {
	t := rescan.lib.t
	t.Helper()
	progress, err := rescan.rescans.Progress(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if running, isRunning := progress[libraryID]; isRunning {
		t.Errorf("library %d's rescan is still in progress, %+v, after its scan; want it ended", libraryID, running)
	}
}

// D535 §7: a video's page says it is being rescanned from the press until the rescan ends: its own,
// its library's, or, for an episode, its series', given the series.
func TestIsRescanning_FromThePressUntilTheRescanEnds(t *testing.T) {
	t.Parallel()
	films := newTestLibrary(t, "movie", "nfo")
	const filmFile = "Metropolis (1927)/Metropolis (1927).mkv"
	films.scan(videoRecord(filmFile))
	film := films.titlesByPath()[filmFile].WorkID
	rescan := films.rescanning()
	isRescanning := func(itemID int64, seriesID *int64) bool {
		t.Helper()
		is, err := rescan.rescans.IsRescanning(t.Context(), itemID, seriesID)
		if err != nil {
			t.Fatalf("IsRescanning(%d): %v", itemID, err)
		}
		return is
	}
	if isRescanning(film, nil) {
		t.Errorf("the film is said rescanned before any rescan; want not")
	}
	if isStarted, err := rescan.rescans.RescanItem(t.Context(), film, false); err != nil || !isStarted {
		t.Fatalf("RescanItem = %v, %v; want it started", isStarted, err)
	}
	if !isRescanning(film, nil) {
		t.Errorf("the film is not said rescanned after its own rescan's press; want it")
	}
	rescan.run()
	rescan.ended(films.libraryID)
	if isRescanning(film, nil) {
		t.Errorf("the film is said rescanned after its rescan ended; want not")
	}
	rescan.library()
	if !isRescanning(film, nil) {
		t.Errorf("the film is not said rescanned while its library's rescan runs; want it")
	}

	seriesLib := orbitoramaLibrary(t)
	seriesLib.scanAll(orbitoramaFiles...)
	episode := seriesLib.titlesByPath()[orbitoramaFiles[0]].WorkID
	seriesID := seriesLib.seriesIDOf(episode)
	seriesRescan := seriesLib.rescanning()
	if isStarted, err := seriesRescan.rescans.RescanItem(t.Context(), seriesID, false); err != nil || !isStarted {
		t.Fatalf("RescanItem of the series = %v, %v; want it started", isStarted, err)
	}
	if is, err := seriesRescan.rescans.IsRescanning(t.Context(), episode, &seriesID); err != nil || !is {
		t.Errorf("an episode given its series is said rescanned %v, %v, while the series' runs; want it", is, err)
	}
	if is, err := seriesRescan.rescans.IsRescanning(t.Context(), episode, nil); err != nil || is {
		t.Errorf("an episode not given its series is said rescanned %v, %v; want only its own rescan seen", is, err)
	}
}
