// SPDX-License-Identifier: GPL-3.0-or-later

package identify_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func imageRecord(path string) nodeproto.Record {
	return nodeproto.Record{Type: nodeproto.RecordSidecar, Sidecar: &nodeproto.Sidecar{
		Path: nodeproto.Path{Text: path}, Kind: nodeproto.SidecarImage, SizeBytes: 100, ModifiedAt: 7,
	}}
}

// owedKinds is each owed item's kinds from a source, sorted, by item.
func owedKinds(report identify.Report, source string) map[int64][]string {
	kinds := map[int64][]string{}
	for _, owed := range report.OwedArtwork {
		if owed.Source != source {
			continue
		}
		for _, ref := range owed.Refs {
			kinds[owed.ItemID] = append(kinds[owed.ItemID], ref.Kind)
		}
		slices.Sort(kinds[owed.ItemID])
	}
	return kinds
}

func TestPass_ImagesBesideTheMediaAreOwedByTheirConventionalNames(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "mixed", "nfo")
	report := lib.scan(
		videoRecord("Films/Salt Harbour (1981)/Salt Harbour (1981).mkv"),
		imageRecord("Films/Salt Harbour (1981)/cover.jpg"),
		imageRecord("Films/Salt Harbour (1981)/poster.jpg"), // more specific than cover: it wins
		imageRecord("Films/Salt Harbour (1981)/Salt Harbour (1981)-fanart.png"),
		imageRecord("Films/Salt Harbour (1981)/Salt Harbour (1981)-thumb.jpg"), // a film has no still
		imageRecord("Films/Salt Harbour (1981)/screenshot.jpg"),                // no convention names it
		videoRecord("Shows/Glass Tide/Season 01/Glass Tide - S01E01.mkv"),
		imageRecord("Shows/Glass Tide/folder.jpg"),
		imageRecord("Shows/Glass Tide/season01-poster.jpg"), // a season shows nothing yet
		imageRecord("Shows/Glass Tide/Season 01/Glass Tide - S01E01-thumb.jpg"),
	)
	files := lib.titlesByPath()
	film := files["Films/Salt Harbour (1981)/Salt Harbour (1981).mkv"].TitleID
	episode := files["Shows/Glass Tide/Season 01/Glass Tide - S01E01.mkv"].TitleID
	series := lib.onlySeries().ID
	got := owedKinds(report, identify.SourceLocal)
	want := map[int64][]string{
		film: {identify.ArtBackdrop, identify.ArtPoster}, series: {identify.ArtPoster}, episode: {identify.ArtStill},
	}
	for item, kinds := range want {
		if !slices.Equal(got[item], kinds) {
			t.Errorf("item %d owes %v from beside it, want %v", item, got[item], kinds)
		}
	}
	if len(got) != len(want) {
		t.Errorf("owed %v; want only the film, the series and the episode", got)
	}
	for _, owed := range report.OwedArtwork {
		for _, ref := range owed.Refs {
			if owed.ItemID == film && ref.Kind == identify.ArtPoster && !strings.HasSuffix(ref.Ref, ":100:7") {
				t.Errorf("the poster's ref %q does not carry its size and time", ref.Ref)
			}
		}
	}

	tmdbOnly := newTestLibrary(t, "movies", "tmdb")
	taken := tmdbOnly.scan(
		videoRecord("Salt Harbour (1981)/Salt Harbour (1981).mkv"), imageRecord("Salt Harbour (1981)/poster.jpg"))
	if kinds := owedKinds(taken, identify.SourceLocal); len(kinds) != 1 {
		t.Errorf("a TMDB-only library owes %v from beside the media; every library takes them at once (D215 §1)",
			kinds)
	}
}

func TestPass_ATMDBOnlyLibraryStillFetchesTheProvidersImageToReplaceTheOneBesideTheMedia(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	film := heat
	film.Artwork = []identify.ArtRef{{Kind: identify.ArtPoster, Ref: "/p.jpg"}}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(film)}
	const rel = "Metropolis (1995)/Metropolis (1995).mkv"
	lib.scan(videoRecord(rel), imageRecord("Metropolis (1995)/poster.jpg"))
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	report := lib.scan(videoRecord(rel), imageRecord("Metropolis (1995)/poster.jpg"))
	titleID := lib.titlesByPath()[rel].TitleID
	if tmdb := owedKinds(report, "tmdb")[titleID]; !slices.Equal(tmdb, []string{identify.ArtPoster}) {
		t.Errorf("TMDB owes %v; a TMDB-only library fetches its poster to replace the one beside the film", tmdb)
	}
}

func TestPass_TheProvidersImageOfAKindBesideTheMediaIsNotFetched(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo_then_tmdb")
	film := heat
	film.Artwork = []identify.ArtRef{
		{Kind: identify.ArtPoster, Ref: "/p.jpg"}, {Kind: identify.ArtBackdrop, Ref: "/b.jpg"},
	}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(film)}
	const rel = "Metropolis (1995)/Metropolis (1995).mkv"
	lib.scan(videoRecord(rel), imageRecord("Metropolis (1995)/poster.jpg"))
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	report := lib.scan(videoRecord(rel), imageRecord("Metropolis (1995)/poster.jpg"))
	titleID := lib.titlesByPath()[rel].TitleID
	if tmdb := owedKinds(report, "tmdb")[titleID]; !slices.Equal(tmdb, []string{identify.ArtBackdrop}) {
		t.Errorf("TMDB owes %v; want only the backdrop, since a poster sits beside the film (D215 §1)", tmdb)
	}
	if local := owedKinds(report, identify.SourceLocal)[titleID]; !slices.Equal(local, []string{identify.ArtPoster}) {
		t.Errorf("beside it owes %v, want the poster", local)
	}
}

func TestPass_AnImageNoLongerBesideTheMediaIsRetired(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo")
	const rel = "Metropolis (1995)/Metropolis (1995).mkv"
	report := lib.scan(videoRecord(rel), imageRecord("Metropolis (1995)/poster.jpg"))
	titleID := lib.titlesByPath()[rel].TitleID
	var posterRef string
	for _, owed := range report.OwedArtwork {
		if owed.ItemID == titleID && owed.Source == identify.SourceLocal {
			posterRef = owed.Refs[0].Ref
		}
	}
	// The poster was stored, as the make_artwork_sizes job would have, and a person chose a backdrop.
	for _, row := range []sqlcgen.InsertArtworkParams{
		{
			ItemID: titleID, Kind: identify.ArtPoster, Source: identify.SourceLocal, SourceRef: &posterRef,
			SourceSha256: []byte{1},
		},
		{
			ItemID: titleID, Kind: identify.ArtBackdrop, Source: identify.SourceLocal, SourceRef: new("gone:1:1"),
			SourceSha256: []byte{2},
		},
	} {
		if err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			_, err := queries.InsertArtwork(t.Context(), row)
			return err //nolint:wrapcheck // a test
		}); err != nil {
			t.Fatal(err)
		}
	}
	storetest.ExecRaw(t, lib.database.Path(), "UPDATE artwork SET is_chosen = 1 WHERE kind = 'backdrop'")
	lib.scan(videoRecord(rel), imageRecord("Metropolis (1995)/poster.jpg"))
	if live := liveKinds(t, lib, titleID); !slices.Equal(live, []string{identify.ArtBackdrop, identify.ArtPoster}) {
		t.Errorf("with the poster still beside the film: %v live, want both", live)
	}
	lib.scan(videoRecord(rel)) // poster.jpg deleted
	if live := liveKinds(t, lib, titleID); !slices.Equal(live, []string{identify.ArtBackdrop}) {
		t.Errorf("with poster.jpg gone: %v live, want only the person's choice, the poster retired", live)
	}
}

// liveKinds are an item's live artwork's kinds, sorted.
func liveKinds(t *testing.T, lib *testLibrary, itemID int64) []string {
	t.Helper()
	rows, err := lib.database.Read().ListLiveArtwork(t.Context(), itemID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := make([]string, 0, len(rows))
	for _, row := range rows {
		kinds = append(kinds, row.Kind)
	}
	slices.Sort(kinds)
	return kinds
}

func TestPass_ASeasonFolderIsReadAsTheFilenameParserReadsOne(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "shows", "nfo")
	// "Staffel 1" is a season folder to the filename parser, so the series' images are above it.
	report := lib.scan(
		videoRecord("Shows/Glass Tide/Staffel 1/Glass Tide - S01E01.mkv"),
		imageRecord("Shows/Glass Tide/folder.jpg"),
	)
	series := lib.onlySeries().ID
	if got := owedKinds(report, identify.SourceLocal)[series]; !slices.Equal(got, []string{identify.ArtPoster}) {
		t.Errorf("the series owes %v from beside it, want its poster from above the season folder", got)
	}
}
