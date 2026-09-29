// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

func TestPass_ReportsTheImagesAMatchNamesUntilTheyAreHeld(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	film := heat
	film.Artwork = []ArtRef{{Kind: ArtPoster, Ref: "/heat-poster.jpg"}, {Kind: ArtBackdrop, Ref: "/heat-back.jpg"}}
	lib.provider.searches[searchKey("Metropolis", 1995)] = []Candidate{lib.provider.add(film)}
	const rel = "Metropolis (1995).mkv"
	if report := lib.scan(videoRecord(rel)); len(report.OwedArtwork) != 0 {
		t.Fatalf("before its match: owed %+v; want nothing, it has no source yet", report.OwedArtwork)
	}
	var followed []OwedArtwork
	lib.pass.AfterPass(func(_ context.Context, report Report) { followed = append(followed, report.OwedArtwork...) })
	if outcome := lib.matchFileTitle(rel); outcome != OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	titleID := lib.titlesByPath()[rel].TitleID
	if len(followed) != 1 || !slices.Equal(followed[0].Refs, film.Artwork) {
		t.Errorf("the match itself left owing %+v; want the film's images, without waiting for a scan", followed)
	}
	report := lib.scan(videoRecord(rel))
	if len(report.OwedArtwork) != 1 || report.OwedArtwork[0].ItemID != titleID ||
		report.OwedArtwork[0].Source != "tmdb" || !slices.Equal(report.OwedArtwork[0].Refs, film.Artwork) {
		t.Fatalf("owed %+v; want the film's poster and backdrop", report.OwedArtwork)
	}
	// The poster is stored; the backdrop was refused by the node, and is held with no hash.
	for _, row := range []sqlcgen.InsertArtworkParams{
		{ItemID: titleID, Kind: ArtPoster, Source: "tmdb", SourceRef: new("/heat-poster.jpg"), SourceSha256: []byte{1}},
		{ItemID: titleID, Kind: ArtBackdrop, Source: "tmdb", SourceRef: new("/heat-back.jpg")},
	} {
		if err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			_, err := queries.InsertArtwork(t.Context(), row)
			return err //nolint:wrapcheck // a test
		}); err != nil {
			t.Fatal(err)
		}
	}
	if report = lib.scan(videoRecord(rel)); len(report.OwedArtwork) != 0 {
		t.Errorf("owed %+v once both are held; want nothing", report.OwedArtwork)
	}
}

func TestChoose_ARematchRetiresTheEarlierWorksImagesTheNewOneLacks(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	film := heat
	film.Artwork = []ArtRef{{Kind: ArtPoster, Ref: "/heat-poster.jpg"}, {Kind: ArtBackdrop, Ref: "/heat-back.jpg"}}
	lib.provider.searches[searchKey("Metropolis", 1995)] = []Candidate{lib.provider.add(film)}
	other := leon
	other.Artwork = []ArtRef{{Kind: ArtPoster, Ref: "/leon-poster.jpg"}}
	lib.provider.add(other)
	const rel = "Metropolis (1995).mkv"
	lib.scan(videoRecord(rel))
	if outcome := lib.matchFileTitle(rel); outcome != OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	titleID := lib.titlesByPath()[rel].TitleID
	// Metropolis's poster and backdrop are stored, and a person chose a backdrop of their own.
	for _, row := range []sqlcgen.InsertArtworkParams{
		{ItemID: titleID, Kind: ArtPoster, Source: "tmdb", SourceRef: new("/heat-poster.jpg"), SourceSha256: []byte{1}},
		{ItemID: titleID, Kind: ArtBackdrop, Source: "tmdb", SourceRef: new("/heat-back.jpg"), SourceSha256: []byte{2}},
		{ItemID: titleID, Kind: ArtBackdrop, Source: "user", SourceSha256: []byte{3}},
	} {
		if err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			_, err := queries.InsertArtwork(t.Context(), row)
			return err //nolint:wrapcheck // a test
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := lib.chooser.Choose(t.Context(), titleID, "tmdb", other.ID); err != nil {
		t.Fatal(err)
	}
	rows, err := lib.database.Read().ListLiveArtwork(t.Context(), titleID)
	if err != nil {
		t.Fatal(err)
	}
	var live []string
	for _, row := range rows {
		live = append(live, row.Kind+" "+row.Source)
	}
	slices.Sort(live)
	if want := []string{"backdrop user"}; !slices.Equal(live, want) {
		t.Errorf("after the rematch: %v live, want %v: none of Metropolis's images, the person's kept (D223 §9)", live, want)
	}
}
