// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
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
	if err != nil || !slices.Equal(letters, []string{"B", "M", "T"}) {
		t.Errorf("the letters of The Gold Rush, A Woman of Paris and The = %v, %v; want B, M and T", letters, err)
	}
}
