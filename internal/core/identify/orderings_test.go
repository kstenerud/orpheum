// SPDX-License-Identifier: AGPL-3.0-or-later

package identify_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/identify/identifytest"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/jobs/jobstest"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

var emberwing = identify.Metadata{ID: "4807", Name: "Emberwing", Year: 2002}

// emberwingFiles are four episodes of Emberwing's one season, named in aired order.
var emberwingFiles = []string{
	"Emberwing/Season 01/Emberwing - S01E01.mkv", "Emberwing/Season 01/Emberwing - S01E02.mkv",
	"Emberwing/Season 01/Emberwing - S01E03.mkv", "Emberwing/Season 01/Emberwing - S01E04.mkv",
}

// emberwingLibrary is a TMDB library of Emberwing's four episodes, TMDB listing them in season 1 as e1
// to e4, and two of its episode groups (episode-orderings.md §2.1): a DVD order of all four, and an
// intended order, a part named "Intended" of e3, e1 and e2, which leaves e4 out.
func emberwingLibrary(t *testing.T) *testLibrary {
	t.Helper()
	lib := newTestLibrary(t, "series", "tmdb")
	lib.provider.Searches[identifytest.SearchKey("Emberwing", 0)] = []identify.Candidate{lib.provider.Add(emberwing)}
	for number := 1; number <= 4; number++ {
		lib.provider.Seasons["4807/1"] = append(lib.provider.Seasons["4807/1"], identify.Episode{
			ID: fmt.Sprintf("e%d", number), Season: 1, Number: number, Name: fmt.Sprintf("Episode %d", number),
		})
	}
	lib.provider.EpisodeGroupLists["4807"] = []identify.EpisodeGroup{
		{ID: "dvd", Name: "DVD Order", Kind: "dvd", EpisodeCount: 4},
		{ID: "intended", Name: "Intended Order", Kind: "absolute", EpisodeCount: 3},
	}
	intended := []identify.EpisodeGroupEntry{
		{EpisodeID: "e3", Order: 0}, {EpisodeID: "e1", Order: 1}, {EpisodeID: "e2", Order: 2},
	}
	lib.provider.EpisodeGroups["intended"] = []identify.EpisodeGroupPart{
		{Order: 1, Name: "Intended", Episodes: intended},
	}
	return lib
}

// matchEmberwing scans the library's files and matches its series, as the queue's first match would.
func (lib *testLibrary) matchEmberwing(files ...string) int64 {
	lib.t.Helper()
	lib.scanAll(files...)
	series := lib.onlySeries()
	if _, err := lib.matcher.Match(lib.t.Context(), series.ID); err != nil {
		lib.t.Fatal(err)
	}
	lib.runOwedPasses()
	return series.ID
}

// orderingsOf are a series' orderings as "name kind label listed".
func (lib *testLibrary) orderingsOf(seriesID int64) []string {
	lib.t.Helper()
	return storetest.QueryRaw(lib.t, lib.database.Path(), fmt.Sprintf(
		`SELECT name, coalesce(kind, ''), coalesce(label, ''), coalesce(listed_count, 0) FROM ordering
		 WHERE sequence_id = %d ORDER BY name`, seriesID))
}

// placesIn are an ordering's places as "file group number", in its order, and its groups' names.
func (lib *testLibrary) placesIn(seriesID int64, ordering string) (places, groupNames []string) {
	lib.t.Helper()
	places = storetest.QueryRaw(lib.t, lib.database.Path(), fmt.Sprintf(
		`SELECT substr(place.path, length(place.path) - 9), grp.number, position.number_in_group
		 FROM ordering JOIN position ON position.ordering_id = ordering.id
		 LEFT JOIN grp ON grp.id = position.grp_id
		 JOIN version ON version.work_id = position.work_id JOIN file_filing ON file_filing.version_id = version.id
		 JOIN place ON place.id = file_filing.file_id
		 WHERE ordering.sequence_id = %d AND ordering.name = '%s' ORDER BY position.sort_order`, seriesID, ordering))
	groupNames = storetest.QueryRaw(lib.t, lib.database.Path(), fmt.Sprintf(
		`SELECT grp.number, group_text.name FROM ordering
		 JOIN grp ON grp.ordering_id = ordering.id JOIN group_text ON group_text.grp_id = grp.id
		 WHERE ordering.sequence_id = %d AND ordering.name = '%s'`, seriesID, ordering))
	return places, groupNames
}

// wantPlaces checks the intended order's places, as placesIn says them, are want.
func (lib *testLibrary) wantPlaces(seriesID int64, when string, want []string) {
	lib.t.Helper()
	if places, _ := lib.placesIn(seriesID, "tmdb:intended"); !slices.Equal(places, want) {
		lib.t.Errorf("%s: places %q; want %q", when, places, want)
	}
}

// choose keeps user 1's choice of an ordering of a series, as the preferences service does.
func (lib *testLibrary) choose(seriesID int64, ordering string) {
	lib.t.Helper()
	storetest.ExecRaw(lib.t, lib.database.Path(), fmt.Sprintf(
		`INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
		 VALUES (1, 'item', %d, 'episode_ordering', '"%s"', 1)
		 ON CONFLICT (user_id, scope, scope_id, axis) DO UPDATE SET value_json = excluded.value_json`,
		seriesID, ordering))
}

// A series' match lists its episode groups from its own record, with no request of their own, each
// an ordering with its kind, label and how many episodes TMDB lists in it; a group's contents are
// fetched once someone chose it, and once only, and place the series' episodes as the group does,
// its part's name the group's; an episode it leaves out has no place in it (episode-orderings.md
// §3.2, §3.4; D170).
func TestOrderings_AChosenEpisodeGroupPlacesTheSeriesEpisodesAsTheGroupDoes(t *testing.T) {
	t.Parallel()
	lib := emberwingLibrary(t)
	// S01E02 comes later.
	seriesID := lib.matchEmberwing(emberwingFiles[0], emberwingFiles[2], emberwingFiles[3])
	if want := []string{"search Emberwing|0", "fetch 4807", "season 4807/1"}; !slices.Equal(lib.provider.Calls, want) {
		t.Errorf("calls %v; want %v, the groups listed by the series' own record", lib.provider.Calls, want)
	}
	want := []string{"aired|aired||0", "tmdb:dvd|dvd|DVD Order|4", "tmdb:intended|absolute|Intended Order|3"}
	if got := lib.orderingsOf(seriesID); !slices.Equal(got, want) {
		t.Errorf("orderings %q; want %q", got, want)
	}
	lib.wantPlaces(seriesID, "the intended order, chosen by no one: not fetched, holding none", nil)
	storetest.InsertUser(t, lib.database, "owner", "admin")
	lib.choose(seriesID, "tmdb:intended")
	calls := len(lib.provider.Calls)
	queue := &jobstest.Queue{}
	refresher := identify.NewRefresher(lib.database, queue, lib.settings, lib.clock, lib.logger)
	if queued, err := refresher.QueueChosenOrderings(t.Context(), seriesID); err != nil || !queued ||
		(*queue)[0].Priority != jobs.PriorityPersonsRequest {
		t.Fatalf("a choice of a group not fetched: queued %v %+v (%v); want its fetch, at the person's priority",
			queued, *queue, err)
	}
	for range 2 { // the job the queue runs, and again
		if _, err := lib.matcher.Match(t.Context(), seriesID); err != nil {
			t.Fatal(err)
		}
		lib.runOwedPasses()
	}
	if got := lib.provider.Calls[calls:]; !slices.Equal(got, []string{"episode_group intended"}) {
		t.Errorf("calls %v; want the chosen group fetched once", got)
	}
	lib.wantPlaces(seriesID, "the intended order, S01E04 in none", []string{"S01E03.mkv|1|1", "S01E01.mkv|1|2"})
	if _, names := lib.placesIn(seriesID, "tmdb:intended"); !slices.Equal(names, []string{"1|Intended"}) {
		t.Errorf("its groups' names %q; want its part's", names)
	}
	if queued, err := refresher.QueueChosenOrderings(t.Context(), seriesID); err != nil || queued {
		t.Errorf("a choice of a group fetched: queued %v (%v); want nothing to fetch", queued, err)
	}
	// A pass keeps placing the episodes: one that arrives takes its place in the order.
	lib.scanAll(emberwingFiles...)
	lib.wantPlaces(seriesID, "after S01E02 arrived, third in the order",
		[]string{"S01E03.mkv|1|1", "S01E01.mkv|1|2", "S01E02.mkv|1|3"})
	if got := lib.provider.Calls[calls+1:]; len(got) != 0 {
		t.Errorf("calls %v after it arrived; want none, its season listed it", got)
	}
}

// A series fetched before its record carried its episode groups has them filled in by the sweep,
// passively, a few at a time behind other work, from a request of their own; one whose fetch gave
// up waits for a person, and is not asked for again (D335 §1, §3; D231 §4).
func TestOrderings_ASeriesFetchedBeforeItsGroupsWereHasThemFilledInByTheSweep(t *testing.T) {
	t.Parallel()
	lib := emberwingLibrary(t)
	lib.provider.RecordsWithoutGroups["4807"] = true
	lib.provider.SetFailure(identify.ErrKeyRefused) // matched, as before this was built, but no list fetched
	lib.scanAll(emberwingFiles...)
	seriesID := lib.onlySeries().ID
	lib.provider.SetFailure(nil)
	queue := &jobstest.Queue{}
	refresher := identify.NewRefresher(lib.database, queue, lib.settings, lib.clock, lib.logger)
	if _, err := lib.matcher.Match(t.Context(), seriesID); err != nil {
		t.Fatal(err)
	}
	storetest.ExecRaw(t, lib.database.Path(),
		"DELETE FROM provider_response WHERE part = 'episode_groups'",
		"DELETE FROM ordering WHERE name <> 'aired'")
	if count, err := refresher.QueueMissing(t.Context()); err != nil || count != 1 || (*queue)[0].ItemID != seriesID ||
		(*queue)[0].Priority >= 0 {
		t.Fatalf("the sweep queued %d %+v (%v); want the series' fetch, behind other work", count, *queue, err)
	}
	calls := len(lib.provider.Calls)
	if _, err := lib.matcher.Match(t.Context(), seriesID); err != nil {
		t.Fatal(err)
	}
	if got := lib.provider.Calls[calls:]; !slices.Equal(got, []string{"episode_groups 4807"}) {
		t.Errorf("calls %v; want the list alone, nothing stored fetched again", got)
	}
	if got := lib.orderingsOf(seriesID); len(got) != 3 {
		t.Errorf("orderings %q; want aired and the two groups listed", got)
	}
	*queue = nil
	if count, err := refresher.QueueMissing(t.Context()); err != nil || count != 0 {
		t.Errorf("filled in: the sweep queued %d (%v); want nothing", count, err)
	}
	storetest.ExecRaw(t, lib.database.Path(), "DELETE FROM provider_response WHERE part = 'episode_groups'")
	job := jobs.Job{ID: 7, Kind: jobs.KindFetchMetadata, Target: jobs.Target{ItemID: seriesID}}
	if err := lib.matcher.RecordFailure(t.Context(), job, "TMDB answered 500 five times"); err != nil {
		t.Fatal(err)
	}
	if count, err := refresher.QueueMissing(t.Context()); err != nil || count != 0 {
		t.Errorf("given up: the sweep queued %d (%v); want it left for a person", count, err)
	}
}

// Principles §22, D211: an episode group's label and its parts' names are clean where they enter,
// as any provider text is.
func TestOrderings_AGroupsNamesAreCleanWhereTheyEnter(t *testing.T) {
	t.Parallel()
	lib := emberwingLibrary(t)
	lib.provider.EpisodeGroupLists["4807"][0].Name = "DVD\u202EOrder"
	lib.provider.EpisodeGroups["dvd"] = []identify.EpisodeGroupPart{
		{Order: 1, Name: "Disc\u200B 1", Episodes: []identify.EpisodeGroupEntry{{EpisodeID: "e1", Order: 0}}},
	}
	seriesID := lib.matchEmberwing(emberwingFiles...)
	storetest.InsertUser(t, lib.database, "owner", "admin")
	lib.choose(seriesID, "tmdb:dvd")
	if _, err := lib.matcher.Match(t.Context(), seriesID); err != nil {
		t.Fatal(err)
	}
	labels := storetest.QueryRaw(t, lib.database.Path(), "SELECT label FROM ordering WHERE name = 'tmdb:dvd'")
	names := storetest.QueryRaw(t, lib.database.Path(), "SELECT name FROM group_text")
	if !slices.Equal(labels, []string{"DVDOrder"}) || !slices.Equal(names, []string{"Disc 1"}) {
		t.Errorf("label %q and group names %q; want them cleaned", labels, names)
	}
}

// A series' refresh fetches again each episode group it fetched, as it does each season, takes its
// list again from its new record, at no request of its own, and places its episodes as the group
// now does: a group TMDB's contributors changed moves them (episode-orderings.md §3.4, §7 Q11).
func TestOrderings_ARefreshFetchesEachGroupItKeepsAgain(t *testing.T) {
	t.Parallel()
	lib := emberwingLibrary(t)
	seriesID := lib.matchEmberwing(emberwingFiles...)
	storetest.InsertUser(t, lib.database, "owner", "admin")
	lib.choose(seriesID, "tmdb:intended")
	if _, err := lib.matcher.Match(t.Context(), seriesID); err != nil {
		t.Fatal(err)
	}
	lib.provider.EpisodeGroups["intended"][0].Episodes = []identify.EpisodeGroupEntry{
		{EpisodeID: "e1", Order: 0}, {EpisodeID: "e3", Order: 1},
	}
	calls := len(lib.provider.Calls)
	lib.refresh(seriesID)
	lib.runOwedPasses()
	want := []string{"fetch 4807", "episode_group intended", "season 4807/1"}
	if got := lib.provider.Calls[calls:]; !slices.Equal(got, want) {
		t.Errorf("calls %v; want %v", got, want)
	}
	lib.wantPlaces(seriesID, "after the group changed", []string{"S01E01.mkv|1|1", "S01E03.mkv|1|2"})
}

// A matched series' aired order has a group for each season its provider lists, the specials and a
// season none of whose episodes is found among them, so that a season's poster has its group to name:
// identify makes the groups and artwork only looks them up (D426 section 5).
func TestOrderings_ASeriesAiredOrderHasAGroupForEachSeasonItsProviderLists(t *testing.T) {
	t.Parallel()
	lib := emberwingLibrary(t)
	listed := emberwing
	listed.SeasonNumbers = []int{0, 1, 2}
	lib.provider.Searches[identifytest.SearchKey("Emberwing", 0)] = []identify.Candidate{lib.provider.Add(listed)}
	seriesID := lib.matchEmberwing(emberwingFiles[0])
	groups := storetest.QueryRaw(t, lib.database.Path(), fmt.Sprintf(
		`SELECT grp.number FROM ordering JOIN grp ON grp.ordering_id = ordering.id
		 WHERE ordering.sequence_id = %d AND ordering.name = 'aired' ORDER BY grp.number`, seriesID))
	if want := []string{"0", "1", "2"}; !slices.Equal(groups, want) {
		t.Errorf("aired groups %q; want %q, one for each season listed", groups, want)
	}
}
