// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/interest"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// ambiguousTitle adds title 10, a film whose search found two Solarises.
func (harness testHarness) ambiguousTitle(t *testing.T) {
	t.Helper()
	librarytest.NodeWithLibrary(t, harness.database,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "tmdb"})
	// With a live file: a title whose files are gone waits for no one.
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO item (id, kind, library_id, added_at) VALUES (10, 'title', 1, 1)`,
		`INSERT INTO title (id, kind, year, identification_state) VALUES (10, 'movie', 2003, 'ambiguous')`,
		`INSERT INTO item_text (item_id, language, is_original, name) VALUES (10, 'und', 1, 'The General')`,
		"INSERT INTO version (id, title_id, created_at) VALUES (1, 10, 1)",
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, filing, version_id, first_seen_at,
		 last_seen_at) VALUES (1, 1, 1, '/m/The General.mkv', 1000, X'01', 'listed', 1, 1, 1)`)
	err := harness.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		for rank, year := range []int64{1972, 2002} {
			err := queries.InsertMatchCandidate(t.Context(), cachegen.InsertMatchCandidateParams{
				ItemID: 10, Rank: int64(rank), Provider: "tmdb", ProviderID: []string{"1", "2"}[rank],
				Name: new("The General"), Year: &year,
			})
			if err != nil {
				return err //nolint:wrapcheck // the test reads it as is
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// otherVideoTitle adds title 20, an other library's video, identified from its file and never
// owed a match (D349 sections 4 and 10).
func (harness testHarness) otherVideoTitle(t *testing.T) {
	t.Helper()
	librarytest.NodeWithLibrary(t, harness.database,
		librarytest.Library{Name: "Home movies", Kind: "other", MetadataSource: "nfo"})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO item (id, kind, library_id, added_at) VALUES (20, 'title', 1, 1)`,
		`INSERT INTO title (id, kind, identification_state) VALUES (20, 'other', 'identified')`,
		`INSERT INTO item_text (item_id, language, is_original, name) VALUES (20, 'und', 1, 'Beach day')`,
		"INSERT INTO version (id, title_id, created_at) VALUES (2, 20, 1)",
		`INSERT INTO file (id, library_id, node_id, path, size_bytes, payload_hash, filing, version_id, first_seen_at,
		 last_seen_at) VALUES (2, 1, 1, '/m/Beach day.mp4', 1000, X'02', 'listed', 2, 1, 1)`,
		`INSERT INTO other_video (title_id, recorded_at, recorded_from, file_name, file_name_key)
		 VALUES (20, 1500000000000, 'name', 'Beach day.mp4', X'00')`)
}

// An other video's recorded date is a field a person may override, and it is never matched online
// (D349 sections 4 and 10): a field the enum once refused, and an error the handler once passed
// through unmapped.
func TestIdentification_AnOtherVideosDateIsOverriddenAndItIsNeverMatched(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.otherVideoTitle(t)

	set := harness.do(t, http.MethodPut, "/items/20/overrides/recorded_at", admin,
		map[string]string{"value": "1600000000000"})
	if set.status != http.StatusNoContent {
		t.Fatalf("PUT an other video's recorded_at override = %d %s", set.status, set.body)
	}

	matched := harness.do(t, http.MethodPost, "/items/20/match", admin,
		map[string]string{"provider": "tmdb", "providerId": "2"})
	wantInvalidAt(t, matched, http.StatusUnprocessableEntity, "path.id")
}

// An other video is never owed a match (D349 section 10), so it is left off the list identified
// titles wait on, however identified it is.
func TestIdentification_AnOtherVideoIsNeverListedAmongTitlesWaitingOnAMatch(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.otherVideoTitle(t)

	listed := harness.do(t, http.MethodGet, "/items?identificationState=identified", admin, nil)
	if listed.status != http.StatusOK {
		t.Fatalf("GET /items?identificationState=identified = %d %s", listed.status, listed.body)
	}
	if strings.Contains(listed.body, `"id":20`) {
		t.Errorf("an other video listed among identified titles: %s; want it left off (D349 §10)", listed.body)
	}
}

func TestIdentification_APersonChoosesAnAmbiguousTitlesMatch(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)

	listed := harness.do(t, http.MethodGet, "/items?identificationState=ambiguous", admin, nil)
	if listed.status != http.StatusOK || !strings.Contains(listed.body, `"id":10`) ||
		!strings.Contains(listed.body, `"identificationState":"ambiguous"`) {
		t.Fatalf("GET /items?identificationState=ambiguous = %d %s", listed.status, listed.body)
	}
	candidates := harness.do(t, http.MethodGet, "/items/10/candidates", admin, nil)
	const second = `{"providerId":"2","name":"The General","year":2002}`
	if candidates.status != http.StatusOK || !strings.Contains(candidates.body, second) ||
		!strings.Contains(candidates.body, `"provider":"tmdb"`) {
		t.Fatalf("GET /items/10/candidates = %d %s", candidates.status, candidates.body)
	}
	chosen := harness.do(t, http.MethodPost, "/items/10/match", admin,
		map[string]string{"provider": "tmdb", "providerId": "2"})
	if chosen.status != http.StatusNoContent {
		t.Fatalf("POST /items/10/match = %d %s", chosen.status, chosen.body)
	}
	confirmed := harness.do(t, http.MethodGet, "/items?identificationState=confirmed", admin, nil)
	if !strings.Contains(confirmed.body, `"id":10`) || !strings.Contains(confirmed.body, `"followedSourceId":"2"`) ||
		!strings.Contains(confirmed.body, `"year":2002`) {
		t.Errorf("after choosing: %s", confirmed.body)
	}
	wantInvalidAt(t, harness.do(t, http.MethodPost, "/items/10/match", admin,
		map[string]string{"provider": "tmdb", "providerId": "404"}), http.StatusUnprocessableEntity, "body.providerId")
	wantInvalidAt(t, harness.do(t, http.MethodPost, "/items/10/match", admin,
		map[string]string{"provider": "imdb", "providerId": "2"}), http.StatusUnprocessableEntity, "body.provider")
	wantError(t, harness.do(t, http.MethodPost, "/items/99/match", admin,
		map[string]string{"provider": "tmdb", "providerId": "2"}), http.StatusNotFound, api.CodeNotFound)
	wantError(t, harness.do(t, http.MethodGet, "/items?identificationState=maybe", admin, nil),
		http.StatusUnprocessableEntity, api.CodeInvalidRequest)
}

func TestIdentification_OverridesAreSetAndRemoved(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	set := harness.do(t, http.MethodPut, "/items/10/overrides/year", admin, map[string]string{"value": "2002"})
	if set.status != http.StatusNoContent {
		t.Fatalf("PUT an override = %d %s", set.status, set.body)
	}
	soon := harness.do(t, http.MethodPut, "/items/10/overrides/year", admin, map[string]string{"value": "soon"})
	wantInvalidAt(t, soon, http.StatusUnprocessableEntity, "body.value")
	if removed := harness.do(t, http.MethodDelete, "/items/10/overrides/year", admin, nil); removed.status !=
		http.StatusNoContent {
		t.Errorf("DELETE the override = %d %s", removed.status, removed.body)
	}
	wantError(t, harness.do(t, http.MethodDelete, "/items/10/overrides/year", admin, nil),
		http.StatusNotFound, api.CodeNotFound)
}

func TestIdentification_OnlyARoleThatEditsMetadataMayChange(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	user := harness.signIn(t, "partner", "partner-password")
	if listed := harness.do(t, http.MethodGet, "/items?identificationState=ambiguous", user,
		nil); listed.status != http.StatusOK {
		t.Errorf("a user listing ambiguous titles = %d %s; the question mark is everyone's to see", listed.status,
			listed.body)
	}
	for _, request := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/items/10/candidates", nil},
		{http.MethodPost, "/items/10/match", map[string]string{"provider": "tmdb", "providerId": "2"}},
		{http.MethodPut, "/items/10/overrides/name", map[string]string{"value": "Mine"}},
		{http.MethodDelete, "/items/10/overrides/name", nil},
		{http.MethodPost, "/items/10/refresh", nil},
	} {
		wantError(t, harness.do(t, request.method, request.path, user, request.body), http.StatusForbidden,
			api.CodeForbidden)
	}
	wantError(t, harness.do(t, http.MethodGet, "/items?identificationState=ambiguous", "", nil),
		http.StatusUnauthorized,
		api.CodeAuthRequired)
}

func TestIdentification_APersonMayRefreshATitleWheneverTheyLike(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	first := harness.do(t, http.MethodPost, "/items/10/refresh", admin, nil)
	if first.status != http.StatusAccepted || !strings.Contains(first.body, `"isQueued":true`) {
		t.Errorf("a refresh = %d %s; want it queued", first.status, first.body)
	}
	again := harness.do(t, http.MethodPost, "/items/10/refresh", admin, nil)
	if again.status != http.StatusAccepted || !strings.Contains(again.body, `"isQueued":false`) {
		t.Errorf("a second refresh while the first waits = %d %s; want accepted, already queued", again.status,
			again.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/items/99/refresh", admin, nil), http.StatusNotFound, api.CodeNotFound)
}

func TestIdentification_AnItemsOwnFailureIsShownOnItsPagesAndARefreshTriesAgain(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO fetch_failure (item_id, failure, failed_at) VALUES (10, 'TMDB answered 500 five times', 1234)`)
	const failure = `"fetchFailure":{"failure":"TMDB answered 500 five times","failedAt":1234}`
	for _, path := range []string{"/items?identificationState=ambiguous", "/items/10"} {
		if shown := harness.do(t, http.MethodGet, path, admin, nil); !strings.Contains(shown.body, failure) {
			t.Errorf("GET %s = %d %s; want its failure and when (D231 section 4)", path, shown.status, shown.body)
		}
	}
	harness.clock.Set(time.UnixMilli(5678))
	if tried := harness.do(t, http.MethodPost, "/items/10/refresh", admin, nil); tried.status != http.StatusAccepted {
		t.Fatalf("try again = %d %s", tried.status, tried.body)
	}
	shown := harness.do(t, http.MethodGet, "/items/10", admin, nil)
	if strings.Contains(shown.body, "fetchFailure") || !strings.Contains(shown.body, `"refreshRequestedAt":5678`) {
		t.Errorf("after try again: %s; want the failure cleared and the request shown as waiting", shown.body)
	}
}

func TestIdentification_AProvidersRateLimitSaysWhichAndHowLongAsSubtitlesDo(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	harness.tmdbProvider.SetFailure(&outbound.RateLimitedError{Provider: "TMDB", RetryAfter: 3 * time.Second})
	answer := harness.do(t, http.MethodPost, "/items/10/match", admin,
		map[string]string{"provider": "tmdb", "providerId": "2"})
	wantErrorSaying(t, answer, http.StatusServiceUnavailable, api.CodeRateLimited, "TMDB asks us to wait")
	if !strings.Contains(answer.body, `"retryAfterMs":3000`) ||
		!strings.Contains(answer.body, `"failedHop":"provider"`) {
		t.Errorf("a rate limited choice: %s, want the wait and the provider as the failing leg (cut-a §5.2)",
			answer.body)
	}
}

// A provider key that is missing or refused, or a provider that is not answering, leaves the way a
// subtitle login does: why in detail.reason, and the provider as the failing leg (cut-a §5.2).
func TestIdentification_AMissingOrRefusedKeySaysWhyWithTheProviderAsTheFailingLeg(t *testing.T) {
	t.Parallel()
	for failure, reason := range map[error]string{
		identify.ErrNoKey: "no_key", identify.ErrKeyRefused: "key_refused",
		fmt.Errorf("tmdb /movie/2: %w: status 502", outbound.ErrUnavailable): "unavailable",
	} {
		harness := newHarness(t)
		admin := harness.createAdminAndSignIn(t)
		harness.ambiguousTitle(t)
		harness.tmdbProvider.SetFailure(failure)
		answer := harness.do(t, http.MethodPost, "/items/10/match", admin,
			map[string]string{"provider": "tmdb", "providerId": "2"})
		wantError(t, answer, http.StatusServiceUnavailable, api.CodeProviderError)
		if !strings.Contains(answer.body, `"reason":"`+reason+`"`) ||
			!strings.Contains(answer.body, `"failedHop":"provider"`) {
			t.Errorf("%v: %s, want reason %s and the provider as the failing leg", failure, answer.body, reason)
		}
	}
}

// A provider not answering says since when, and when it is next looked at, as its provider service
// knows it, so a person is told how long it has been and when to try again (D231 §12).
func TestIdentification_AProviderNotAnsweringSaysSinceWhenAndTheNextCheck(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	harness.queue.Park(t.Context(), jobs.Parking{Destination: jobs.DestinationTMDBAPI, Reason: "unreachable"})
	harness.tmdbProvider.SetFailure(fmt.Errorf("tmdb /movie/2: %w: status 502", outbound.ErrUnavailable))
	answer := harness.do(t, http.MethodPost, "/items/10/match", admin,
		map[string]string{"provider": "tmdb", "providerId": "2"})
	since, next := harness.clock.Now().UnixMilli(), harness.clock.Now().Add(30*time.Second).UnixMilli()
	want := fmt.Sprintf(`"detail":{"reason":"unavailable","notAnsweringAt":%d,"nextCheckAt":%d}`, since, next)
	if answer.status != http.StatusServiceUnavailable || !strings.Contains(answer.body, want) {
		t.Errorf("a match while TMDB is down = %d %s; want %s", answer.status, answer.body, want)
	}
}

// A refresh asked for while TMDB is not answering says so, since when and when it is next looked
// at, beside being queued (D231 §12).
func TestIdentification_ARefreshWhileTMDBIsDownSaysWhatItWaitsFor(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	fine := harness.do(t, http.MethodPost, "/items/10/refresh", admin, nil)
	if strings.Contains(fine.body, "waitingFor") {
		t.Errorf("a refresh while TMDB answers = %s; want nothing waited for", fine.body)
	}
	harness.queue.Park(t.Context(), jobs.Parking{Destination: jobs.DestinationTMDBAPI, Reason: "unreachable"})
	refreshed := harness.do(t, http.MethodPost, "/items/10/refresh", admin, nil)
	want := fmt.Sprintf(`"waitingFor":{"destination":"tmdb-api","name":"TMDB","reason":"unreachable","downAt":%d`,
		harness.clock.Now().UnixMilli())
	if refreshed.status != http.StatusAccepted || !strings.Contains(refreshed.body, want) {
		t.Errorf("a refresh while TMDB is down = %d %s; want %s", refreshed.status, refreshed.body, want)
	}
}

// A person matches a title by pasting its address at TMDB, and a paste that names no work of its
// kind is refused beside the field it was typed in (the owner's staging, 2026-10-21).
func TestIdentification_APersonMatchesATitleByAPastedLink(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	for _, refused := range []string{"The General", "https://www.themoviedb.org/tv/2-solaris"} {
		wantInvalidAt(t, harness.do(t, http.MethodPost, "/items/10/match-by-link", admin,
			map[string]string{"link": refused}), http.StatusUnprocessableEntity, "body.link")
	}
	chosen := harness.do(t, http.MethodPost, "/items/10/match-by-link", admin,
		map[string]string{"link": "https://www.themoviedb.org/movie/2-solaris"})
	if chosen.status != http.StatusNoContent {
		t.Fatalf("POST /items/10/match-by-link = %d %s", chosen.status, chosen.body)
	}
	confirmed := harness.do(t, http.MethodGet, "/items?identificationState=confirmed", admin, nil)
	if !strings.Contains(confirmed.body, `"id":10`) || !strings.Contains(confirmed.body, `"followedSourceId":"2"`) {
		t.Errorf("after pasting TMDB's address: %s", confirmed.body)
	}
}

// A session says the page it shows, for its work to come first, and that it left it (D324 §1); a
// page it does not reach is refused (TestReach_AnEditorReachesOnlyTheLibrariesTheirRoleLists).
func TestInterest_ASessionSaysThePageItShowsAndLeavesIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	if shown := harness.do(t, http.MethodPut, "/interest/10", admin, nil); shown.status != http.StatusNoContent {
		t.Fatalf("PUT /interest/10 = %d %s", shown.status, shown.body)
	}
	if looks := harness.looks.Looks(time.Minute); len(looks) != 1 || looks[0].ItemID != 10 {
		t.Errorf("looks %+v; want the session at film 10", looks)
	}
	if left := harness.do(t, http.MethodDelete, "/interest/10", admin, nil); left.status != http.StatusNoContent {
		t.Fatalf("DELETE /interest/10 = %d %s", left.status, left.body)
	}
	if looks := harness.looks.Looks(time.Minute); len(looks) != 0 {
		t.Errorf("looks %+v after leaving; want none", looks)
	}
}

// A series' page that shows a season says which, and its look is that season's, which leaving the
// series' own page does not end (the owner, 2026-10-24); without one, it is the series' own.
func TestInterest_ASeasonsPageSaysItsSeason(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.ambiguousTitle(t)
	shown := harness.do(t, http.MethodPut, "/interest/10?season=2", admin, nil)
	if shown.status != http.StatusNoContent {
		t.Fatalf("PUT /interest/10?season=2 = %d %s", shown.status, shown.body)
	}
	if looks := harness.looks.Looks(time.Minute); len(looks) != 1 || looks[0].ItemID != 10 || looks[0].Season != 2 {
		t.Errorf("looks %+v; want the session at item 10's season 2", looks)
	}
	if left := harness.do(t, http.MethodDelete, "/interest/10", admin, nil); left.status != http.StatusNoContent {
		t.Fatalf("DELETE /interest/10 = %d %s", left.status, left.body)
	}
	if looks := harness.looks.Looks(time.Minute); len(looks) != 1 {
		t.Errorf("looks %+v after leaving the page without a season; want season 2's still", looks)
	}
	if shown = harness.do(t, http.MethodPut, "/interest/10", admin, nil); shown.status != http.StatusNoContent {
		t.Fatalf("PUT /interest/10 = %d %s", shown.status, shown.body)
	}
	if looks := harness.looks.Looks(time.Minute); len(looks) != 1 || looks[0].Season != interest.NoSeason {
		t.Errorf("looks %+v; want the item's own page, no season", looks)
	}
}
