// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// What waits for a person's match is counted by one statement, whatever the library holds, never
// by fetching its titles (D303, DL6; the access audit's A7).
func TestWaitingForAPerson_IsOneCountingStatement(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	lib.scan(videoRecord("Metropolis (1995)/Metropolis (1995).mkv"), videoRecord("Arkon (1979)/Arkon (1979).mkv"))
	ctx, tally := querytally.New(t.Context())
	if _, err := lib.chooser.WaitingForAPerson(ctx); err != nil {
		t.Fatal(err)
	}
	if got := tally.String(); got != "CountWaitingForAPerson×1" {
		t.Errorf("counting what waits ran %q; want the one counting statement", got)
	}
}

// "Needs a match" lists what the count counts, no more: a series no search found is listed once,
// never its episodes (D209), and one not searched yet waits for no one (the owner's staging,
// 2026-10-21, where the count said 15 and the list was hundreds of episodes).
func TestItems_ListWhatWaitingForAPersonCounts(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "shows", "tmdb")
	lib.scanAll("Nowhere Show/Season 01/Nowhere Show - S01E01.mkv",
		"Nowhere Show/Season 01/Nowhere Show - S01E02.mkv", "Unsearched Show/Season 01/Unsearched Show - S01E01.mkv")
	series, err := lib.database.Read().ListLibrarySeries(t.Context(), &lib.libraryID)
	if err != nil || len(series) != 2 {
		t.Fatalf("series %+v, %v; want two", series, err)
	}
	nowhere := series[0].ID
	if names := lib.seriesNames(); names[nowhere] != "Nowhere Show" {
		nowhere = series[1].ID
	}
	if outcome, matchErr := lib.matcher.Match(t.Context(), nowhere); matchErr != nil ||
		outcome != identify.OutcomeNoMatch {
		t.Fatalf("Match = %s, %v; want no match", outcome, matchErr)
	}
	items, err := lib.chooser.Items(t.Context(), auth.EveryLibrary(), identify.IdentificationUnidentified)
	if err != nil || len(items) != 1 || items[0].ID != nowhere {
		t.Errorf("unidentified items %+v, %v; want the one series searched for, not its episodes", items, err)
	}
	if count, countErr := lib.chooser.WaitingForAPerson(t.Context()); countErr != nil || count != len(items) {
		t.Errorf("WaitingForAPerson = %d, %v; want the %d the list shows", count, countErr, len(items))
	}
}
