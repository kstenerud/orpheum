// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func TestWhereabouts_NameTheLibrariesTheCallerReachesAndAnExtrasFilm(t *testing.T) {
	t.Parallel()
	world := newShelf(t)
	world.scan([]nodeproto.ScanRecord{
		observedRecord("metropolis.mkv", "heat"), observedRecord("trailer.mkv", "trailer"),
		observedRecord("pilot.mkv", "pilot"),
	}, true)
	heat := world.fileAsFilm("Metropolis", "metropolis.mkv")
	trailer := world.fileAsFilm("Metropolis trailer", "trailer.mkv")
	pilot := world.fileAsFilm("Pilot", "pilot.mkv")
	err := world.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
		return queries.UpsertScannerExtraRelation(t.Context(), sqlcgen.UpsertScannerExtraRelationParams{
			OwnerTitle: heat, ExtraTitle: trailer, ExtraType: "trailer",
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	series := librarytest.Item(t, world.database, librarytest.ItemSpec{Kind: "collection", LibraryID: 1})
	storetest.ExecRaw(t, world.database.Path(),
		fmt.Sprintf("INSERT INTO collection (id, kind, membership, created_by) VALUES (%d, 'series', 'explicit', "+
			"'scanner')", series),
		fmt.Sprintf("INSERT INTO collection_member (collection_id, member_title) VALUES (%d, %d)", series, pilot))

	extra, err := world.browser.Whereabouts(t.Context(), auth.EveryLibrary(), trailer, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(extra.Libraries) != 1 || extra.Libraries[0].ID != 1 || extra.Libraries[0].Name == "" {
		t.Errorf("the extra's libraries = %v; want library 1, the one its file is in, named", extra.Libraries)
	}
	if extra.FilmOfExtra == nil || *extra.FilmOfExtra != (FilmOfExtra{ID: heat, Name: "Metropolis"}) {
		t.Errorf("the extra's film = %+v; want Metropolis (%d)", extra.FilmOfExtra, heat)
	}
	ofSeries, err := world.browser.Whereabouts(t.Context(), auth.EveryLibrary(), series, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(ofSeries.Libraries), []int64{1}) || ofSeries.FilmOfExtra != nil {
		t.Errorf("the series' whereabouts = %+v; want library 1, its episode's, and no film", ofSeries)
	}
	unreached, err := world.browser.Whereabouts(t.Context(), auth.LibrariesOnly(2), heat, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(unreached.Libraries) != 0 {
		t.Errorf("libraries a caller does not reach = %v; want none named", unreached.Libraries)
	}
}

// ids are libraries' ids, in their order.
func ids(libraries []ListingLibrary) []int64 {
	list := make([]int64, 0, len(libraries))
	for _, library := range libraries {
		list = append(list, library.ID)
	}
	return list
}
