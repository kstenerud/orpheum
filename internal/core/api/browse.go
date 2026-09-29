// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

const tagBrowse = "Browse"

// EntryBody is one film or series in a list.
type EntryBody struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Year                int64  `json:"year,omitempty"`
	IdentificationState string `json:"identificationState"`
	LibraryID           int64  `json:"libraryId"`
	AddedAt             int64  `json:"addedAt"`
	PosterHash          string `doc:"Its poster's hash: GET /bytes/art/{hash}/portrait-m.webp" json:"posterHash,omitempty"` //nolint:lll // one struct tag
}

// ArtBody is the images an item shows, by the hash GET /bytes/art/{hash}/{size}.{format} takes.
type ArtBody struct {
	PosterHash   string `json:"posterHash,omitempty"`
	BackdropHash string `json:"backdropHash,omitempty"`
	StillHash    string `doc:"An episode's frame" json:"stillHash,omitempty"`
}

func artBody(art library.Art) ArtBody {
	return ArtBody{PosterHash: art.Poster, BackdropHash: art.Backdrop, StillHash: art.Still}
}

// TitlesInput asks for one page of films or series.
type TitlesInput struct {
	AuthorizationHeader
	Kind   string `enum:"movie,series" query:"kind" required:"true"`
	Sort   string `enum:"name,added" query:"sort" required:"true"`
	Cursor string `doc:"From the previous page's nextCursor" query:"cursor"`
	Limit  int64  `maximum:"500" minimum:"1" query:"limit" required:"true"`
}

// TitlesOutput is one page.
type TitlesOutput struct {
	Body struct {
		Titles     []EntryBody `json:"titles"`
		NextCursor string      `doc:"Empty on the last page" json:"nextCursor,omitempty"`
	}
}

// StreamBody is one stream of a file.
type StreamBody struct {
	ID       int64   `doc:"Stable while the file is unchanged" json:"id"`
	Kind     string  `json:"kind"`
	Codec    *string `json:"codec,omitempty"`
	Language *string `doc:"BCP 47" json:"language,omitempty"`
	Title    *string `json:"title,omitempty"`
	Channels *int64  `json:"channels,omitempty"`
}

// FileDetailBody is one file of a version; its path is shown to an admin only.
type FileDetailBody struct {
	ID        int64        `json:"id"`
	Path      string       `json:"path,omitempty"`
	SizeBytes *int64       `json:"sizeBytes,omitempty"`
	Streams   []StreamBody `json:"streams"`
}

// VersionBody is one version of a title (D155): its files, in part order.
type VersionBody struct {
	ID        int64            `json:"id"`
	EditionID *int64           `doc:"Absent when the cut is unknown" json:"editionId,omitempty"`
	Files     []FileDetailBody `json:"files"`
}

// ExtraBody is one extra of a title (D162).
type ExtraBody struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// EpisodePlaceBody is an episode's series and place in its aired order.
type EpisodePlaceBody struct {
	SeriesID      int64  `json:"seriesId"`
	SeriesName    string `json:"seriesName"`
	SeasonNumber  int64  `json:"seasonNumber"`
	EpisodeNumber int64  `json:"episodeNumber"`
}

// ItemDetailOutput is a title's detail page.
type ItemDetailOutput struct {
	Body struct {
		ID                  int64             `json:"id"`
		Kind                string            `enum:"movie,episode,extra" json:"kind"`
		Name                string            `json:"name"`
		Overview            string            `json:"overview,omitempty"`
		Tagline             string            `json:"tagline,omitempty"`
		Year                int64             `json:"year,omitempty"`
		RuntimeMs           int64             `json:"runtimeMs,omitempty"`
		IdentificationState string            `json:"identificationState"`
		FollowedSource      string            `json:"followedSource,omitempty"`
		LibraryID           int64             `json:"libraryId"`
		Versions            []VersionBody     `json:"versions"`
		Extras              []ExtraBody       `json:"extras"`
		Episode             *EpisodePlaceBody `json:"episode,omitempty"`
		Art                 ArtBody           `json:"art"`
	}
}

// EpisodeBody is one episode of a series.
type EpisodeBody struct {
	ID                  int64  `json:"id"`
	SeasonNumber        int64  `json:"seasonNumber"`
	EpisodeNumber       int64  `json:"episodeNumber"`
	Name                string `json:"name"`
	Year                int64  `json:"year,omitempty"`
	RuntimeMs           int64  `json:"runtimeMs,omitempty"`
	IdentificationState string `json:"identificationState"`
	HasLiveFile         bool   `doc:"Whether it has a live file" json:"hasLiveFile"`
	StillHash           string `doc:"Its still's hash: GET /bytes/art/{hash}/landscape-s.webp" json:"stillHash,omitempty"` //nolint:lll // one struct tag
}

// CollectionOutput is a series with its episodes in aired order.
type CollectionOutput struct {
	Body struct {
		ID                  int64         `json:"id"`
		Kind                string        `enum:"series" json:"kind"`
		Name                string        `json:"name"`
		Overview            string        `json:"overview,omitempty"`
		Year                int64         `json:"year,omitempty"`
		IdentificationState string        `json:"identificationState,omitempty"`
		LibraryID           int64         `json:"libraryId"`
		Episodes            []EpisodeBody `json:"episodes"`
		Art                 ArtBody       `json:"art"`
	}
}

// SearchInput is a search.
type SearchInput struct {
	AuthorizationHeader
	Query string `query:"q" required:"true"`
	Limit int64  `maximum:"100" minimum:"1" query:"limit" required:"true"`
}

// HitBody is one search result: a title, or a series.
type HitBody struct {
	ID   int64  `json:"id"`
	Kind string `enum:"movie,episode,extra,series" json:"kind"`
	Name string `json:"name"`
	Year int64  `json:"year,omitempty"`
}

// SearchOutput is a search's results, best first.
type SearchOutput struct {
	Body struct {
		Results []HitBody `json:"results"`
	}
}

func (server *Server) registerBrowse() {
	tags := []string{tagBrowse}
	huma.Register(server.api, huma.Operation{
		OperationID: "getTitles", Method: http.MethodGet, Path: "/titles",
		Summary: "Films or series, by name in the library's locale or newest first", Tags: tags,
	}, server.getTitles)
	huma.Register(server.api, huma.Operation{
		OperationID: "getTitle", Method: http.MethodGet, Path: "/items/{id}",
		Summary: "A title's detail: versions, files, streams, extras and, for an episode, its place", Tags: tags,
	}, server.getItem)
	huma.Register(server.api, huma.Operation{
		OperationID: "getSeries", Method: http.MethodGet, Path: "/collections/{id}",
		Summary: "A series with its episodes in aired order", Tags: tags,
	}, server.getCollection)
	huma.Register(server.api, huma.Operation{
		OperationID: "search", Method: http.MethodGet, Path: "/search",
		Summary: "Titles and series by name, in any script (D48)", Tags: tags,
	}, server.search)
}

func (server *Server) getTitles(ctx context.Context, input *TitlesInput) (*TitlesOutput, error) {
	dependencies, _, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	page, err := dependencies.Browse.List(ctx, library.ListQuery{
		Kind: input.Kind, Sort: input.Sort, Cursor: input.Cursor, Limit: input.Limit,
	})
	if err != nil {
		return nil, server.browseError(ctx, "list titles", err)
	}
	output := &TitlesOutput{}
	output.Body.Titles = make([]EntryBody, 0, len(page.Entries))
	for _, entry := range page.Entries {
		output.Body.Titles = append(output.Body.Titles, EntryBody{
			ID: entry.ID, Name: entry.Name, Year: entry.Year, IdentificationState: entry.Ident,
			LibraryID: entry.LibraryID,
			AddedAt:   entry.AddedAt, PosterHash: entry.Poster,
		})
	}
	output.Body.NextCursor = page.NextCursor
	return output, nil
}

func (server *Server) getItem(ctx context.Context, input *ItemInput) (*ItemDetailOutput, error) {
	dependencies, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	detail, err := dependencies.Browse.Title(ctx, input.ID)
	if err != nil {
		return nil, server.browseError(ctx, "read a title", err)
	}
	output := &ItemDetailOutput{}
	body := &output.Body
	body.ID, body.Kind, body.Name = detail.ID, detail.Kind, ptr.ValueOrZero(detail.Name)
	body.IdentificationState = detail.Identification
	body.Overview = ptr.ValueOrZero(detail.Overview)
	body.Tagline = ptr.ValueOrZero(detail.Tagline)
	body.FollowedSource = ptr.ValueOrZero(detail.FollowedSource)
	body.Year = ptr.ValueOrZero(detail.Year)
	body.RuntimeMs = ptr.ValueOrZero(detail.RuntimeMs)
	body.LibraryID = ptr.ValueOrZero(detail.LibraryID)
	body.Versions = make([]VersionBody, 0, len(detail.Versions))
	for _, version := range detail.Versions {
		versionBody := VersionBody{ID: version.ID, EditionID: version.EditionID, Files: []FileDetailBody{}}
		for _, file := range version.Files {
			fileBody := FileDetailBody{ID: file.ID, SizeBytes: file.Size, Streams: []StreamBody{}}
			if principal.IsAdmin() {
				fileBody.Path = file.Path
			}
			for _, stream := range file.Streams {
				fileBody.Streams = append(fileBody.Streams, StreamBody{
					ID: stream.ID, Kind: stream.Kind, Codec: stream.Codec, Language: stream.Lang,
					Title:    stream.TitleRaw,
					Channels: stream.ChannelCount,
				})
			}
			versionBody.Files = append(versionBody.Files, fileBody)
		}
		body.Versions = append(body.Versions, versionBody)
	}
	body.Extras = make([]ExtraBody, 0, len(detail.Extras))
	for _, extra := range detail.Extras {
		body.Extras = append(body.Extras, ExtraBody{
			ID: extra.ExtraTitle, Kind: extra.ExtraType, Name: ptr.ValueOrZero(extra.Name),
		})
	}
	body.Art = artBody(detail.Art)
	body.Episode = episodePlaceBody(detail.Episode)
	return output, nil
}

// episodePlaceBody is an episode's place as the API shows it, or nil for a title in no series.
func episodePlaceBody(place *sqlcgen.GetEpisodePlaceRow) *EpisodePlaceBody {
	if place == nil {
		return nil
	}
	return &EpisodePlaceBody{
		SeriesID: place.CollectionID, SeriesName: ptr.ValueOrZero(place.SeriesName),
		SeasonNumber: ptr.ValueOrZero(place.GroupNumber), EpisodeNumber: ptr.ValueOrZero(place.NumberInGroup),
	}
}

func (server *Server) getCollection(ctx context.Context, input *ItemInput) (*CollectionOutput, error) {
	dependencies, _, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	series, err := dependencies.Browse.Series(ctx, input.ID)
	if err != nil {
		return nil, server.browseError(ctx, "read a series", err)
	}
	output := &CollectionOutput{}
	body := &output.Body
	body.ID, body.Kind = series.ID, "series"
	body.Name, body.Overview = ptr.ValueOrZero(series.Name), ptr.ValueOrZero(series.Overview)
	body.Year, body.LibraryID = ptr.ValueOrZero(series.Year), ptr.ValueOrZero(series.LibraryID)
	body.IdentificationState = ptr.ValueOrZero(series.Identification)
	body.Episodes = make([]EpisodeBody, 0, len(series.Episodes))
	for _, episode := range series.Episodes {
		body.Episodes = append(body.Episodes, EpisodeBody{
			ID: episode.TitleID, SeasonNumber: ptr.ValueOrZero(episode.GroupNumber),
			EpisodeNumber: ptr.ValueOrZero(episode.NumberInGroup),
			Name:          ptr.ValueOrZero(episode.Name),
			Year:          ptr.ValueOrZero(episode.Year), RuntimeMs: ptr.ValueOrZero(episode.RuntimeMs),
			IdentificationState: episode.Identification,
			HasLiveFile:         episode.Available, StillHash: series.Stills[episode.TitleID],
		})
	}
	body.Art = artBody(series.Art)
	return output, nil
}

func (server *Server) search(ctx context.Context, input *SearchInput) (*SearchOutput, error) {
	dependencies, _, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	hits, err := dependencies.Browse.Search(ctx, input.Query, input.Limit)
	if err != nil {
		return nil, server.internal(ctx, "search", err)
	}
	output := &SearchOutput{}
	output.Body.Results = make([]HitBody, 0, len(hits))
	for _, hit := range hits {
		kind := "series"
		if hit.TitleKind != nil {
			kind = *hit.TitleKind
		}
		output.Body.Results = append(output.Body.Results, HitBody{
			ID: hit.ItemID, Kind: kind, Name: hit.Name, Year: ptr.ValueOrZero(hit.Year),
		})
	}
	return output, nil
}

// browseError maps browse's refusals to API errors.
func (server *Server) browseError(ctx context.Context, doing string, err error) error {
	switch {
	case errors.Is(err, library.ErrNoItem):
		return newError(http.StatusNotFound, CodeNotFound, "no such title or series")
	case errors.Is(err, library.ErrBadCursor):
		return newError(http.StatusUnprocessableEntity, CodeInvalidRequest, "the cursor is not one this server gave")
	}
	return server.internal(ctx, doing, err)
}
