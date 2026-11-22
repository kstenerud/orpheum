// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// chosenAndWatched is a library whose "Metropolis (1927).mkv" a person matched to 950 and alice
// watched ten minutes of, with a search's answers kept for it, rescanned as rescan says.
type chosenAndWatched struct {
	rescan        *rescanning
	images        *imagesAskedAgain
	chosen, alice int64
	callsBefore   int
}

func newChosenAndWatched(t *testing.T) chosenAndWatched {
	t.Helper()
	rescan, images, _ := fullRescanning(t)
	lib := rescan.lib
	const chosenFile = "Metropolis (1927).mkv"
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1927)] = []identify.Candidate{
		lib.provider.Add(metropolis),
	}
	lib.provider.Add(identify.Metadata{ID: "950", Name: "Metropolis", Year: 1986})
	lib.scan(videoRecord(crowdFile), videoRecord(chosenFile))
	file := lib.titlesByPath()[chosenFile]
	if err := lib.chooser.Choose(t.Context(), file.WorkID, "tmdb", "950"); err != nil {
		t.Fatal(err)
	}
	err := lib.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		return queries.InsertMatchCandidate(t.Context(), cachegen.InsertMatchCandidateParams{
			ItemID: file.WorkID, Rank: 0, Provider: "tmdb", ProviderID: "950", Name: new("Metropolis"),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	alice := storetest.InsertUser(t, lib.database, "alice", "user")
	storetest.ExecRaw(t, lib.database.Path(),
		`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at, position_read_at)
		 VALUES (`+strconv.FormatInt(alice, 10)+`, `+strconv.FormatInt(file.ID, 10)+`, 600000, 1, 1, 1)`)
	return chosenAndWatched{
		rescan: rescan, images: images, chosen: file.WorkID, alice: alice, callsBefore: len(lib.provider.Calls),
	}
}

// candidates are the search's answers kept for the chosen film.
func (world chosenAndWatched) candidates(t *testing.T) []cachegen.MatchCandidate {
	t.Helper()
	kept, err := world.rescan.lib.cache.Read().ListMatchCandidates(t.Context(), world.chosen)
	if err != nil {
		t.Fatal(err)
	}
	return kept
}

// wantAPersonsDoingKept fails unless the chosen film still follows the person's choice and alice's
// ten minutes of it are hers still.
func (world chosenAndWatched) wantAPersonsDoingKept(t *testing.T) {
	t.Helper()
	if id, _ := world.rescan.lib.followed("Metropolis (1927).mkv"); id != "950" {
		t.Errorf("the chosen film follows %s; want 950, the person's choice", id)
	}
	watching, err := world.rescan.lib.database.Read().ListRecentPlayPositions(t.Context(),
		sqlcgen.ListRecentPlayPositionsParams{
			UserID: world.alice, IsEveryLibrary: 1, LibraryIdsJSON: "[]", MaxRows: 10,
		})
	if err != nil || len(watching) != 1 || watching[0].WorkID != world.chosen || watching[0].PositionMs != 600000 {
		t.Errorf("what alice was watching: %+v, %v; want the chosen film, ten minutes in", watching, err)
	}
}

// D535 §6: a rescan that discards what was made has its scan throw out what was made of its files,
// throws out the search's answers kept for its films and their images, and fetches each one's record
// again, the one a person chose too; the match a person chose, and what alice watched, stay.
func TestRescan_ADiscardingRescanThrowsOutWhatWasFetchedButKeepsAPersonsDoing(t *testing.T) {
	t.Parallel()
	world := newChosenAndWatched(t)
	lib := world.rescan.lib
	isStarted, err := world.rescan.rescans.RescanLibrary(t.Context(), lib.libraryID, true)
	if err != nil || !isStarted || !slices.Equal(world.rescan.scans.discarding, []int64{lib.libraryID}) {
		t.Fatalf("a discarding rescan = %v, %v, discarding %v; want it started, its library's scan discarding",
			isStarted, err, world.rescan.scans.discarding)
	}
	world.rescan.run()
	if kept := world.candidates(t); len(kept) != 0 {
		t.Errorf("the chosen film's kept search answers are %+v; want them thrown out", kept)
	}
	if !slices.Contains(world.images.discarded, world.chosen) {
		t.Errorf("images discarded of %v; want the chosen film's, %d, among them", world.images.discarded,
			world.chosen)
	}
	if !slices.Contains(lib.provider.Calls[world.callsBefore:], "fetch 950") {
		t.Errorf("the provider was asked %v; want the chosen film's record fetched again", lib.provider.Calls)
	}
	world.wantAPersonsDoingKept(t)
}

// D535 §6: a rescan that keeps what was made throws none of it out.
func TestRescan_ARescanThatKeepsWhatWasMadeThrowsOutNothing(t *testing.T) {
	t.Parallel()
	world := newChosenAndWatched(t)
	lib := world.rescan.lib
	if _, err := world.rescan.rescans.RescanLibrary(t.Context(), lib.libraryID, false); err != nil {
		t.Fatal(err)
	}
	world.rescan.run()
	if len(world.rescan.scans.discarding) != 0 || len(world.images.discarded) != 0 || len(world.candidates(t)) != 1 {
		t.Errorf("a rescan keeping what was made discarded scans %v, images %v, and kept answers %+v; want "+
			"nothing thrown out", world.rescan.scans.discarding, world.images.discarded, world.candidates(t))
	}
	world.wantAPersonsDoingKept(t)
}
