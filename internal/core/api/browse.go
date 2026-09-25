// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/library"
)

const tagBrowse = "Browse"

// EntryBody is one film or series in a list.
type EntryBody struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Year      int64  `json:"year,omitempty"`
	Ident     string `json:"ident"`
	LibraryID int64  `json:"libraryId"`
	AddedAt   int64  `json:"addedAt"`
	Poster    string `doc:"Its poster's hash: GET /art/{hash}/portrait-m.webp" json:"poster,omitempty"`
}

// ArtBody is the images an item shows, by the hash GET /art/{hash}/{size}.{format} takes.
type ArtBody struct {
	Poster   string `json:"poster,omitempty"`
	Backdrop string `json:"backdrop,omitempty"`
	Still    string `doc:"An episode's frame" json:"still,omitempty"`
}

func artBody(art library.Art) ArtBody {
	return ArtBody{Poster: art.Poster, Backdrop: art.Backdrop, Still: art.Still}
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
	Lang     *string `doc:"BCP 47" json:"lang,omitempty"`
	Title    *string `json:"title,omitempty"`
	Channels *int64  `json:"channels,omitempty"`
}

// FileDetailBody is one file of a version; its path is shown to an admin only.
type FileDetailBody struct {
	ID      int64        `json:"id"`
	Path    string       `json:"path,omitempty"`
	Size    *int64       `json:"size,omitempty"`
	Streams []StreamBody `json:"streams"`
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
	Type string `json:"type"`
	Name string `json:"name"`
}

// EpisodePlaceBody is an episode's series and place in its aired order.
type EpisodePlaceBody struct {
	SeriesID   int64  `json:"seriesId"`
	SeriesName string `json:"seriesName"`
	Season     int64  `json:"season"`
	Number     int64  `json:"number"`
}

// ItemDetailOutput is a title's detail page.
type ItemDetailOutput struct {
	Body struct {
		ID        int64             `json:"id"`
		Kind      string            `enum:"movie,episode,extra" json:"kind"`
		Name      string            `json:"name"`
		Overview  string            `json:"overview,omitempty"`
		Tagline   string            `json:"tagline,omitempty"`
		Year      int64             `json:"year,omitempty"`
		RuntimeMs int64             `json:"runtimeMs,omitempty"`
		Ident     string            `json:"ident"`
		Source    string            `json:"source,omitempty"`
		LibraryID int64             `json:"libraryId"`
		Versions  []VersionBody     `json:"versions"`
		Extras    []ExtraBody       `json:"extras"`
		Episode   *EpisodePlaceBody `json:"episode,omitempty"`
		Art       ArtBody           `json:"art"`
	}
}

// EpisodeBody is one episode of a series.
type EpisodeBody struct {
	ID        int64  `json:"id"`
	Season    int64  `json:"season"`
	Number    int64  `json:"number"`
	Name      string `json:"name"`
	Year      int64  `json:"year,omitempty"`
	RuntimeMs int64  `json:"runtimeMs,omitempty"`
	Ident     string `json:"ident"`
	Available bool   `doc:"Whether it has a live file" json:"available"`
	Still     string `doc:"Its still's hash: GET /art/{hash}/landscape-s.webp" json:"still,omitempty"`
}

// CollectionOutput is a series with its episodes in aired order.
type CollectionOutput struct {
	Body struct {
		ID        int64         `json:"id"`
		Kind      string        `enum:"series" json:"kind"`
		Name      string        `json:"name"`
		Overview  string        `json:"overview,omitempty"`
		Year      int64         `json:"year,omitempty"`
		Ident     string        `json:"ident,omitempty"`
		LibraryID int64         `json:"libraryId"`
		Episodes  []EpisodeBody `json:"episodes"`
		Art       ArtBody       `json:"art"`
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
		OperationID: "getItem", Method: http.MethodGet, Path: "/items/{id}",
		Summary: "A title's detail: versions, files, streams, extras and, for an episode, its place", Tags: tags,
	}, server.getItem)
	huma.Register(server.api, huma.Operation{
		OperationID: "getCollection", Method: http.MethodGet, Path: "/collections/{id}",
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
		List: input.Kind, Sort: input.Sort, Cursor: input.Cursor, Limit: input.Limit,
	})
	if err != nil {
		return nil, server.browseError(ctx, "list titles", err)
	}
	output := &TitlesOutput{}
	output.Body.Titles = make([]EntryBody, 0, len(page.Entries))
	for _, entry := range page.Entries {
		output.Body.Titles = append(output.Body.Titles, EntryBody{
			ID: entry.ID, Name: entry.Name, Year: entry.Year, Ident: entry.Ident, LibraryID: entry.LibraryID,
			AddedAt: entry.AddedAt, Poster: entry.Poster,
		})
	}
	output.Body.NextCursor = page.Next
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
	body.ID, body.Kind, body.Name, body.Ident = detail.ID, detail.Kind, text(detail.Name), detail.Ident
	body.Overview, body.Tagline, body.Source = text(detail.Overview), text(detail.Tagline), text(detail.Source)
	body.Year, body.RuntimeMs, body.LibraryID = number(detail.Year), number(detail.RuntimeMs), number(detail.LibraryID)
	body.Versions = make([]VersionBody, 0, len(detail.Versions))
	for _, version := range detail.Versions {
		versionBody := VersionBody{ID: version.ID, EditionID: version.EditionID, Files: []FileDetailBody{}}
		for _, file := range version.Files {
			fileBody := FileDetailBody{ID: file.ID, Size: file.Size, Streams: []StreamBody{}}
			if principal.IsAdmin() {
				fileBody.Path = file.Path
			}
			for _, stream := range file.Streams {
				fileBody.Streams = append(fileBody.Streams, StreamBody{
					ID: stream.ID, Kind: stream.Kind, Codec: stream.Codec, Lang: stream.Lang, Title: stream.TitleRaw,
					Channels: stream.Channels,
				})
			}
			versionBody.Files = append(versionBody.Files, fileBody)
		}
		body.Versions = append(body.Versions, versionBody)
	}
	body.Extras = make([]ExtraBody, 0, len(detail.Extras))
	for _, extra := range detail.Extras {
		body.Extras = append(body.Extras, ExtraBody{ID: extra.ExtraTitle, Type: extra.Type, Name: text(extra.Name)})
	}
	body.Art = artBody(detail.Art)
	if place := detail.Episode; place != nil {
		body.Episode = &EpisodePlaceBody{
			SeriesID: place.CollectionID, SeriesName: text(place.SeriesName), Season: number(place.Grp),
			Number: number(place.Number),
		}
	}
	return output, nil
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
	body.ID, body.Kind, body.Name, body.Overview = series.ID, "series", text(series.Name), text(series.Overview)
	body.Year, body.Ident, body.LibraryID = number(series.Year), text(series.Ident), number(series.LibraryID)
	body.Episodes = make([]EpisodeBody, 0, len(series.Episodes))
	for _, episode := range series.Episodes {
		body.Episodes = append(body.Episodes, EpisodeBody{
			ID: episode.TitleID, Season: number(episode.Grp), Number: number(episode.Number), Name: text(episode.Name),
			Year: number(episode.Year), RuntimeMs: number(episode.RuntimeMs), Ident: episode.Ident,
			Available: episode.Available, Still: series.Stills[episode.TitleID],
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
			ID: hit.ItemID, Kind: kind, Name: hit.Name, Year: number(hit.Year),
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
