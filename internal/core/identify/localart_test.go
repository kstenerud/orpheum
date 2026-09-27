// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func image(path string) nodeproto.Record {
	return nodeproto.Record{Type: nodeproto.RecordSidecar, Sidecar: &nodeproto.Sidecar{
		Path: nodeproto.Path{Text: path}, Kind: nodeproto.SidecarImage, Size: 100, MtimeMs: 7,
	}}
}

// owedKinds is each owed item's kinds from a source, sorted, by item.
func owedKinds(report Report, source string) map[int64][]string {
	kinds := map[int64][]string{}
	for _, owed := range report.Artwork {
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
	lib := newWorld(t, "mixed", "nfo")
	report := lib.scan(
		video("Films/Salt Harbour (1981)/Salt Harbour (1981).mkv"),
		image("Films/Salt Harbour (1981)/cover.jpg"),
		image("Films/Salt Harbour (1981)/poster.jpg"), // more specific than cover: it wins
		image("Films/Salt Harbour (1981)/Salt Harbour (1981)-fanart.png"),
		image("Films/Salt Harbour (1981)/Salt Harbour (1981)-thumb.jpg"), // a film has no still
		image("Films/Salt Harbour (1981)/screenshot.jpg"),                // no convention names it
		video("Shows/Glass Tide/Season 01/Glass Tide - S01E01.mkv"),
		image("Shows/Glass Tide/folder.jpg"),
		image("Shows/Glass Tide/season01-poster.jpg"), // a season shows nothing yet
		image("Shows/Glass Tide/Season 01/Glass Tide - S01E01-thumb.jpg"),
	)
	files := lib.files()
	film := files["Films/Salt Harbour (1981)/Salt Harbour (1981).mkv"].TitleID
	episode := files["Shows/Glass Tide/Season 01/Glass Tide - S01E01.mkv"].TitleID
	series := lib.onlySeries().ID
	got := owedKinds(report, SourceLocal)
	want := map[int64][]string{film: {ArtBackdrop, ArtPoster}, series: {ArtPoster}, episode: {ArtStill}}
	for item, kinds := range want {
		if !slices.Equal(got[item], kinds) {
			t.Errorf("item %d owes %v from beside it, want %v", item, got[item], kinds)
		}
	}
	if len(got) != len(want) {
		t.Errorf("owed %v; want only the film, the series and the episode", got)
	}
	for _, owed := range report.Artwork {
		for _, ref := range owed.Refs {
			if owed.ItemID == film && ref.Kind == ArtPoster && !strings.HasSuffix(ref.Ref, ":100:7") {
				t.Errorf("the poster's ref %q does not carry its size and time", ref.Ref)
			}
		}
	}

	tmdbOnly := newWorld(t, "movies", "tmdb")
	taken := tmdbOnly.scan(
		video("Salt Harbour (1981)/Salt Harbour (1981).mkv"), image("Salt Harbour (1981)/poster.jpg"))
	if kinds := owedKinds(taken, SourceLocal); len(kinds) != 1 {
		t.Errorf("a TMDB-only library owes %v from beside the media; every library takes them at once (D215 §1)",
			kinds)
	}
}

func TestPass_ATMDBOnlyLibraryStillFetchesTheProvidersImageToReplaceTheOneBesideTheMedia(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "tmdb")
	film := heat
	film.Artwork = []ArtRef{{Kind: ArtPoster, Ref: "/p.jpg"}}
	lib.provider.searches[searchKey("Metropolis", 1995)] = []Candidate{lib.provider.add(film)}
	const rel = "Metropolis (1995)/Metropolis (1995).mkv"
	lib.scan(video(rel), image("Metropolis (1995)/poster.jpg"))
	if outcome := lib.match(rel); outcome != OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	report := lib.scan(video(rel), image("Metropolis (1995)/poster.jpg"))
	titleID := lib.files()[rel].TitleID
	if tmdb := owedKinds(report, "tmdb")[titleID]; !slices.Equal(tmdb, []string{ArtPoster}) {
		t.Errorf("TMDB owes %v; a TMDB-only library fetches its poster to replace the one beside the film", tmdb)
	}
}

func TestPass_TheProvidersImageOfAKindBesideTheMediaIsNotFetched(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo_then_tmdb")
	film := heat
	film.Artwork = []ArtRef{{Kind: ArtPoster, Ref: "/p.jpg"}, {Kind: ArtBackdrop, Ref: "/b.jpg"}}
	lib.provider.searches[searchKey("Metropolis", 1995)] = []Candidate{lib.provider.add(film)}
	const rel = "Metropolis (1995)/Metropolis (1995).mkv"
	lib.scan(video(rel), image("Metropolis (1995)/poster.jpg"))
	if outcome := lib.match(rel); outcome != OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	report := lib.scan(video(rel), image("Metropolis (1995)/poster.jpg"))
	titleID := lib.files()[rel].TitleID
	if tmdb := owedKinds(report, "tmdb")[titleID]; !slices.Equal(tmdb, []string{ArtBackdrop}) {
		t.Errorf("TMDB owes %v; want only the backdrop, since a poster sits beside the film (D215 §1)", tmdb)
	}
	if local := owedKinds(report, SourceLocal)[titleID]; !slices.Equal(local, []string{ArtPoster}) {
		t.Errorf("beside it owes %v, want the poster", local)
	}
}

func TestPass_AnImageNoLongerBesideTheMediaIsRetired(t *testing.T) {
	t.Parallel()
	lib := newWorld(t, "movies", "nfo")
	const rel = "Metropolis (1995)/Metropolis (1995).mkv"
	report := lib.scan(video(rel), image("Metropolis (1995)/poster.jpg"))
	titleID := lib.files()[rel].TitleID
	var posterRef string
	for _, owed := range report.Artwork {
		if owed.ItemID == titleID && owed.Source == SourceLocal {
			posterRef = owed.Refs[0].Ref
		}
	}
	// The poster was stored, as the artwork job would have, and a person chose a backdrop.
	for _, row := range []sqlcgen.InsertArtworkParams{
		{ItemID: titleID, Kind: ArtPoster, Source: SourceLocal, SourceRef: &posterRef, ContentHash: []byte{1}},
		{ItemID: titleID, Kind: ArtBackdrop, Source: SourceLocal, SourceRef: new("gone:1:1"), ContentHash: []byte{2}},
	} {
		if err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			_, err := queries.InsertArtwork(t.Context(), row)
			return err //nolint:wrapcheck // a test
		}); err != nil {
			t.Fatal(err)
		}
	}
	storetest.ExecRaw(t, lib.database.Path(), "UPDATE artwork SET chosen = 1 WHERE kind = 'backdrop'")
	lib.scan(video(rel), image("Metropolis (1995)/poster.jpg"))
	if live := liveKinds(t, lib, titleID); !slices.Equal(live, []string{ArtBackdrop, ArtPoster}) {
		t.Errorf("with the poster still beside the film: %v live, want both", live)
	}
	lib.scan(video(rel)) // poster.jpg deleted
	if live := liveKinds(t, lib, titleID); !slices.Equal(live, []string{ArtBackdrop}) {
		t.Errorf("with poster.jpg gone: %v live, want only the person's choice, the poster retired", live)
	}
}

// liveKinds are an item's live artwork's kinds, sorted.
func liveKinds(t *testing.T, lib *world, itemID int64) []string {
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
	lib := newWorld(t, "shows", "nfo")
	// "Staffel 1" is a season folder to the filename parser, so the series' images are above it.
	report := lib.scan(
		video("Shows/Glass Tide/Staffel 1/Glass Tide - S01E01.mkv"),
		image("Shows/Glass Tide/folder.jpg"),
	)
	series := lib.onlySeries().ID
	if got := owedKinds(report, SourceLocal)[series]; !slices.Equal(got, []string{ArtPoster}) {
		t.Errorf("the series owes %v from beside it, want its poster from above the season folder", got)
	}
}
