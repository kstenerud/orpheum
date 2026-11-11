// SPDX-License-Identifier: AGPL-3.0-or-later

package tmdb

import (
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
)

// emberwingAtTMDB answers as TMDB does for Emberwing's record, which carries its episode groups when
// asked to, its list of groups and one group's contents (shapes from TMDB's API reference,
// episode-orderings.md §2.1; the ids are made up).
func emberwingAtTMDB(t *testing.T) *Client {
	t.Helper()
	server, seen := fakeTMDB(t, map[string]string{
		"/tv/7437": `{"id":7437,"name":"Emberwing","first_air_date":"2002-10-04",
			"episode_groups":{"id":7437,"results":[
				{"id":"5e90a","name":"Intended Order","description":"The intended order","type":2,
				 "episode_count":14,"group_count":2,"network":null},
				{"id":"5e90b","name":"DVD Order","description":"","type":3,"episode_count":23,"group_count":2,
				 "network":null}]}}`,
		"/tv/7437/episode_groups": `{"id":7437,"results":[{"id":"5e90a","name":"Intended Order","type":2,
			"episode_count":14,"group_count":2}]}`,
		"/tv/episode_group/5e90a": `{"id":"5e90a","name":"Intended Order","type":2,"episode_count":3,
			"group_count":2,"groups":[
				{"id":"g1","name":"Season 1","order":1,"locked":true,"episodes":[
					{"id":2003,"name":"Stillness","season_number":1,"episode_number":11,"order":0},
					{"id":2001,"name":"The Kettle Job","season_number":1,"episode_number":1,"order":1}]},
				{"id":"g0","name":"Specials","order":0,"locked":true,"episodes":[
					{"id":2900,"name":"Here's How It Went","season_number":0,"episode_number":1,"order":0}]}]}`,
	})
	t.Cleanup(func() {
		if got := (*seen)[0].URL.Query().Get("append_to_response"); got != "credits,external_ids,episode_groups" {
			t.Errorf("a series' fetch appended %q; want its episode groups too, at no request of their own", got)
		}
	})
	return newClient(t, server.URL, "k")
}

// A series' record carries its list of episode groups, the same as the list's own answer, each group
// with its kind by TMDB's type (episode-orderings.md §2.1).
func TestEpisodeGroups_ASeriesRecordCarriesItsListOfGroups(t *testing.T) {
	t.Parallel()
	client := emberwingAtTMDB(t)
	record, err := client.Fetch(t.Context(), identify.KindSeries, "7437")
	if err != nil {
		t.Fatal(err)
	}
	list, isCarried := client.EpisodeGroupsIn(record)
	if !isCarried {
		t.Fatal("the record carries no list of episode groups")
	}
	groups, err := client.ParseEpisodeGroups(list)
	want := []identify.EpisodeGroup{
		{ID: "5e90a", Name: "Intended Order", Kind: "absolute", EpisodeCount: 14},
		{ID: "5e90b", Name: "DVD Order", Kind: "dvd", EpisodeCount: 23},
	}
	if err != nil || !slices.Equal(groups, want) {
		t.Errorf("the record's groups = %+v, %v; want %+v", groups, err, want)
	}
	if _, isCarried := client.EpisodeGroupsIn([]byte(`{"id":7437,"name":"Emberwing"}`)); isCarried {
		t.Error("a record fetched before it carried its groups says it carries them")
	}
	alone, err := client.FetchEpisodeGroups(t.Context(), "7437")
	if err != nil {
		t.Fatal(err)
	}
	if groups, err = client.ParseEpisodeGroups(alone); err != nil || len(groups) != 1 || groups[0].Kind != "absolute" {
		t.Errorf("the list's own answer = %+v, %v", groups, err)
	}
}

// A group's contents are its parts, each with its name, its order and its episodes by their TMDB ids
// and their order (episode-orderings.md §2.1).
func TestEpisodeGroups_AGroupIsItsPartsAndTheirEpisodes(t *testing.T) {
	t.Parallel()
	client := emberwingAtTMDB(t)
	if _, err := client.Fetch(t.Context(), identify.KindSeries, "7437"); err != nil {
		t.Fatal(err)
	}
	body, err := client.FetchEpisodeGroup(t.Context(), "5e90a")
	if err != nil {
		t.Fatal(err)
	}
	parts, err := client.ParseEpisodeGroup(body)
	want := []identify.EpisodeGroupEntry{{EpisodeID: "2003", Order: 0}, {EpisodeID: "2001", Order: 1}}
	if err != nil || len(parts) != 2 || parts[0].Name != "Season 1" || parts[0].Order != 1 ||
		!slices.Equal(parts[0].Episodes, want) || parts[1].Order != 0 || parts[1].Episodes[0].EpisodeID != "2900" {
		t.Errorf("the group's parts = %+v, %v", parts, err)
	}
	if _, err := client.FetchEpisodeGroup(t.Context(), "../x"); err == nil {
		t.Error("an id that is not a group's was asked for")
	}
}
