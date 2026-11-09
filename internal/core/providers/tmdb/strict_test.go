// SPDX-License-Identifier: AGPL-3.0-or-later

package tmdb

import (
	"errors"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/pkg/strictjson"
)

// A reply is read strictly (D456): one result's title said twice is refused, not read as the last.
func TestSearch_AReplyWithAKeySaidTwiceIsRefused(t *testing.T) {
	t.Parallel()
	server, _ := fakeTMDB(t, map[string]string{"/search/movie": `{"page":1,"results":[
		{"id":4801,"title":"The Gold Rush","title":"Something Else","release_date":"1999-03-31"}]}`})
	query := identify.Query{Kind: "movie", Name: "The Gold Rush", Year: 1999}
	_, err := newClient(t, server.URL, "v3key").Search(t.Context(), query)
	if !errors.Is(err, strictjson.ErrRefused) {
		t.Errorf("a reply with a duplicate key: %v; want strictjson.ErrRefused", err)
	}
}
