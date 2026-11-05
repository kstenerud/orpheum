// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

// ListFilters are a list's filters, which GET /titles and its letters take alike, combined with each
// other and with its query (search-by-more-2026-11-04.md §4).
type ListFilters struct {
	Genre     int64 `doc:"Only the titles in this genre, by its id from GET /genres" minimum:"0" query:"genre"`
	Person    int64 `doc:"Only the titles crediting this person, by their id from a title's credits or a search" minimum:"0" query:"person"`                                                           //nolint:lll // one struct tag
	Decade    int64 `doc:"Only the titles first out in this decade, by its first year (1980 for the 1980s); a series by its first air date" maximum:"9990" minimum:"0" multipleOf:"10" query:"decade"` //nolint:lll // one struct tag
	Unwatched bool  `doc:"Only the titles the person has not watched: a series while one of its episodes with a file is not" query:"unwatched"`                                                        //nolint:lll // one struct tag
}

// withFilters is query with filters.
func (filters ListFilters) withFilters(query library.ListQuery) library.ListQuery {
	query.GenreID, query.PersonID, query.Decade = filters.Genre, filters.Person, filters.Decade
	query.IsUnwatched = filters.Unwatched
	return query
}

// GenreBody is a genre.
type GenreBody struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// TitleCount is how many titles the person reaches are in it, where a listing of genres counts.
	TitleCount int64 `doc:"How many films, series or other videos the person reaches are in it; absent on a title's page" json:"titleCount,omitempty"` //nolint:lll // one struct tag
}

// CreditBody is a person a title credits.
type CreditBody struct {
	PersonID   int64  `doc:"GET /people/{id}; GET /items?person= lists their items" json:"personId"`
	Name       string `json:"name"`
	Character  string `doc:"A cast member's part; absent for crew and where the source names none" json:"character,omitempty"`                                         //nolint:lll // one struct tag
	PictureRef string `doc:"Their picture as their source names it, a TMDB profile path or an NFO's thumb; not fetched by the server yet" json:"pictureRef,omitempty"` //nolint:lll // one struct tag
}

// CreditsBody is a title's people, in billing order within each role.
type CreditsBody struct {
	Cast      []CreditBody `json:"cast"`
	Directors []CreditBody `json:"directors"`
	Writers   []CreditBody `json:"writers"`
}

// peopleBodies are a title's genres and credits as the API says them: nil for none, which a page
// leaves out.
func peopleBodies(people library.TitlePeople) ([]GenreBody, *CreditsBody) {
	var genres []GenreBody
	for _, genre := range people.Genres {
		genres = append(genres, GenreBody{ID: genre.ID, Name: genre.Name})
	}
	credits := func(credited []library.CreditedPerson) []CreditBody {
		bodies := make([]CreditBody, 0, len(credited))
		for _, person := range credited {
			bodies = append(bodies, CreditBody{
				PersonID: person.PersonID, Name: person.Name, Character: person.Character,
				PictureRef: person.PictureRef,
			})
		}
		return bodies
	}
	if len(people.Cast)+len(people.Directors)+len(people.Writers) == 0 {
		return genres, nil
	}
	return genres, &CreditsBody{
		Cast: credits(people.Cast), Directors: credits(people.Directors), Writers: credits(people.Writers),
	}
}

// PersonBody is a credited person, whom titles credit (a user is never a person, D416 §1, §4).
type PersonBody struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// PictureRef is their picture; GET /items?person= lists the items crediting them (D425 §7).
	PictureRef string `doc:"Their picture as their source names it, a TMDB profile path or an NFO's thumb; not fetched by the server yet. GET /items?person= lists the items crediting them, each with their credit" json:"pictureRef,omitempty"` //nolint:lll // one struct tag
}

// PersonOutput is a credited person's page.
type PersonOutput struct {
	Body PersonBody
}

// GenresInput asks for the genres of a kind of title.
type GenresInput struct {
	AuthorizationHeader
	Kind    string `doc:"movie, series or other: the genres of that kind's titles; any: of every kind" enum:"movie,series,other,any" query:"kind" required:"true"` //nolint:lll // one struct tag
	Library int64  `doc:"As GET /items takes it" minimum:"0" query:"library"`
}

// GenresOutput is the genres with a title the person reaches, by name.
type GenresOutput struct {
	Body struct {
		Genres []GenreBody `json:"genres"`
	}
}

// GroupedSearchInput is a grouped search.
type GroupedSearchInput struct {
	AuthorizationHeader
	Query  string `doc:"What was typed: from its first letter, in any script (D48)" maxLength:"200" query:"query" required:"true"` //nolint:lll // one struct tag
	Group  string `doc:"One group's next page alone; absent for the first page of each" enum:"titles,people,genres" query:"group"` //nolint:lll // one struct tag
	Cursor string `doc:"With group, that group's nextCursor from the page before" query:"cursor"`
	Limit  int64  `doc:"At most this many in each group" maximum:"100" minimum:"1" query:"limit" required:"true"`
}

// GroupHitBody is a title, person or genre a grouped search found.
type GroupHitBody struct {
	ID         int64  `doc:"A title's: GET /items/{id}, of any kind; a person's: GET /people/{id}; a genre's: GET /items?genre=" json:"id"` //nolint:lll // one struct tag
	Kind       string `doc:"A title's kind; absent for a person or a genre" enum:"movie,series,other" json:"kind,omitempty"`                //nolint:lll // one struct tag
	Name       string `json:"name"`
	Year       int64  `json:"year,omitempty"`
	PictureRef string `doc:"A person's picture as their source names it; not fetched by the server yet" json:"pictureRef,omitempty"` //nolint:lll // one struct tag
	TitleCount int64  `doc:"A genre's titles the person reaches" json:"titleCount,omitempty"`
}

// GroupBody is one group of a grouped search, the best first.
type GroupBody struct {
	Hits       []GroupHitBody `json:"hits"`
	NextCursor string         `doc:"The group's next page, with group; absent on its last" json:"nextCursor,omitempty"`
}

// GroupedSearchOutput is a grouped search's page: titles, people and genres.
type GroupedSearchOutput struct {
	Body struct {
		Titles GroupBody `json:"titles"`
		People GroupBody `json:"people"`
		Genres GroupBody `json:"genres"`
	}
}

func (server *Server) registerPeople() {
	tags := []string{tagBrowse}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getPerson", Method: http.MethodGet, Path: "/people/{id}",
		Summary: "A person: their name and their picture; GET /items?person= lists what credits them", Tags: tags,
	}, server.getPerson)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getGenres", Method: http.MethodGet, Path: "/genres",
		Summary: "The genres with a title the person reaches, each with how many", Tags: tags,
	}, server.getGenres)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "searchGrouped", Method: http.MethodGet, Path: "/search/grouped",
		Summary: "Titles, people and genres by what was typed, from its first letter, each group the best first",
		Tags:    tags,
	}, server.searchGrouped)
}

func (server *Server) getPerson(ctx context.Context, input *ItemInput) (*PersonOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	person, err := dependencies.Browser.Person(ctx, principal.Reach, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "read a person", err)
	}
	output := &PersonOutput{}
	body := &output.Body
	body.ID, body.Name, body.PictureRef = person.ID, person.Name, person.PictureRef
	return output, nil
}

func (server *Server) getGenres(ctx context.Context, input *GenresInput) (*GenresOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	reach, err := server.listReach(ctx, dependencies, principal, listOf{kind: input.Kind, libraryID: input.Library})
	if err != nil {
		return nil, err
	}
	genres, err := dependencies.Browser.Genres(ctx, reach, input.Kind)
	if err != nil {
		return nil, server.libraryError(ctx, "list the genres", err)
	}
	output := &GenresOutput{}
	output.Body.Genres = make([]GenreBody, 0, len(genres))
	for _, genre := range genres {
		output.Body.Genres = append(output.Body.Genres, GenreBody{
			ID: genre.ID, Name: genre.Name, TitleCount: genre.TitleCount,
		})
	}
	return output, nil
}

func (server *Server) searchGrouped(ctx context.Context, input *GroupedSearchInput) (*GroupedSearchOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	var offset int64
	if input.Cursor != "" {
		offset, err = strconv.ParseInt(input.Cursor, 10, 64)
		if input.Group == "" || err != nil || offset < 0 {
			return nil, invalidRequestAt(http.StatusUnprocessableEntity, "query.cursor",
				"a cursor is a group's nextCursor, with its group")
		}
	}
	hits, err := dependencies.Browser.SearchGrouped(ctx, library.GroupedQuery{
		Reach: principal.Reach, Query: input.Query, Group: input.Group, Offset: offset, Limit: input.Limit,
	})
	if err != nil {
		return nil, server.libraryError(ctx, "search", err)
	}
	next := func(hasMore bool) string {
		if !hasMore {
			return ""
		}
		return strconv.FormatInt(offset+input.Limit, 10)
	}
	output := &GroupedSearchOutput{}
	body := &output.Body
	body.Titles.Hits = make([]GroupHitBody, 0, len(hits.Titles))
	for _, hit := range hits.Titles {
		body.Titles.Hits = append(body.Titles.Hits, GroupHitBody{
			ID: hit.ItemID, Kind: ptr.ValueOrZero(hit.TitleKind), Name: hit.Name, Year: ptr.ValueOrZero(hit.Year),
		})
		if hit.TitleKind == nil {
			body.Titles.Hits[len(body.Titles.Hits)-1].Kind = library.ListSeries
		}
	}
	body.People.Hits = make([]GroupHitBody, 0, len(hits.People))
	for _, hit := range hits.People {
		body.People.Hits = append(body.People.Hits, GroupHitBody{
			ID: hit.PersonID, Name: hit.Name, PictureRef: ptr.ValueOrZero(hit.PictureRef),
		})
	}
	body.Genres.Hits = make([]GroupHitBody, 0, len(hits.Genres))
	for _, genre := range hits.Genres {
		body.Genres.Hits = append(body.Genres.Hits, GroupHitBody{
			ID: genre.ID, Name: genre.Name, TitleCount: genre.TitleCount,
		})
	}
	body.Titles.NextCursor = next(hits.HasMoreTitles)
	body.People.NextCursor = next(hits.HasMorePeople)
	body.Genres.NextCursor = next(hits.HasMoreGenres)
	return output, nil
}
