// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"context"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/core/library"
)

func TestPass_ReportsTheImagesAMatchNamesOnEveryPass(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	film := heat
	film.Artwork = []identify.ImageRef{
		{Kind: library.ArtPoster, Ref: "/heat-poster.jpg"}, {Kind: library.ArtBackdrop, Ref: "/heat-back.jpg"},
	}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(film)}
	const rel = "Metropolis (1995).mkv"
	if report := lib.scan(videoRecord(rel)); len(report.NamedImages) != 0 {
		t.Fatalf("before its match: named %+v; want nothing, it has no source yet", report.NamedImages)
	}
	var followed []identify.NamedImages
	lib.pass.AfterPass(func(_ context.Context, report identify.Report) {
		followed = append(followed, report.NamedImages...)
	})
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	titleID := lib.titlesByPath()[rel].TitleID
	if len(followed) != 1 || !slices.Equal(followed[0].Refs, film.Artwork) {
		t.Errorf("the match itself reported %+v; want the film's images, without waiting for a scan", followed)
	}
	for range 2 { // the artwork service, not the pass, says which it holds
		report := lib.scan(videoRecord(rel))
		if len(report.NamedImages) != 1 || report.NamedImages[0].ItemID != titleID ||
			report.NamedImages[0].Source != "tmdb" || !slices.Equal(report.NamedImages[0].Refs, film.Artwork) {
			t.Fatalf("named %+v; want the film's poster and backdrop", report.NamedImages)
		}
	}
}

func TestChoose_ARematchReportsTheNewWorksImages(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "tmdb")
	film := heat
	film.Artwork = []identify.ImageRef{
		{Kind: library.ArtPoster, Ref: "/heat-poster.jpg"}, {Kind: library.ArtBackdrop, Ref: "/heat-back.jpg"},
	}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(film)}
	other := leon
	other.Artwork = []identify.ImageRef{{Kind: library.ArtPoster, Ref: "/leon-poster.jpg"}}
	lib.provider.Add(other)
	const rel = "Metropolis (1995).mkv"
	lib.scan(videoRecord(rel))
	var rematched []identify.NamedImages
	lib.pass.AfterPass(func(_ context.Context, report identify.Report) {
		rematched = append(rematched, report.Rematched...)
	})
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	if len(rematched) != 0 {
		t.Fatalf("a first match reported %+v rematched; want nothing", rematched)
	}
	titleID := lib.titlesByPath()[rel].TitleID
	if err := lib.chooser.Choose(t.Context(), titleID, "tmdb", other.ID); err != nil {
		t.Fatal(err)
	}
	if len(rematched) != 1 || rematched[0].ItemID != titleID || rematched[0].Source != "tmdb" ||
		!slices.Equal(rematched[0].Refs, other.Artwork) {
		t.Errorf("after the rematch: reported %+v; want the title with Léon's images, so Metropolis's go (D223 §9)",
			rematched)
	}
}
