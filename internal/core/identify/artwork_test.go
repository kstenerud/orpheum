// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/core/library"
)

func TestPass_ReportsTheImagesAMatchNamesOnEveryPass(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movie", "tmdb")
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
	lib := newTestLibrary(t, "movie", "tmdb")
	film := heat
	film.Artwork = []identify.ImageRef{
		{Kind: library.ArtPoster, Ref: "/heat-poster.jpg"}, {Kind: library.ArtBackdrop, Ref: "/heat-back.jpg"},
	}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(film)}
	other := theo
	other.Artwork = []identify.ImageRef{{Kind: library.ArtPoster, Ref: "/theo-poster.jpg"}}
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
		t.Errorf("after the rematch: reported %+v; want the title with Théo's images, so Metropolis's go (D223 §9)",
			rematched)
	}
}

func TestPass_ReportsASeriesSeasonsPostersByTheirSeasonOnEveryPass(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "series", "tmdb")
	series := identify.Metadata{ID: "4807", Name: "Emberwing", Year: 2002, Artwork: []identify.ImageRef{
		{Kind: library.ArtPoster, Ref: "/emberwing.jpg"},
		{Kind: library.ArtPoster, GroupNumber: new(int64(1)), Ref: "/season-1.jpg"},
	}}
	lib.provider.Searches[identifytest.SearchKey("Emberwing", 0)] = []identify.Candidate{lib.provider.Add(series)}
	lib.provider.Seasons["1437/1"] = []identify.Episode{{ID: "e1", Season: 1, Number: 1, Name: "Stillness"}}
	const rel = "Emberwing/Season 01/Emberwing - S01E01.mkv"
	lib.scanAll(rel)
	seriesID := lib.onlySeries().ID
	if _, err := lib.matcher.Match(t.Context(), seriesID); err != nil {
		t.Fatal(err)
	}
	lib.runOwedPasses()
	// Every pass names them from the series' stored record, so a series matched before its seasons'
	// posters were read gets them at its next pass (D379).
	report := lib.scan(videoRecord(rel))
	var named []string
	for _, item := range report.NamedImages {
		for _, ref := range item.Refs {
			season := "own"
			if ref.GroupNumber != nil {
				season = fmt.Sprint(*ref.GroupNumber)
			}
			named = append(named, fmt.Sprintf("%d %s %s", item.ItemID, season, ref.Ref))
		}
	}
	want := []string{fmt.Sprintf("%d own /emberwing.jpg", seriesID), fmt.Sprintf("%d 1 /season-1.jpg", seriesID)}
	if !slices.Equal(named, want) {
		t.Errorf("a pass named %q; want the series' poster and season 1's, by its season", named)
	}
}
