// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"maps"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// D402: the rail's letter follows the sort key, so a title is under the letter of its name's first
// word that is no article; a name that is only an article stays under it.
func TestLetters_ATitleIsUnderItsFirstWordThatIsNoArticle(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("m.mkv", "m"), observedRecord("b.mkv", "b"), observedRecord("t.mkv", "t"),
	}, true)
	world.fileAsFilm("The Gold Rush", "m.mkv")
	world.fileAsFilm("A Woman of Paris", "b.mkv")
	world.fileAsFilm("The", "t.mkv")
	letters, err := world.browser.Letters(t.Context(), ListQuery{
		Reach: auth.EveryLibrary(), Kinds: []string{ListFilms}, Sort: SortName,
	})
	if err != nil || !slices.Equal(letters, []string{"G", "T", "W"}) {
		t.Errorf("the letters of The Gold Rush, A Woman of Paris and The = %v, %v; want G, T and W", letters, err)
	}
}

// A list by name says each entry's letter as the rail names it, by the same key: The Gold Rush under G,
// so the rail can stand at the letter of the poster beside it (the owner, 2026-11-11).
func TestList_SaysEachEntrysRailLetter(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("m.mkv", "m"), observedRecord("n.mkv", "n"), observedRecord("b.mkv", "b"),
	}, true)
	world.fileAsFilm("The Gold Rush", "m.mkv")
	world.fileAsFilm("7th Heaven", "n.mkv")
	world.fileAsFilm("A Woman of Paris", "b.mkv")
	page, err := world.browser.List(t.Context(), ListQuery{
		Reach: auth.EveryLibrary(), Kinds: []string{ListFilms}, Sort: SortName, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	said := map[string]string{}
	for _, entry := range page.Entries {
		said[entry.Name] = entry.Letter
	}
	want := map[string]string{"The Gold Rush": "G", "7th Heaven": "#", "A Woman of Paris": "W"}
	if !maps.Equal(said, want) {
		t.Errorf("the letters said = %v; want %v", said, want)
	}
}
