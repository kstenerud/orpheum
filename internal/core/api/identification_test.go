// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// ambiguousTitle adds title 10, a film whose search found two Solarises.
func (harness testHarness) ambiguousTitle(t *testing.T) {
	t.Helper()
	librarytest.LibraryOn(t, harness.database, 0,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "tmdb"})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO item (id, kind, library_id, added_at) VALUES (10, 'title', 1, 1)`,
		`INSERT INTO title (id, kind, year, identification_state) VALUES (10, 'movie', 2003, 'ambiguous')`,
		`INSERT INTO item_text (item_id, language, is_original, name) VALUES (10, 'und', 1, 'The General')`)
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
		map[string]string{"provider": "tmdb", "providerId": "2"}), http.StatusNotFound, CodeNotFound)
	wantError(t, harness.do(t, http.MethodGet, "/items?identificationState=maybe", admin, nil),
		http.StatusUnprocessableEntity, CodeInvalidRequest)
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
		http.StatusNotFound, CodeNotFound)
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
			CodeForbidden)
	}
	wantError(t, harness.do(t, http.MethodGet, "/items?identificationState=ambiguous", "", nil),
		http.StatusUnauthorized,
		CodeAuthRequired)
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
	wantError(t, harness.do(t, http.MethodPost, "/items/99/refresh", admin, nil), http.StatusNotFound, CodeNotFound)
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
	wantErrorSaying(t, answer, http.StatusServiceUnavailable, CodeRateLimited, "TMDB asks us to wait")
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
		wantError(t, answer, http.StatusServiceUnavailable, CodeProviderError)
		if !strings.Contains(answer.body, `"reason":"`+reason+`"`) ||
			!strings.Contains(answer.body, `"failedHop":"provider"`) {
			t.Errorf("%v: %s, want reason %s and the provider as the failing leg", failure, answer.body, reason)
		}
	}
}
