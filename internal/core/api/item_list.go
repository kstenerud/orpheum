// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/library"
)

// ItemEntryBody is one item in GET /items, the one list (D416 §2, D425 §7): its id, kind, name and
// art, the fields its kind has, and what the list's filter asked for. The document says it as a
// oneOf with a discriminator on kind, a title's entry or a series' (D425 §7), so a generated client
// narrows on kind; it is written as the variant its kind is, without the other kind's fields.
type ItemEntryBody struct {
	Kind string `json:"kind"`
	ItemEntryCommon
	TitleEntryFields
	SeriesEntryFields
}

// ItemEntryCommon is what every kind of entry has.
type ItemEntryCommon struct {
	ID                  int64  `doc:"GET /items/{id}" json:"id"`
	Name                string `json:"name"`
	Year                int64  `json:"year,omitempty"`
	IdentificationState string `json:"identificationState"`
	// IsGuessed is whether its match is a guess no person confirmed (D301 §4).
	IsGuessed    bool   `json:"isGuessed,omitempty"`
	AddedAt      int64  `doc:"When the scan that found it ran, Unix milliseconds; absent in a list by identification" json:"addedAt,omitempty"` //nolint:lll // one struct tag
	PosterSha256 string `doc:"Its poster's hash: GET /bytes/art/{sourceSha256}/portrait-m.webp" json:"posterSha256,omitempty"`                  //nolint:lll // one struct tag
	// NoPosterStillSha256 is, for a film or series with no poster, the 16:9 picture shown in the top of
	// its poster's place (D362).
	NoPosterStillSha256 string `doc:"A film's or series' 16:9 picture when it has no poster, shown above the mark of a missing poster: GET /bytes/art/{sourceSha256}/landscape-s.webp" json:"noPosterStillSha256,omitempty"` //nolint:lll // one struct tag
	// Unavailable says why none of its files can be read now; absent when one can (D241 §3).
	Unavailable *UnavailableBody `json:"unavailable,omitempty"`
	IsWatched   bool             `doc:"Whether the person watched it: a film or other video by playing it to its finish or by hand, a series once they watched every episode it counts (D370)" json:"isWatched,omitempty"` //nolint:lll // one struct tag
	// Identification is where its match stands, in a list by identification alone (D425 §7).
	Identification *IdentificationBody `doc:"Where its match stands; present in a list by identification alone" json:"identification,omitempty"` //nolint:lll // one struct tag
	// Letter is the alphabet rail's letter it is under, in a list by name or by file name.
	Letter string `doc:"The alphabet rail's letter it is under, # or A to Z, as GET /items/letters names them; present in a list by name or by file name" json:"letter,omitempty"` //nolint:lll // one struct tag
	// Credit is the person's credit on it, in a list by person alone (D425 §7).
	Credit *ItemCreditBody `doc:"The person's credit on it; present in a list by person alone" json:"credit,omitempty"`
}

// TitleEntryFields are what only a film's, an episode's, an extra's or an other video's entry has.
type TitleEntryFields struct {
	// LandscapeSha256 is an other video's 16:9 picture: its still, else its backdrop (D349).
	LandscapeSha256 string `doc:"An other video's 16:9 picture: GET /bytes/art/{sourceSha256}/landscape-s.webp" json:"landscapeSha256,omitempty"` //nolint:lll // one struct tag
	// Recorded is when an other video was recorded, and where that came from; absent for the rest.
	Recorded       *RecordedBody `json:"recorded,omitempty"`
	WatchedPercent int64         `doc:"How far, in percent, the person is through a film, where Continue Watching would resume it; absent for a series, and for a film they are not partway through, have hidden since, or whose file's length is not known" json:"watchedPercent,omitempty"` //nolint:lll // one struct tag
}

// SeriesEntryFields are what only a series' entry has.
type SeriesEntryFields struct {
	EpisodeCount         int64 `doc:"How many episodes a series has with a file, in the ordering the person watches it in; absent for a film" json:"episodeCount,omitempty"`               //nolint:lll // one struct tag
	FinishedEpisodeCount int64 `doc:"How many of those episodes the person watched, by playing or by hand (D370); absent when none, and for a film" json:"finishedEpisodeCount,omitempty"` //nolint:lll // one struct tag
}

// TitleEntryBody is a film's, an episode's, an extra's or an other video's entry in a list.
type TitleEntryBody struct {
	Kind string `doc:"What it is: a film, an episode, an extra or an other video (D349, D416 §5)" enum:"film,episode,extra,other" json:"kind"` //nolint:lll // one struct tag
	ItemEntryCommon
	TitleEntryFields
}

// SeriesEntryBody is a series' entry in a list.
type SeriesEntryBody struct {
	Kind string `doc:"A series (D416 §5)" enum:"series" json:"kind"`
	ItemEntryCommon
	SeriesEntryFields
}

// MarshalJSON writes the variant its kind is.
func (body ItemEntryBody) MarshalJSON() ([]byte, error) {
	var variant any = TitleEntryBody{
		Kind: body.Kind, ItemEntryCommon: body.ItemEntryCommon, TitleEntryFields: body.TitleEntryFields,
	}
	if body.Kind == itemKindSeries {
		variant = SeriesEntryBody{
			Kind: body.Kind, ItemEntryCommon: body.ItemEntryCommon, SeriesEntryFields: body.SeriesEntryFields,
		}
	}
	encoded, err := json.Marshal(variant)
	if err != nil {
		return nil, fmt.Errorf("write a list's entry: %w", err)
	}
	return encoded, nil
}

// Schema is the union on kind, which huma cannot derive from a Go struct, as ItemDetailBody's.
func (ItemEntryBody) Schema(registry huma.Registry) *huma.Schema {
	return unionOnKind(registry.Schema(reflect.TypeFor[TitleEntryBody](), true, "TitleEntryBody"),
		registry.Schema(reflect.TypeFor[SeriesEntryBody](), true, "SeriesEntryBody"))
}

// IdentificationBody is where an item's match stands, as Needs a match shows it.
type IdentificationBody struct {
	FollowedSource     string            `doc:"Where its own values come from: nfo, tmdb, or none" json:"followedSource,omitempty"` //nolint:lll // one struct tag
	FollowedSourceID   string            `doc:"Its id at its provider" json:"followedSourceId,omitempty"`
	FetchFailure       *FetchFailureBody `json:"fetchFailure,omitempty"`
	RefreshRequestedAt int64             `doc:"When a person asked for it to be refreshed, while no fetch has answered yet" json:"refreshRequestedAt,omitempty"` //nolint:lll // one struct tag
}

// ItemCreditBody is a credited person's credit on an item: their jobs, never their roles (D186 §2,
// D416 §4), and their part in the cast.
type ItemCreditBody struct {
	Jobs      []string `doc:"Their jobs on it, cast first" json:"jobs" enum:"cast,director,writer"`
	Character string   `doc:"Their part, in the cast; absent for crew and where the source names none" json:"character,omitempty"` //nolint:lll // one struct tag
}

// itemEntryBody is a list's entry as the API says it.
func itemEntryBody(entry library.Entry, nodeName func(nodeID int64) string) ItemEntryBody {
	return ItemEntryBody{
		Kind: saidKind(entry.Kind),
		ItemEntryCommon: ItemEntryCommon{
			ID: entry.ID, Name: entry.Name, Year: entry.Year,
			IdentificationState: entry.IdentificationState, IsGuessed: entry.IsGuessed,
			AddedAt: entry.AddedAt, PosterSha256: entry.PosterSha256,
			NoPosterStillSha256: entry.NoPosterStillSha256,
			Unavailable:         unavailableBody(entry.Unreadable, nodeName), IsWatched: entry.IsWatched,
			Letter: entry.Letter,
		},
		TitleEntryFields: TitleEntryFields{
			LandscapeSha256: entry.LandscapeSha256, Recorded: recordedBody(entry.Other),
			WatchedPercent: entry.WatchedPercent,
		},
		SeriesEntryFields: SeriesEntryFields{
			EpisodeCount: entry.EpisodeCount, FinishedEpisodeCount: entry.FinishedEpisodeCount,
		},
	}
}

// ListItemsInput asks for one page of the one list (D416 §2, §6): films, series or other videos, of
// every library the person reaches or of one (D349), or the items in one state of identification.
type ListItemsInput struct {
	AuthorizationHeader
	Kinds          []string `default:"film,series,other" doc:"The kinds it lists, each named, comma-separated: film, series or other. Absent: films, series and other videos together, every library's. A kind a later server adds is listed only where a client names it (D432 §1)" enum:"film,series,other" minItems:"1" query:"kind"`                                                                                                                                                                                                                             //nolint:lll // one struct tag
	Sort           string   `default:"name" doc:"name: in the library's locale. added: newest first, by the scan that found it, most recent first, then by its file's own modified time, newest first; a film is as new as the scan that first found it, a series as its newest episode. recorded: newest recorded first, and fileName: by the files' names, other videos only. released: newest first by a film's release date or a series' first air date, those with none last, films and series only" enum:"name,added,recorded,fileName,released" query:"sort"` //nolint:lll // one struct tag
	Order          string   `doc:"The way the sort runs (D366): ascending, A to Z or oldest first; descending, Z to A or newest first; absent for the sort's own, A to Z by a name and newest first by a date. What has no date is last either way" enum:"ascending,descending" query:"order"`                                                                                                                                                                                                                                                                       //nolint:lll // one struct tag
	Library        int64    `doc:"One library's alone; absent for every library the person reaches, less, for films and series, those they left out (leftOutLibraryIds)" minimum:"0" query:"library"`                                                                                                                                                                                                                                                                                                                                                                //nolint:lll // one struct tag
	IsHome         bool     `doc:"The list as a row of Home shows it, with no library: of every library the person reaches less those they left out of Home (leftOutOfHomeLibraryIds, D366), in place of those left out of Films and Series" query:"home"`                                                                                                                                                                                                                                                                                                           //nolint:lll // one struct tag
	Identification string   `doc:"Only the films, extras and series in this state of identification, as Needs a match lists them, by name, each with its identification; with no other filter than library" enum:"identified,ambiguous,unidentified,confirmed" query:"identification"`                                                                                                                                                                                                                                                                               //nolint:lll // one struct tag
	From           string   `doc:"A name, by name or file name only and with no cursor: A to Z, the page starts at the first item sorted at or after it, as a letter of the alphabet does; Z to A, at the last item sorted before it, so the letter after a letter starts that letter's items" maxLength:"100" query:"from"`                                                                                                                                                                                                                                         //nolint:lll // one struct tag
	// A list of films, series or other videos may be read whole in one answer, its limit left out
	// (D475); a list by identification is read a page at a time.
	PageOrWholeInput
	Query string `doc:"Only the items whose name matches it, in any script (D48): a film, other video or series by its own name, never by an episode's or an extra's; every other criterion applies as without it. A query that matches nothing, or with nothing an index can match (only spaces, or only punctuation in a longer query), lists nothing. The items come from the first 10000 names it matches, the best first" maxLength:"200" query:"query"` //nolint:lll // one struct tag
	ListFilters
}

// ListItemsOutput is one page of the one list.
type ListItemsOutput struct {
	Body struct {
		Items      []ItemEntryBody `json:"items"`
		NextCursor string          `doc:"The next page's cursor; absent on the last" json:"nextCursor,omitempty"`
		Total      int64           `doc:"How many items the whole list holds, so a client can size it before its other pages come and see where its end now is; on every page but one asked with from, and absent for an empty list and a list by identification" json:"total,omitempty"` //nolint:lll // one struct tag
	}
}

func (server *Server) registerItemList() {
	tags := []string{tagBrowse}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listItems", Method: http.MethodGet, Path: "/items",
		Summary: "The one list: films, series or other videos by name in the library's locale or newest first, " +
			"filtered by library, genre, person, decade, unwatched or identification", Tags: tags,
		Extensions: callableBy(capabilitySignedIn),
	}, server.listItems)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listItemLetters", Method: http.MethodGet, Path: "/items/letters",
		Summary: "The letters of the alphabet a list by name has items under", Tags: tags,
		Extensions: callableBy(capabilitySignedIn),
	}, server.listItemLetters)
}

func (server *Server) listItems(ctx context.Context, input *ListItemsInput) (*ListItemsOutput, error) {
	input.Kinds = keptKinds(input.Kinds)
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if input.Identification != "" {
		return server.listItemsByIdentification(ctx, dependencies, principal, input)
	}
	isByName := input.Sort == library.SortName || input.Sort == library.SortFileName
	if input.From != "" && (!isByName || input.Cursor != "") {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "query.from",
			"from starts a list by name, in place of its first page: it takes no cursor")
	}
	limit := input.Limit
	if limit == 0 {
		limit = library.WholeList
	}
	if input.IsHome && input.Library != 0 {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "query.home",
			"a row of Home is of every library the person keeps on it, not of one library")
	}
	reach, err := server.listReach(ctx, dependencies, principal,
		listOf{kinds: input.Kinds, libraryID: input.Library, isHome: input.IsHome})
	if err != nil {
		return nil, err
	}
	page, err := dependencies.Browser.List(ctx, input.withFilters(library.ListQuery{
		Kinds: input.Kinds, Sort: input.Sort, Order: input.Order, Cursor: input.Cursor, From: input.From,
		Limit: limit, Reach: reach, UserID: principal.UserID, Query: input.Query,
	}))
	if err != nil {
		return nil, server.libraryError(ctx, "list items", err)
	}
	output := &ListItemsOutput{}
	output.Body.Items = make([]ItemEntryBody, 0, len(page.Entries))
	nodeName := server.nodeNamer(ctx, dependencies)
	for _, entry := range page.Entries {
		output.Body.Items = append(output.Body.Items, itemEntryBody(entry, nodeName))
	}
	output.Body.NextCursor = page.NextCursor
	output.Body.Total = page.Total
	if input.Person != 0 {
		if err := server.addCredits(ctx, dependencies, input.Person, output.Body.Items); err != nil {
			return nil, err
		}
	}
	return output, nil
}

// addCredits puts the person's credit on each of the items a list by person found (D425 §7).
func (server *Server) addCredits(
	ctx context.Context, dependencies *Dependencies, personID int64, items []ItemEntryBody,
) error {
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	credits, err := dependencies.Browser.CreditsOn(ctx, personID, ids)
	if err != nil {
		return server.libraryError(ctx, "list a person's credits", err)
	}
	for index := range items {
		roles := credits[items[index].ID]
		if len(roles) == 0 {
			continue
		}
		credit := &ItemCreditBody{Jobs: make([]string, 0, len(roles))}
		for _, role := range roles {
			credit.Jobs = append(credit.Jobs, role.Role)
			if credit.Character == "" {
				credit.Character = role.Character
			}
		}
		items[index].Credit = credit
	}
	return nil
}

// listItemsByIdentification is the list Needs a match shows: the films, extras and series in one
// state of identification, by name, each with where its match stands, paged by cursor (D425 §7).
func (server *Server) listItemsByIdentification(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, input *ListItemsInput,
) (*ListItemsOutput, error) {
	if !isEveryListKind(input.Kinds) || input.Sort != library.SortName || input.Order != "" || input.IsHome ||
		input.From != "" || input.Query != "" || input.ListFilters != (ListFilters{}) {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "query.identification",
			"a list by identification is of every kind, by name, and takes no other filter than library")
	}
	if input.Limit == 0 {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "query.identification",
			"a list by identification is read a page at a time: it takes a limit")
	}
	reach, err := server.listReach(ctx, dependencies, principal,
		listOf{kinds: library.ListKinds(), libraryID: input.Library})
	if err != nil {
		return nil, err
	}
	page, err := dependencies.Chooser.Items(ctx, reach, input.Identification, input.Cursor, input.Limit)
	if errors.Is(err, library.ErrBadCursor) {
		return nil, server.libraryError(ctx, "list items by identification", err)
	}
	if err != nil {
		return nil, server.identifyError(ctx, "list items by identification", err)
	}
	output := &ListItemsOutput{}
	output.Body.Items = make([]ItemEntryBody, 0, len(page.Items))
	for _, item := range page.Items {
		output.Body.Items = append(output.Body.Items, identificationEntry(item))
	}
	output.Body.NextCursor = page.NextCursor
	return output, nil
}

// identificationEntry is an item a list by identification found, as the API says it.
func identificationEntry(item identify.ItemSummary) ItemEntryBody {
	identification := &IdentificationBody{
		FollowedSource: item.FollowedSource, FollowedSourceID: item.FollowedSourceID,
		RefreshRequestedAt: unixMillisOrZero(item.RefreshRequestedAt),
	}
	if failure := item.FetchFailure; failure != nil {
		identification.FetchFailure = &FetchFailureBody{
			Failure: failure.Failure, FailedAt: failure.FailedAt.UnixMilli(),
		}
	}
	return ItemEntryBody{
		Kind: saidKind(item.Kind),
		ItemEntryCommon: ItemEntryCommon{
			ID: item.ID, Name: item.Name, Year: item.Year,
			IdentificationState: item.IdentificationState, Identification: identification,
		},
	}
}
