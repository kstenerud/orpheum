// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
)

// shownSeasonSeen is a series' answer's shown season as a test reads it: its number and its episodes'
// ids, and the season's own first page, GET /items/{id}/groups/{group}'s, raw, to compare it with.
type shownSeasonSeen struct {
	SeasonNumber int64             `json:"seasonNumber"`
	Episodes     []json.RawMessage `json:"episodes"`
	NextCursor   string            `json:"nextCursor"`
}

// shownSeasonOf is GET /items/20's shown season, the series shelveDVDOrder shelves, asked at query
// ("" for none).
func (harness testHarness) shownSeasonOf(t *testing.T, token, query string) *shownSeasonSeen {
	t.Helper()
	var answer struct {
		ShownSeason *shownSeasonSeen `json:"shownSeason"`
	}
	harness.readJSON(t, "/items/20"+query, token, &answer)
	return answer.ShownSeason
}

// A series' answer carries the season its page shows and that season's first page of episodes, as
// the season's own read answers it, so the page draws its episodes from its one read (D477 §1): the
// season asked for, where the series has it; else that of the person's next episode; else the first.
func TestItem_ASeriesCarriesTheShownSeasonsFirstPage(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t) // aired: 21 (file 2), 22 (3), 23 (4), then 24 (5) in season 2
	seasonPage := func(t *testing.T, season int64) shownSeasonSeen {
		t.Helper()
		var page shownSeasonSeen
		harness.readJSON(t, fmt.Sprintf("/items/20/groups/%d?limit=100", season), user, &page)
		page.SeasonNumber = season
		return page
	}
	same := func(t *testing.T, got *shownSeasonSeen, want shownSeasonSeen, when string) {
		t.Helper()
		if got == nil {
			t.Fatalf("%s: no shown season; want season %d", when, want.SeasonNumber)
		}
		sameEpisodes := slices.EqualFunc(got.Episodes, want.Episodes,
			func(a, b json.RawMessage) bool { return string(a) == string(b) })
		if got.SeasonNumber != want.SeasonNumber || got.NextCursor != want.NextCursor || !sameEpisodes {
			t.Errorf("%s: shown season %d %s (cursor %q); want season %d %s (cursor %q), its own read's",
				when, got.SeasonNumber, got.Episodes, got.NextCursor, want.SeasonNumber, want.Episodes, want.NextCursor)
		}
	}
	same(t, harness.shownSeasonOf(t, user, ""), seasonPage(t, 1), "nothing watched, none asked")
	same(t, harness.shownSeasonOf(t, user, "?season=2"), seasonPage(t, 2), "season 2 asked")
	harness.setPositionReadAt(t, 5, 500, 9) // partway through 24, the next episode now
	same(t, harness.shownSeasonOf(t, user, ""), seasonPage(t, 2), "partway through season 2, none asked")
	same(t, harness.shownSeasonOf(t, user, "?season=7"), seasonPage(t, 2), "a season it has not asked")
	same(t, harness.shownSeasonOf(t, user, "?season=1"), seasonPage(t, 1), "season 1 asked")
	if episodes := seasonPage(t, 1).Episodes; len(episodes) != 3 {
		t.Errorf("season 1's own read: %d episodes; want 3, so the comparison means something", len(episodes))
	}
}

// aroundSeen is what plays after a file and before it, in short: "next:title file / previous:title
// file", each "none" where nothing does.
type aroundSeen struct {
	UpNext *struct {
		Episode struct {
			ItemID int64 `json:"itemId"`
			FileID int64 `json:"fileId"`
		} `json:"episode"`
		Ordering string `json:"ordering"`
	} `json:"upNext"`
	Previous *struct {
		ItemID int64 `json:"itemId"`
		FileID int64 `json:"fileId"`
	} `json:"previous"`
}

func (around aroundSeen) String() string {
	next, previous := "none", "none"
	if around.UpNext != nil {
		next = fmt.Sprintf("%d file %d %s", around.UpNext.Episode.ItemID, around.UpNext.Episode.FileID,
			around.UpNext.Ordering)
	}
	if around.Previous != nil {
		previous = fmt.Sprintf("%d file %d", around.Previous.ItemID, around.Previous.FileID)
	}
	return "next " + next + " / previous " + previous
}

// An episode's answer carries what plays after it and before it, as GET /files/{id}/up-next says it
// of its first file, so its page names its next episode from its one read (D477 §2); a film's
// carries neither.
func TestItem_AnEpisodeCarriesWhatPlaysAfterItAndBefore(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t) // aired: 21 (file 2), 22 (3), 23 (4), then 24 (5) in season 2
	for titleID, fileID := range map[int64]int64{21: 2, 22: 3, 23: 4, 24: 5} {
		var inItem, ofFile aroundSeen
		harness.readJSON(t, fmt.Sprintf("/items/%d", titleID), user, &inItem)
		harness.readJSON(t, fmt.Sprintf("/files/%d/up-next", fileID), user, &ofFile)
		if inItem.String() != ofFile.String() {
			t.Errorf("episode %d's answer: %s; want %s, as its file's up-next", titleID, inItem, ofFile)
		}
	}
	var middle aroundSeen
	harness.readJSON(t, "/items/22", user, &middle)
	if got, want := middle.String(), "next 23 file 4 aired / previous 21 file 2"; got != want {
		t.Errorf("episode 22's answer: %s; want %s", got, want)
	}
}

// Each of a title's files carries where its person's playback of it starts, by the rule the player's
// start reads it (D477 §3): partway through, where they left it; at its start, or before its
// threshold, nothing.
func TestItem_EachFileCarriesWhereItResumes(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveDVDOrder(t) // each episode's file is 1000 ms long
	harness.setPositionReadAt(t, 3, 500, 9)
	harness.setPositionReadAt(t, 4, 5, 9) // half a percent: not partway through
	resumeOf := func(t *testing.T, titleID int64) int64 {
		t.Helper()
		var answer struct {
			Versions []struct {
				Files []struct {
					ResumePositionMs int64 `json:"resumePositionMs"`
				} `json:"files"`
			} `json:"versions"`
		}
		harness.readJSON(t, fmt.Sprintf("/items/%d", titleID), user, &answer)
		if len(answer.Versions) != 1 || len(answer.Versions[0].Files) != 1 {
			t.Fatalf("title %d: %d versions; want one of one file", titleID, len(answer.Versions))
		}
		return answer.Versions[0].Files[0].ResumePositionMs
	}
	for titleID, want := range map[int64]int64{22: 500, 23: 0, 24: 0} {
		if got := resumeOf(t, titleID); got != want {
			t.Errorf("title %d's file resumes at %d ms; want %d", titleID, got, want)
		}
	}
	harness.mark(t, http.MethodPut, "/items/22/watched", user)
	if got := resumeOf(t, 22); got != 0 {
		t.Errorf("marked by hand since: resumes at %d ms; want 0, a fresh start (D370 §4)", got)
	}
}
