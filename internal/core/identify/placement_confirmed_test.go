// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// confirmedPlacement is placedByNameLibrary's unnumbered episode with its placement confirmed.
func confirmedPlacement(t *testing.T) (lib *testLibrary, titleID int64) {
	t.Helper()
	lib = placedByNameLibrary(t)
	titleID = lib.titlesByPath()[placedByNameUnplaced].WorkID
	if err := lib.chooser.ConfirmGuess(t.Context(), titleID); err != nil {
		t.Fatal(err)
	}
	return lib, titleID
}

func (lib *testLibrary) placeOf(titleID int64) []string {
	lib.t.Helper()
	return storetest.QueryRaw(lib.t, lib.database.Path(),
		fmt.Sprintf("SELECT group_number || 'x' || number_in_group FROM position WHERE work_id = %d", titleID))
}

// A confirmed placement sticks (D401, amending D307 §2): a pass that places the episode by its
// name at the place confirmed leaves it unmarked, and a repeat pass writes no row.
func TestSeries_AConfirmedPlacementByNameStaysUnmarkedThroughLaterPasses(t *testing.T) {
	t.Parallel()
	lib, titleID := confirmedPlacement(t)
	if lib.isGuessed(titleID) {
		t.Fatal("the placement is still marked after it was confirmed")
	}
	lib.scanAll(placedByNameNumbered, placedByNameUnplaced, placedByNameBonus)
	if lib.isGuessed(titleID) {
		t.Error("a pass marked the confirmed placement a guess again")
	}
	lib.clock.Advance(time.Hour)
	before, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lib.pass.IdentifyLibrary(t.Context(), lib.libraryID); err != nil {
		t.Fatal(err)
	}
	after, err := lib.database.RowsChanged(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("a pass over a confirmed placement changed %d rows; want none", after-before)
	}
	if lib.isGuessed(titleID) {
		t.Error("the confirmed placement is marked after a repeat pass")
	}
}

// A pass that places a confirmed episode somewhere else makes a new guess, which stays a guess
// through the passes after it until a person confirms that place (D401).
func TestSeries_APlacementByNameAtAnotherPlaceAfterConfirmingIsANewGuess(t *testing.T) {
	t.Parallel()
	lib, titleID := confirmedPlacement(t)
	lib.provider.Seasons["615/1"] = []identify.Episode{{ID: "1001", Season: 1, Number: 1, Name: "Something Else"}}
	lib.provider.Seasons["615/2"] = []identify.Episode{
		{ID: "1101", Season: 2, Number: 10, Name: "A Comet of My Own"},
		{ID: "1102", Season: 2, Number: 11, Name: "Comet Pilot 3000"},
	}
	// The series' season listings are fetched again, as a rescan of its match does.
	series := lib.onlySeries()
	if err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.DeleteSeasonListings(t.Context(), sqlcgen.DeleteSeasonListingsParams{
			ItemID: series.ID, Provider: "tmdb",
		})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.matcher.Match(t.Context(), series.ID); err != nil {
		t.Fatal(err)
	}
	lib.runOwedPasses()
	if place := lib.placeOf(titleID); !slices.Equal(place, []string{"2x11"}) {
		t.Fatalf("the episode is at %v; want 2x11, where its name now places it", place)
	}
	if !lib.isGuessed(titleID) {
		t.Fatal("a placement somewhere else is not marked a new guess")
	}
	lib.scanAll(placedByNameNumbered, placedByNameUnplaced, placedByNameBonus)
	if !lib.isGuessed(titleID) {
		t.Error("the new guess lost its mark on the next pass")
	}
	if err := lib.chooser.ConfirmGuess(t.Context(), titleID); err != nil {
		t.Fatal(err)
	}
	lib.scanAll(placedByNameNumbered, placedByNameUnplaced, placedByNameBonus)
	if lib.isGuessed(titleID) {
		t.Error("the second place, confirmed, is marked a guess again")
	}
}
