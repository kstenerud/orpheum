// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"errors"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// A kind no list holds, as a music library's would be until one is built, fails the list by name
// and by date added, alone or beside a kind it holds, where it was listed and counted as films (the
// code audit of 2026-11-21, finding 10). By release date it was refused already, as a sort it does
// not come in (ErrNoSuchSort).
func TestBrowse_AKindNoListHoldsFailsRatherThanListingFilms(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{observedRecord("a.mkv", "a")}, true)
	world.fileAsFilm("Élan Vital", "a.mkv")
	const music ListKind = "music"
	for _, kinds := range [][]ListKind{{music}, {ListFilms, music}} {
		for _, sort := range []string{SortName, SortAdded} {
			page, err := world.browser.List(t.Context(), ListQuery{
				Reach: auth.EveryLibrary(), Kinds: kinds, Sort: sort, Limit: WholeList,
			})
			if !errors.Is(err, ErrNoSuchKind) {
				t.Errorf("%v by %s = %+v, %v; want ErrNoSuchKind", kinds, sort, page, err)
			}
		}
	}
}
