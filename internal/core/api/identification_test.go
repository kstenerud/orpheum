// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// fakeProvider is TMDB behind the provider interface: it fetches the films it has, by id.
type fakeProvider struct {
	films map[string]identify.Metadata
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{films: map[string]identify.Metadata{
		"2": {ID: "2", Name: "The General", Year: 2002, Overview: "Soderbergh's."},
	}}
}

func (*fakeProvider) Name() string { return "tmdb" }

func (*fakeProvider) Search(context.Context, identify.Query) ([]identify.Candidate, error) {
	return []identify.Candidate{}, nil
}

func (*fakeProvider) Find(context.Context, string, string, string) (string, error) {
	return "", identify.ErrNoMatch
}

func (fake *fakeProvider) Fetch(_ context.Context, _, id string) ([]byte, error) {
	film, found := fake.films[id]
	if !found {
		return nil, identify.ErrNoMatch
	}
	return json.Marshal(film) //nolint:wrapcheck,musttag // a test fake stores Metadata as Go names it
}

func (*fakeProvider) FetchSeason(context.Context, string, int) ([]byte, error) {
	return nil, identify.ErrNoMatch
}

func (*fakeProvider) ParseSeason([]byte) ([]identify.Episode, error) {
	return []identify.Episode{}, nil
}

func (*fakeProvider) Parse(_ string, body []byte) (identify.Metadata, error) {
	var film identify.Metadata
	err := json.Unmarshal(body, &film) //nolint:musttag // as the fake stored it
	return film, err                   //nolint:wrapcheck // a test fake
}

// ambiguousTitle adds title 10, a film whose search found two Solarises.
func (harness testHarness) ambiguousTitle(t *testing.T) {
	t.Helper()
	storetest.ExecRaw(t, harness.database,
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, writable, created_at)
		 VALUES (1, 'Films', 'movies', 'tmdb', 'manual', 0, 1)`,
		`INSERT INTO item (id, kind, library_id, created_at) VALUES (10, 'title', 1, 1)`,
		`INSERT INTO title (id, kind, year, identification) VALUES (10, 'movie', 2003, 'ambiguous')`,
		`INSERT INTO item_text (item_id, lang, is_original, name) VALUES (10, 'und', 1, 'The General')`)
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

	listed := harness.do(t, http.MethodGet, "/items?identification=ambiguous", admin, nil)
	if listed.status != http.StatusOK || !strings.Contains(listed.body, `"id":10`) ||
		!strings.Contains(listed.body, `"identification":"ambiguous"`) {
		t.Fatalf("GET /items?identification=ambiguous = %d %s", listed.status, listed.body)
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
	confirmed := harness.do(t, http.MethodGet, "/items?identification=confirmed", admin, nil)
	if !strings.Contains(confirmed.body, `"id":10`) || !strings.Contains(confirmed.body, `"sourceId":"2"`) ||
		!strings.Contains(confirmed.body, `"year":2002`) {
		t.Errorf("after choosing: %s", confirmed.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/items/10/match", admin,
		map[string]string{"provider": "tmdb", "providerId": "404"}), http.StatusUnprocessableEntity, CodeInvalidRequest)
	wantError(t, harness.do(t, http.MethodPost, "/items/99/match", admin,
		map[string]string{"provider": "tmdb", "providerId": "2"}), http.StatusNotFound, CodeNotFound)
	wantError(t, harness.do(t, http.MethodGet, "/items?identification=maybe", admin, nil),
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
	wantError(t, harness.do(t, http.MethodPut, "/items/10/overrides/year", admin, map[string]string{"value": "soon"}),
		http.StatusUnprocessableEntity, CodeInvalidRequest)
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
	if _, err := harness.service.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	user := harness.signIn(t, "partner", "partner-password")
	if listed := harness.do(t, http.MethodGet, "/items?identification=ambiguous", user,
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
	wantError(t, harness.do(t, http.MethodGet, "/items?identification=ambiguous", "", nil), http.StatusUnauthorized,
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
