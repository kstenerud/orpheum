// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

func imageRecord(path string) nodeproto.ScanRecord {
	return nodeproto.ScanRecord{Type: nodeproto.RecordSidecar, Sidecar: &nodeproto.Sidecar{
		Path: nodeproto.Path{Text: path}, Kind: nodeproto.SidecarImage, SizeBytes: 100, ModifiedAt: 7,
	}}
}

// namedKinds is each item's kinds its provider names, sorted, by item.
func namedKinds(report identify.Report, source string) map[int64][]string {
	kinds := map[int64][]string{}
	for _, named := range report.NamedImages {
		if named.Source != source {
			continue
		}
		for _, ref := range named.Refs {
			kinds[named.ItemID] = append(kinds[named.ItemID], ref.Kind)
		}
		slices.Sort(kinds[named.ItemID])
	}
	return kinds
}

// localKinds is each item's kinds found beside the media, sorted, by item.
func localKinds(report identify.Report) map[int64][]string {
	kinds := map[int64][]string{}
	if report.BesideTheMedia == nil {
		return kinds
	}
	for _, image := range report.BesideTheMedia.Images {
		kinds[image.ItemID] = append(kinds[image.ItemID], image.Kind)
		slices.Sort(kinds[image.ItemID])
	}
	return kinds
}

func TestPass_ImagesBesideTheMediaAreFoundByTheirConventionalNames(t *testing.T) {
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
	got := localKinds(report)
	want := map[int64][]string{
		film: {library.ArtBackdrop, library.ArtPoster}, series: {library.ArtPoster}, episode: {library.ArtStill},
	}
	for item, kinds := range want {
		if !slices.Equal(got[item], kinds) {
			t.Errorf("item %d has %v found beside it, want %v", item, got[item], kinds)
		}
	}
	if len(got) != len(want) {
		t.Errorf("found %v; want only the film, the series and the episode", got)
	}
	for _, image := range report.BesideTheMedia.Images {
		isPoster := image.ItemID == film && image.Kind == library.ArtPoster
		if isPoster && (image.SizeBytes != 100 || image.ModifiedAtMs != 7) {
			t.Errorf("the poster %+v does not carry its size and time", image)
		}
	}

	tmdbOnly := newTestLibrary(t, "movies", "tmdb")
	taken := tmdbOnly.scan(
		videoRecord("Salt Harbour (1981)/Salt Harbour (1981).mkv"), imageRecord("Salt Harbour (1981)/poster.jpg"))
	if kinds := localKinds(taken); len(kinds) != 1 {
		t.Errorf("a TMDB-only library found %v beside the media; every library takes them at once (D215 §1)",
			kinds)
	}
}

func TestPass_ReportsEveryImageTheProviderNamesBesideThoseBesideTheMedia(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo_then_tmdb")
	film := heat
	film.Artwork = []identify.ImageRef{
		{Kind: library.ArtPoster, Ref: "/p.jpg"}, {Kind: library.ArtBackdrop, Ref: "/b.jpg"},
	}
	lib.provider.Searches[identifytest.SearchKey("Metropolis", 1995)] = []identify.Candidate{lib.provider.Add(film)}
	const rel = "Metropolis (1995)/Metropolis (1995).mkv"
	lib.scan(videoRecord(rel), imageRecord("Metropolis (1995)/poster.jpg"))
	if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
		t.Fatalf("outcome %s", outcome)
	}
	report := lib.scan(videoRecord(rel), imageRecord("Metropolis (1995)/poster.jpg"))
	titleID := lib.titlesByPath()[rel].TitleID
	// Which of them is owed, the library's preference says, and the artwork service decides for a
	// pass and a match alike (TestTake_InALibraryThatShowsTheMediasOwnImagesAProvidersOfAKindBesideItIsNotOwed).
	tmdb := namedKinds(report, "tmdb")[titleID]
	if !slices.Equal(tmdb, []string{library.ArtBackdrop, library.ArtPoster}) {
		t.Errorf("TMDB names %v; want its poster and backdrop, reported as they are", tmdb)
	}
	if local := localKinds(report)[titleID]; !slices.Equal(local, []string{library.ArtPoster}) {
		t.Errorf("beside it found %v, want the poster", local)
	}
}

func TestPass_AnImageNoLongerBesideTheMediaIsNoLongerReported(t *testing.T) {
	t.Parallel()
	lib := newTestLibrary(t, "movies", "nfo")
	const rel = "Metropolis (1995)/Metropolis (1995).mkv"
	report := lib.scan(videoRecord(rel), imageRecord("Metropolis (1995)/poster.jpg"))
	titleID := lib.titlesByPath()[rel].TitleID
	if got := localKinds(report)[titleID]; !slices.Equal(got, []string{library.ArtPoster}) {
		t.Fatalf("with poster.jpg beside the film: found %v, want the poster", got)
	}
	report = lib.scan(videoRecord(rel)) // poster.jpg deleted
	if report.BesideTheMedia == nil || report.BesideTheMedia.LibraryID != lib.libraryID ||
		len(report.BesideTheMedia.Images) != 0 {
		t.Errorf("with poster.jpg gone: reported %+v; want the library's pass, with nothing found, so the "+
			"artwork service retires the poster (D215 §1)", report.BesideTheMedia)
	}
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
	if got := localKinds(report)[series]; !slices.Equal(got, []string{library.ArtPoster}) {
		t.Errorf("the series has %v found beside it, want its poster from above the season folder", got)
	}
}
