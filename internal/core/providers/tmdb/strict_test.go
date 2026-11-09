// SPDX-License-Identifier: AGPL-3.0-or-later

package tmdb

import (
	"errors"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// TMDB is another's sender (D458): a reply with one result's title said twice is read, keeping the
// last, the common rule.
func TestSearch_AReplyWithAKeySaidTwiceKeepsItsLastValue(t *testing.T) {
	t.Parallel()
	server, _ := fakeTMDB(t, map[string]string{"/search/movie": `{"page":1,"results":[
		{"id":4801,"title":"The Gold Rush","title":"Something Else","release_date":"1999-03-31"}]}`})
	query := identify.Query{Kind: "movie", Name: "The Gold Rush", Year: 1999}
	candidates, err := newClient(t, server.URL, "v3key").Search(t.Context(), query)
	if err != nil {
		t.Fatalf("a reply with a duplicate key: %v; want it read", err)
	}
	if len(candidates) != 1 || candidates[0].Name != "Something Else" {
		t.Errorf("candidates = %+v, want one named the last title, Something Else", candidates)
	}
}

// A reply past D456's limits is still refused from another's sender (D458 §2).
func TestSearch_AReplyNestedPastTheLimitIsRefused(t *testing.T) {
	t.Parallel()
	deep := strings.Repeat("[", strictjson.MaxDepth+1)
	server, _ := fakeTMDB(t, map[string]string{"/search/movie": `{"page":1,"results":` + deep})
	query := identify.Query{Kind: "movie", Name: "The Gold Rush", Year: 1999}
	_, err := newClient(t, server.URL, "v3key").Search(t.Context(), query)
	if !errors.Is(err, strictjson.ErrRefused) {
		t.Errorf("a reply nested past the limit: %v; want strictjson.ErrRefused", err)
	}
}
