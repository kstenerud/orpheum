// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// identifiedTables are the tables a pass writes, each read whole in a fixed order.
var identifiedTables = []string{
	"SELECT * FROM item ORDER BY id",
	"SELECT * FROM work ORDER BY id",
	"SELECT * FROM sequence ORDER BY id",
	"SELECT * FROM version ORDER BY id",
	"SELECT place.id, place.file_id, place.filing, place.filing_reason, file_filing.filing, " +
		"file_filing.filing_reason, file_filing.version_id, file_filing.identified_from_path FROM place " +
		"LEFT JOIN file_filing ON file_filing.file_id = place.file_id ORDER BY place.id, file_filing.library_kind",
	// Not its id: a surrogate, numbered in the order a pass happens to resolve its titles.
	"SELECT item_id, language, is_original, name, overview, tagline, search_alternates FROM item_text " +
		"ORDER BY item_id, language",
	"SELECT * FROM sort_key ORDER BY item_id, locale",
	"SELECT * FROM field_override ORDER BY item_id, field, language",
	"SELECT * FROM item_provider_id ORDER BY item_id, provider, provider_id",
	"SELECT * FROM ordering ORDER BY id",
	"SELECT * FROM position ORDER BY ordering_id, work_id",
	"SELECT * FROM extra_relation ORDER BY owner_work, extra_work",
}

// A pass that reads its library from the snapshot writes what one that reads each row as it needs
// it writes, step for step, through scans that add, move, rename, change and remove files, a
// person's override and its removal, and a series' match (D303, DL7, M9).
func TestPass_FromTheSnapshotWritesWhatAPassReadingAsItGoesWrites(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name, kind, source string
		steps              []func(lib *testLibrary)
	}{
		{"films and series filed by NFO", "movie", "nfo", nfoScenario()},
		{"films matched at TMDB", "movie", "tmdb", tmdbFilmsScenario()},
		{"series matched at TMDB", "series", "tmdb", tmdbSeriesScenario()},
	} {
		fromSnapshot := newTestLibrary(t, scenario.kind, scenario.source)
		asItGoes := newTestLibrary(t, scenario.kind, scenario.source)
		identify.ReadAsItGoes(asItGoes.pass)
		for number, step := range scenario.steps {
			step(fromSnapshot)
			step(asItGoes)
			for _, query := range identifiedTables {
				got := storetest.QueryRaw(t, fromSnapshot.database.Path(), query)
				want := storetest.QueryRaw(t, asItGoes.database.Path(), query)
				if !slices.Equal(got, want) {
					t.Errorf("%s, after step %d: %s\nfrom the snapshot: %v\nreading as it goes: %v",
						scenario.name, number+1, query, got, want)
				}
			}
		}
	}
}

func nfoScenario() []func(lib *testLibrary) {
	metropolisNFO := `<movie><title>Metropolis</title><year>1927</year><plot>A city.</plot>` +
		`<uniqueid type="tmdb">9019</uniqueid><lockdata>true</lockdata></movie>`
	first := []nodeproto.ScanRecord{
		nfoRecord("Metropolis (1927)/Metropolis (1927).nfo", metropolisNFO),
		videoRecord("Metropolis (1927)/Metropolis (1927).mkv"),
		videoRecord("Metropolis (1927)/Metropolis (1927)-trailer.mkv"),
		videoRecord("Metropolis (1927)/Featurettes/Making Metropolis.mkv"),
		videoRecord("Arkon (1979)/Arkon (1979).mkv"),
		videoRecord("Greed (1924).mkv"),
		nfoRecord("Test Show/tvshow.nfo", `<tvshow><title>Test Show</title><year>2024</year></tvshow>`),
		videoRecord("Test Show/Season 01/Test Show - S01E01.mkv"),
		videoRecord("Test Show/Season 01/Test Show - S01E02.mkv"),
		nfoRecord("Test Show/Season 01/Test Show - S01E02.nfo",
			`<episodedetails><title>The Second</title><season>1</season><episode>2</episode></episodedetails>`),
	}
	return []func(lib *testLibrary){
		func(lib *testLibrary) { lib.scan(first...) },
		func(lib *testLibrary) { lib.scan(first...) },
		func(lib *testLibrary) {
			// Arkon's folder renamed, Greed gone, a new episode, Metropolis's NFO retitled, and an episode's
			// NFO numbering it otherwise.
			changed := slices.Clone(first)
			changed[0] = nfoRecord("Metropolis (1927)/Metropolis (1927).nfo",
				`<movie><title>Metropolis (1927 cut)</title><year>1927</year>`+
					`<uniqueid type="tmdb">9019</uniqueid></movie>`)
			changed[4] = videoMoved("Arkon (1979)/Arkon (1979).mkv", "Arkon (1979) {edition-Director}/Arkon (1979).mkv")
			changed[9] = nfoRecord("Test Show/Season 01/Test Show - S01E02.nfo",
				`<episodedetails><title>The Second</title><season>1</season><episode>4</episode></episodedetails>`)
			changed = slices.Delete(changed, 5, 6)
			changed = append(changed, videoRecord("Test Show/Season 01/Test Show - S01E03.mkv"))
			lib.scan(changed...)
		},
		func(lib *testLibrary) {
			person := storetest.InsertUser(lib.t, lib.database, "alice", "admin")
			title := lib.titlesByPath()["Test Show/Season 01/Test Show - S01E01.mkv"].WorkID
			err := lib.chooser.SetOverride(lib.t.Context(), title, identify.FieldName, "The First", person)
			if err != nil {
				lib.t.Fatal(err)
			}
		},
		func(lib *testLibrary) {
			title := lib.titlesByPath()["Test Show/Season 01/Test Show - S01E01.mkv"].WorkID
			if err := lib.chooser.ClearOverride(lib.t.Context(), title, identify.FieldName); err != nil {
				lib.t.Fatal(err)
			}
		},
	}
}

func tmdbFilmsScenario() []func(lib *testLibrary) {
	films := []nodeproto.ScanRecord{videoRecord("Metropolis (1927).mkv"), videoRecord("Théo (1994)/Théo (1994).mkv")}
	return []func(lib *testLibrary){
		func(lib *testLibrary) {
			lib.provider.Searches[identifytest.SearchKey("Metropolis", 1927)] = []identify.Candidate{
				lib.provider.Add(metropolis),
			}
			lib.provider.Searches[identifytest.SearchKey("Théo", 1994)] = []identify.Candidate{lib.provider.Add(theo)}
			lib.scan(films...)
		},
		func(lib *testLibrary) {
			for _, rel := range []string{"Metropolis (1927).mkv", "Théo (1994)/Théo (1994).mkv"} {
				if outcome := lib.matchFileTitle(rel); outcome != identify.OutcomeMatched {
					lib.t.Fatalf("%s: %s, want matched", rel, outcome)
				}
			}
		},
		func(lib *testLibrary) { lib.scan(films...) },
		func(lib *testLibrary) {
			person := storetest.InsertUser(lib.t, lib.database, "alice", "admin")
			title := lib.titlesByPath()["Metropolis (1927).mkv"].WorkID
			if err := lib.chooser.SetOverride(lib.t.Context(), title, identify.FieldYear, "1928", person); err != nil {
				lib.t.Fatal(err)
			}
		},
		func(lib *testLibrary) {
			lib.scan(videoRecord("Metropolis (1927).mkv"), videoMoved("Théo (1994)/Théo (1994).mkv", "Théo (1994).mkv"))
		},
	}
}

func tmdbSeriesScenario() []func(lib *testLibrary) {
	episodes := []string{"Orbitorama/Season 02/Orbitorama - S02E10.mkv", "Orbitorama/Season 02/Orbitorama - S02E11.mkv"}
	return []func(lib *testLibrary){
		func(lib *testLibrary) {
			found := []identify.Candidate{lib.provider.Add(orbitorama)}
			lib.provider.Searches[identifytest.SearchKey("Orbitorama", 0)] = found
			lib.provider.Seasons["615/2"] = []identify.Episode{
				{ID: "1101", Season: 2, Number: 10, Name: "A Comet of My Own", Year: 2000},
				{ID: "1102", Season: 2, Number: 11, Name: "How Hermes Requisitioned His Groove Back", Year: 2000},
			}
			lib.scanAll(episodes...)
		},
		func(lib *testLibrary) {
			if _, err := lib.matcher.Match(lib.t.Context(), lib.onlySeries().ID); err != nil {
				lib.t.Fatal(err)
			}
			lib.runOwedPasses()
		},
		func(lib *testLibrary) { lib.scanAll(episodes...) },
		func(lib *testLibrary) {
			lib.provider.Seasons["615/2"] = append(lib.provider.Seasons["615/2"],
				identify.Episode{ID: "1103", Season: 2, Number: 12, Name: "Raging Bender", Year: 2000})
			lib.scanAll(append(slices.Clone(episodes), "Orbitorama/Season 02/Orbitorama - S02E12.mkv")...)
			if _, err := lib.matcher.Match(lib.t.Context(), lib.onlySeries().ID); err != nil {
				lib.t.Fatal(err)
			}
			lib.runOwedPasses()
		},
	}
}
