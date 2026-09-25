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
	lib := newWorld(t, "movies", "tmdb")
	film := heat
	film.Artwork = []ArtRef{{Kind: ArtPoster, Ref: "/heat-poster.jpg"}, {Kind: ArtBackdrop, Ref: "/heat-back.jpg"}}
	lib.provider.searches[searchKey("Metropolis", 1995)] = []Candidate{lib.provider.add(film)}
	const rel = "Metropolis (1995).mkv"
	if report := lib.scan(video(rel)); len(report.Artwork) != 0 {
		t.Fatalf("before its match: owed %+v; want nothing, it has no source yet", report.Artwork)
	}
	var followed []OwedArtwork
	lib.pass.AfterPass(func(_ context.Context, report Report) { followed = append(followed, report.Artwork...) })
	if outcome := lib.match(rel); outcome != OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	titleID := lib.files()[rel].TitleID
	if len(followed) != 1 || !slices.Equal(followed[0].Refs, film.Artwork) {
		t.Errorf("the match itself left owing %+v; want the film's images, without waiting for a scan", followed)
	}
	report := lib.scan(video(rel))
	if len(report.Artwork) != 1 || report.Artwork[0].ItemID != titleID || report.Artwork[0].Source != "tmdb" ||
		!slices.Equal(report.Artwork[0].Refs, film.Artwork) {
		t.Fatalf("owed %+v; want the film's poster and backdrop", report.Artwork)
	}
	// The poster is stored; the backdrop was refused by the node, and is held with no hash.
	for _, row := range []sqlcgen.InsertArtworkParams{
		{ItemID: titleID, Kind: ArtPoster, Source: "tmdb", SourceRef: new("/heat-poster.jpg"), ContentHash: []byte{1}},
		{ItemID: titleID, Kind: ArtBackdrop, Source: "tmdb", SourceRef: new("/heat-back.jpg")},
	} {
		if err := lib.database.Write(t.Context(), func(queries *sqlcgen.Queries) error {
			_, err := queries.InsertArtwork(t.Context(), row)
			return err //nolint:wrapcheck // a test
		}); err != nil {
			t.Fatal(err)
		}
	}
	if report = lib.scan(video(rel)); len(report.Artwork) != 0 {
		t.Errorf("owed %+v once both are held; want nothing", report.Artwork)
	}
}
