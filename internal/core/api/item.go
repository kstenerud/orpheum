// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

// itemKindSeries is a series' kind, among the item kinds the API says (D416 §5).
const itemKindSeries = "series"

// ItemDetailBody is one item's detail, of whichever kind: a title's or a series' (D416 §2). The
// document says it as a oneOf with a discriminator on kind, so a generated client narrows on kind
// (D425 §7); exactly one of its fields is set.
type ItemDetailBody struct {
	Title  *ItemBody
	Series *SeriesBody
}

// MarshalJSON writes the one variant that is set.
func (body ItemDetailBody) MarshalJSON() ([]byte, error) {
	var variant any = body.Title
	if body.Series != nil {
		variant = body.Series
	}
	encoded, err := json.Marshal(variant)
	if err != nil {
		return nil, fmt.Errorf("write an item's detail: %w", err)
	}
	return encoded, nil
}

// Schema is the union on kind, which huma cannot derive from a Go struct.
func (ItemDetailBody) Schema(registry huma.Registry) *huma.Schema {
	return unionOnKind(registry.Schema(reflect.TypeFor[ItemBody](), true, "ItemBody"),
		registry.Schema(reflect.TypeFor[SeriesBody](), true, "SeriesBody"))
}

// unionOnKind is an item's body of whichever kind, a title's or a series', as the document says it:
// a oneOf with a discriminator on kind, so a generated client narrows on kind (D425 §7).
func unionOnKind(title, series *huma.Schema) *huma.Schema {
	mapping := map[string]string{itemKindSeries: series.Ref}
	for _, kind := range []string{
		kindFilm, library.TitleKindEpisode, library.TitleKindExtra, library.TitleKindOther,
	} {
		mapping[kind] = title.Ref
	}
	return &huma.Schema{
		OneOf:         []*huma.Schema{title, series},
		Discriminator: &huma.Discriminator{PropertyName: "kind", Mapping: mapping},
	}
}

// ItemDetailOutput is an item's detail page.
type ItemDetailOutput struct {
	Body ItemDetailBody
}

func (server *Server) registerItem() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getItem", Method: http.MethodGet, Path: "/items/{id}",
		Summary: "An item's detail, by its kind: a title's versions, files, streams, extras and, for an " +
			"episode, its place and what plays after it and before it; a series' seasons, each with how " +
			"many episodes it has, and the shown season's first page of episodes",
		Description: "A credited person is not an item here: their id answers 404, and their page is " +
			"GET /people/{id} (D425 §7).",
		Tags:       []string{tagBrowse},
		Extensions: callableBy(capabilitySignedIn),
	}, server.getItem)
}

// ItemDetailInput asks for an item's detail; a series' at the season its page shows.
type ItemDetailInput struct {
	AuthorizationHeader
	ID     int64 `path:"id"`
	Season int64 `default:"-1" doc:"For a series, the season its page shows, 0 for the specials, whose first page of episodes the answer carries; -1, the default, for the season of the person's next episode, else the first" minimum:"-1" query:"season"` //nolint:lll // one struct tag
}

func (server *Server) getItem(ctx context.Context, input *ItemDetailInput) (*ItemDetailOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	isCollection, err := dependencies.Browser.IsCollection(ctx, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "read an item's kind", err)
	}
	output := &ItemDetailOutput{}
	if isCollection {
		output.Body.Series, err = server.seriesBody(ctx, dependencies, principal, input.ID, input.Season)
	} else {
		output.Body.Title, err = server.titleBody(ctx, dependencies, principal, input.ID)
	}
	if err != nil {
		return nil, err
	}
	return output, nil
}

// ItemBody is a title's detail: a film's, an episode's, an extra's or an other video's (D416 §2).
type ItemBody struct {
	ID                  int64             `json:"id"`
	Kind                string            `enum:"film,episode,extra,other" json:"kind"`
	Name                string            `json:"name"`
	Overview            string            `json:"overview,omitempty"`
	Tagline             string            `json:"tagline,omitempty"`
	Year                int64             `json:"year,omitempty"`
	RuntimeMs           int64             `json:"runtimeMs,omitempty"`
	IdentificationState string            `json:"identificationState"`
	IsGuessed           bool              `doc:"Whether its match is a guess no person confirmed (D301)" json:"isGuessed,omitempty"` //nolint:lll // one struct tag
	FollowedSource      string            `json:"followedSource,omitempty"`
	Versions            []VersionBody     `json:"versions"`
	Extras              []ExtraBody       `json:"extras"`
	Episode             *EpisodePlaceBody `json:"episode,omitempty"`
	Art                 ArtBody           `json:"art"`
	FetchFailure        *FetchFailureBody `json:"fetchFailure,omitempty"`
	RefreshRequestedAt  int64             `doc:"When a person asked for it to be refreshed, while no fetch has answered yet" json:"refreshRequestedAt,omitempty"`                                               //nolint:lll // one struct tag
	Unavailable         *UnavailableBody  `doc:"Why none of its files can be read now; absent when one can" json:"unavailable,omitempty"`                                                                       //nolint:lll // one struct tag
	IsWatched           bool              `doc:"Whether the person watched it, by playing it to its finish or by hand (D370)" json:"isWatched,omitempty"`                                                       //nolint:lll // one struct tag
	Libraries           []LibraryNameBody `doc:"The libraries its person reaches that list it, by name: its page goes up to the one the person came through, else the first (D367)" json:"libraries,omitempty"` //nolint:lll // one struct tag
	Genres              []GenreBody       `doc:"Its genres in its source's order; an episode's are its series'; absent when it has none" json:"genres,omitempty"`                                               //nolint:lll // one struct tag
	Credits             *CreditsBody      `doc:"Its cast, directors and writers in billing order; an episode's are its series'; absent when it credits no one" json:"credits,omitempty"`                        //nolint:lll // one struct tag
	FilmOfExtra         *FilmOfExtraBody  `doc:"The film an extra belongs to, which its page goes up to (D367); absent for anything else" json:"filmOfExtra,omitempty"`                                         //nolint:lll // one struct tag
	UpNext              *UpNextBody       `doc:"For an episode, what plays after its first file, as GET /files/{id}/up-next says it (D477 §2); absent where nothing does" json:"upNext,omitempty"`              //nolint:lll // one struct tag
	Previous            *NextUpBody       `doc:"For an episode, what plays before its first file, as GET /files/{id}/up-next says it (D477 §2); absent where nothing does" json:"previous,omitempty"`           //nolint:lll // one struct tag
}

// SeriesBody is a series with its seasons, the groups of the ordering its person watches it in, in
// its order (aired's with the specials last); each season's episodes are
// GET /items/{id}/groups/{group} (D306 §9).
type SeriesBody struct {
	ID                  int64             `json:"id"`
	Kind                string            `enum:"series" json:"kind"`
	Name                string            `json:"name"`
	Overview            string            `json:"overview,omitempty"`
	Year                int64             `json:"year,omitempty"`
	IdentificationState string            `json:"identificationState,omitempty"`
	IsGuessed           bool              `doc:"Whether its match is a guess no person confirmed (D301)" json:"isGuessed,omitempty"`                                     //nolint:lll // one struct tag
	Ordering            string            `doc:"The ordering its person watches it in, which its seasons are the groups of; absent for aired" json:"ordering,omitempty"` //nolint:lll // one struct tag
	Seasons             []SeasonBody      `json:"seasons"`
	Orderings           []OrderingBody    `doc:"Every ordering it has, aired first; its page offers a choice when there is more than one" json:"orderings,omitempty"`            //nolint:lll // one struct tag
	FilesOrdering       string            `doc:"The ordering its files' episode numbers are in, which an admin sets (D210 §1); absent for aired" json:"filesOrdering,omitempty"` //nolint:lll // one struct tag
	NotInOrderingCount  int64             `doc:"How many of its episodes the ordering in use leaves out: GET /items/{id}/not-in-order" json:"notInOrderingCount,omitempty"`      //nolint:lll // one struct tag
	Art                 ArtBody           `json:"art"`
	FetchFailure        *FetchFailureBody `json:"fetchFailure,omitempty"`
	RefreshRequestedAt  int64             `doc:"When a person asked for it to be refreshed, while no fetch has answered yet" json:"refreshRequestedAt,omitempty"`                                                                //nolint:lll // one struct tag
	Unavailable         *UnavailableBody  `doc:"Why none of its episodes' files can be read now; absent when one can" json:"unavailable,omitempty"`                                                                              //nolint:lll // one struct tag
	NextUp              *NextUpBody       `doc:"The person's next episode, which the page offers to play; absent when every one is finished or none has a file" json:"nextUp,omitempty"`                                         //nolint:lll // one struct tag
	IsWatched           bool              `doc:"Whether the person watched every one of its episodes with a file, in the ordering they watch it in (D370)" json:"isWatched,omitempty"`                                           //nolint:lll // one struct tag
	Genres              []GenreBody       `doc:"Its genres in its source's order; absent when it has none" json:"genres,omitempty"`                                                                                              //nolint:lll // one struct tag
	Credits             *CreditsBody      `doc:"Its cast, directors and writers in billing order; absent when it credits no one" json:"credits,omitempty"`                                                                       //nolint:lll // one struct tag
	Libraries           []LibraryNameBody `doc:"The libraries its person reaches that list one of its episodes, by name: its page goes up to the one the person came through, else the first (D367)" json:"libraries,omitempty"` //nolint:lll // one struct tag
	ShownSeason         *ShownSeasonBody  `doc:"The season its page shows, the one asked for where it has it, and its first page of episodes (D477 §1); absent where it has no season" json:"shownSeason,omitempty"`             //nolint:lll // one struct tag
}

// ShownSeasonBody is the season a series' page shows and its first page of episodes, as
// GET /items/{id}/groups/{group} answers it, whose nextCursor the page reads the rest by (D477 §1).
type ShownSeasonBody struct {
	SeasonNumber int64         `json:"seasonNumber"`
	Episodes     []EpisodeBody `json:"episodes"`
	NextCursor   string        `doc:"The next page's cursor, for GET /items/{id}/groups/{group}; absent on the last" json:"nextCursor,omitempty"` //nolint:lll // one struct tag
}

// shownSeasonPageSize is how many episodes a series' answer carries of its shown season: the web
// client's page of them (series.tsx's episodesPerPage).
const shownSeasonPageSize = 100

// titleBody is a title's detail as principal sees it.
func (server *Server) titleBody(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, titleID int64,
) (*ItemBody, error) {
	detail, err := dependencies.Browser.Title(ctx, principal.Reach, principal.UserID, titleID)
	if err != nil {
		return nil, server.libraryError(ctx, "read a title", err)
	}
	body := &ItemBody{}
	body.ID, body.Kind, body.Name = detail.ID, saidKind(detail.Kind), ptr.ValueOrZero(detail.Name)
	body.Genres, body.Credits = peopleBodies(detail.People)
	body.IdentificationState, body.IsGuessed = ptr.ValueOrZero(detail.IdentificationState), detail.IsGuessed
	body.Overview = ptr.ValueOrZero(detail.Overview)
	body.Tagline = ptr.ValueOrZero(detail.Tagline)
	body.FollowedSource = ptr.ValueOrZero(detail.FollowedSource)
	body.Year = ptr.ValueOrZero(detail.Year)
	body.RuntimeMs = ptr.ValueOrZero(detail.RuntimeMs)
	body.FetchFailure = fetchFailureBody(detail.FetchFailure, detail.FetchFailedAt)
	body.RefreshRequestedAt = ptr.ValueOrZero(detail.RefreshRequestedAt)
	body.Versions = make([]VersionBody, 0, len(detail.Versions))
	for _, version := range detail.Versions {
		versionBody := VersionBody{ID: version.ID, Files: []FileDetailBody{}}
		for _, file := range version.Files {
			fileBody := FileDetailBody{
				ID: file.ID, SizeBytes: file.SizeBytes, Streams: []StreamBody{}, ResumePositionMs: file.ResumeAtMs,
			}
			// Paths are shown where the admin administers, never from away (the Plex-chain review's T2).
			if administersHere(ctx, principal) {
				fileBody.Path, fileBody.OtherPaths = file.Path, file.Paths[1:]
			}
			for _, stream := range file.Streams {
				fileBody.Streams = append(fileBody.Streams, StreamBody{
					ID: stream.ID, Kind: stream.Kind, Codec: stream.Codec, Language: stream.Language,
					ContainerTitle: stream.ContainerTitle,
					ChannelCount:   stream.ChannelCount,
				})
			}
			versionBody.Files = append(versionBody.Files, fileBody)
		}
		body.Versions = append(body.Versions, versionBody)
	}
	body.Extras = make([]ExtraBody, 0, len(detail.Extras))
	for _, extra := range detail.Extras {
		body.Extras = append(body.Extras, ExtraBody{
			ID: extra.ExtraWork, ExtraType: saidValue(extra.ExtraType), Name: ptr.ValueOrZero(extra.Name),
		})
	}
	body.Art = artBody(detail.Art)
	body.Episode = episodePlaceBody(detail.Episode)
	whereabouts, err := server.whereabouts(ctx, dependencies, principal.Reach, detail.ID, detail.Kind)
	if err != nil {
		return nil, err
	}
	body.Libraries = libraryNameBodies(whereabouts.Libraries)
	body.FilmOfExtra = filmOfExtraBody(whereabouts.FilmOfExtra)
	body.Unavailable = unavailableBody(detail.Unreadable, server.nodeNamer(ctx, dependencies))
	body.IsWatched = detail.IsWatched
	if body.UpNext, body.Previous, err = server.titleNeighbours(ctx, dependencies, principal, detail); err != nil {
		return nil, err
	}
	if detail.Unreadable != nil && detail.Unreadable.Reason == library.StillReading {
		// Its page open, its files are read before what else waits, so Play works soon (D304 §4).
		if readErr := dependencies.Playback.ReadWhenViewed(ctx, detail.ID); readErr != nil {
			server.logger.WarnContext(ctx, "could not ask for a title's read", "titleId", detail.ID, "err", readErr)
		}
	}
	return body, nil
}

// titleNeighbours are what plays after an episode and before it, as its page's Play plays it: from
// the first version's first file, the one GET /files/{id}/up-next is asked of (D477 §2); none for
// any other title, and none while none of its files can be read, whose page plays nothing.
func (server *Server) titleNeighbours(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, detail library.TitleDetail,
) (*UpNextBody, *NextUpBody, error) {
	if detail.Kind != library.TitleKindEpisode || detail.Unreadable != nil || len(detail.Versions) == 0 ||
		len(detail.Versions[0].Files) == 0 {
		return nil, nil, nil
	}
	neighbours, err := dependencies.Browser.NeighboursOf(ctx, principal.Reach, principal.UserID,
		detail.Versions[0].Files[0].ID)
	if err != nil {
		return nil, nil, server.libraryError(ctx, "read what plays after an episode and before it", err)
	}
	upNext, previous := server.neighbourBodies(ctx, dependencies, neighbours)
	return upNext, previous, nil
}

// episodePlaceBody is an episode's place as the API shows it, or nil for a title in no series.
func episodePlaceBody(place *library.EpisodePlace) *EpisodePlaceBody {
	if place == nil {
		return nil
	}
	body := &EpisodePlaceBody{
		SeriesID: place.SequenceID, SeriesName: ptr.ValueOrZero(place.SeriesName),
		SeasonNumber: ptr.ValueOrZero(place.GroupNumber), EpisodeNumber: ptr.ValueOrZero(place.NumberInGroup),
	}
	if place.Ordering != library.OrderingAired {
		body.Ordering = place.Ordering
	}
	if in := place.InOrdering; in != nil {
		body.InOrdering = &OrderingPlaceBody{
			GroupNumber: in.Group, NumberInGroup: in.NumberInGroup, GroupName: in.GroupName,
			IsOnlyGroup: in.IsOnlyGroup,
		}
	}
	return body
}

// seriesBody is a series as principal sees it.
func (server *Server) seriesBody(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, seriesID, season int64,
) (*SeriesBody, error) {
	series, err := dependencies.Browser.Series(ctx, principal.Reach, principal.UserID, seriesID)
	if err != nil {
		return nil, server.libraryError(ctx, "read a series", err)
	}
	body := &SeriesBody{}
	body.ID, body.Kind = series.ID, itemKindSeries
	body.Genres, body.Credits = peopleBodies(series.People)
	if series.Ordering != library.OrderingAired {
		body.Ordering = series.Ordering
	}
	body.Name, body.Overview = ptr.ValueOrZero(series.Name), ptr.ValueOrZero(series.Overview)
	body.Year = ptr.ValueOrZero(series.Year)
	body.IdentificationState, body.IsGuessed = ptr.ValueOrZero(series.IdentificationState), series.IsGuessed
	body.FetchFailure = fetchFailureBody(series.FetchFailure, series.FetchFailedAt)
	body.RefreshRequestedAt = ptr.ValueOrZero(series.RefreshRequestedAt)
	body.Unavailable = unavailableBody(series.Unreadable, server.nodeNamer(ctx, dependencies))
	body.Seasons = make([]SeasonBody, 0, len(series.Seasons))
	for _, season := range series.Seasons {
		body.Seasons = append(body.Seasons, SeasonBody{
			SeasonNumber: season.Number, Name: season.Name, EpisodeCount: season.EpisodeCount,
			PosterSha256: season.PosterSha256, StillSha256: season.StillSha256, IsWatched: season.IsWatched,
		})
	}
	body.Orderings = make([]OrderingBody, 0, len(series.Orderings))
	for _, ordering := range series.Orderings {
		body.Orderings = append(body.Orderings, OrderingBody{
			Name: ordering.Name, Kind: saidValue(ordering.Kind), Label: ordering.Label,
			ListedCount:  ordering.ListedCount,
			EpisodeCount: ordering.EpisodeCount, IsAwaitingFetch: ordering.IsAwaitingFetch,
			IsUnlisted: ordering.IsUnlisted,
		})
	}
	if series.FilesOrdering != library.OrderingAired {
		body.FilesOrdering = series.FilesOrdering
	}
	body.NotInOrderingCount = series.NotInOrderingCount
	body.IsWatched = series.IsWatched
	whereabouts, err := server.whereabouts(ctx, dependencies, principal.Reach, series.ID, itemKindSeries)
	if err != nil {
		return nil, err
	}
	body.Libraries = libraryNameBodies(whereabouts.Libraries)
	body.Art = artBody(series.Art)
	next, hasNext, err := dependencies.Browser.NextUp(ctx, principal.Reach, principal.UserID, series.ID,
		series.Ordering)
	if err != nil {
		return nil, server.libraryError(ctx, "read a series' next episode", err)
	}
	if hasNext {
		nextUp := server.nextUpBody(ctx, dependencies, next)
		body.NextUp = &nextUp
	}
	if body.ShownSeason, err = server.shownSeasonBody(ctx, dependencies, principal, body, season); err != nil {
		return nil, err
	}
	return body, nil
}

// shownSeasonBody is the season a series' page shows and its first page of episodes (D477 §1): the
// one asked for, where the series has it; else that of the person's next episode; else the first, as
// the page chooses; nil for a series with no season.
func (server *Server) shownSeasonBody(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, series *SeriesBody, asked int64,
) (*ShownSeasonBody, error) {
	if len(series.Seasons) == 0 {
		return nil, nil //nolint:nilnil // a series with no season shows none
	}
	has := func(number int64) bool {
		return slices.ContainsFunc(series.Seasons, func(each SeasonBody) bool { return each.SeasonNumber == number })
	}
	shown := series.Seasons[0].SeasonNumber
	switch {
	case asked >= 0 && has(asked):
		shown = asked
	case series.NextUp != nil && has(series.NextUp.GroupNumber):
		shown = series.NextUp.GroupNumber
	}
	page, err := dependencies.Browser.Season(ctx, principal.Reach, library.SeasonQuery{
		SeriesID: series.ID, UserID: principal.UserID, Season: shown, Limit: shownSeasonPageSize,
	})
	if err != nil {
		return nil, server.libraryError(ctx, "read a series' shown season", err)
	}
	episodes := server.episodePageOutput(ctx, dependencies, page).Body
	return &ShownSeasonBody{SeasonNumber: shown, Episodes: episodes.Episodes, NextCursor: episodes.NextCursor}, nil
}
