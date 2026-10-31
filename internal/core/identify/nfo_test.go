// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"errors"
	"maps"
	"slices"
	"testing"
)

func TestParseNFO_AKodiMovieWithItsCorrectionsLocked(t *testing.T) {
	t.Parallel()
	body := `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>
<movie>
  <title>Metropolis</title>
  <originaltitle>Metropolis</originaltitle>
  <plot>A group of professional bank robbers...</plot>
  <runtime>170</runtime>
  <premiered>1995-12-15</premiered>
  <uniqueid type="imdb">tt0234567</uniqueid>
  <uniqueid type="tmdb" default="true">949</uniqueid>
  <playcount>3</playcount>
  <lockedfields>Name|Overview|Genres</lockedfields>
</movie>
https://www.themoviedb.org/movie/9019`
	nfo, err := ParseNFO([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if nfo.Kind != "movie" || nfo.Title != "Metropolis" || nfo.Year != 1995 || nfo.RuntimeMs != 170*60_000 ||
		nfo.Plot == "" {
		t.Errorf("nfo = %+v", nfo)
	}
	if want := map[string]string{"imdb": "tt0234567", "tmdb": "9019"}; !maps.Equal(nfo.ProviderIDs, want) {
		t.Errorf("provider ids %v, want %v", nfo.ProviderIDs, want)
	}
	if want := []string{"name", "overview"}; !slices.Equal(nfo.LockedFields, want) {
		t.Errorf("locked %v, want %v: the fields we model, from lockedfields", nfo.LockedFields, want)
	}
}

func TestParseNFO_LockdataLocksEveryFieldItSets(t *testing.T) {
	t.Parallel()
	nfo, err := ParseNFO([]byte(`<movie><title>Arkons</title><year>1986</year><lockdata>true</lockdata></movie>`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"name", "year"}; !slices.Equal(nfo.LockedFields, want) {
		t.Errorf("locked %v, want %v", nfo.LockedFields, want)
	}
}

func TestParseNFO_EpisodesShowsAndTheDevLibrarysOwn(t *testing.T) {
	t.Parallel()
	episode, err := ParseNFO([]byte(`<episodedetails><title>Pilot</title><season>1</season><episode>2</episode>` +
		`<aired>2024-01-08</aired></episodedetails>`))
	if err != nil || episode.Kind != "episodedetails" || episode.Season != 1 || episode.Episode != 2 ||
		episode.Year != 2024 {
		t.Errorf("episode = %+v, %v", episode, err)
	}
	show, err := ParseNFO([]byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tvshow>\n  <title>Test Show</title>\n" +
		"  <year>2024</year>\n  <tvdbid>81189</tvdbid>\n</tvshow>\n"))
	if err != nil || show.Kind != "tvshow" || show.Title != "Test Show" || show.ProviderIDs["tvdb"] != "81189" {
		t.Errorf("show = %+v, %v", show, err)
	}
	// dev_media.py's movie_nfo.
	dev, err := ParseNFO([]byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<movie>\n  <title>Remux Film</title>\n" +
		"  <year>2021</year>\n  <plot>A synthetic film generated for Orpheum's development library.</plot>\n" +
		"</movie>\n"))
	if err != nil || dev.Title != "Remux Film" || dev.Year != 2021 || len(dev.LockedFields) != 0 {
		t.Errorf("the dev library's NFO = %+v, %v", dev, err)
	}
}

func TestParseNFO_RefusesWhatIsNotOne(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", "plain text", "<html><body>hi</body></html>", "<movie><title>unclosed"} {
		if _, err := ParseNFO([]byte(body)); !errors.Is(err, ErrNotNFO) {
			t.Errorf("ParseNFO(%q) = %v, want ErrNotNFO", body, err)
		}
	}
}

func TestYearOfDate_ReadsTheYearOfAnISODateOrZero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		date string
		want int
	}{
		{"1995-12-15", 1995}, {" 2024-01-02\n", 2024}, {"1999", 1999}, {"", 0}, {"unknown", 0}, {"-5-01-01", 0},
	}
	for _, test := range cases {
		if got := YearOfDate(test.date); got != test.want {
			t.Errorf("YearOfDate(%q) = %d, want %d", test.date, got, test.want)
		}
	}
}
