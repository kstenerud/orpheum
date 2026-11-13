// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library"
)

// homeNewestShown is how many of the newest films or series a row of Home shows, a best guess: three
// screens of a Fire TV's row of six, the rest a list's own page away. A justified constant.
const homeNewestShown = 18

// InProgressBody is one row of Continue Watching: a work, and the file of it the person was most
// recently partway through.
type InProgressBody struct {
	ID           int64             `json:"id"`
	Name         string            `json:"name"`
	Year         int64             `json:"year,omitempty"`
	PosterSha256 string            `json:"posterSha256,omitempty"`
	FileID       int64             `json:"fileId"`
	PositionMs   int64             `json:"positionMs"`
	DurationMs   int64             `doc:"0 when the file's length is unknown" json:"durationMs"`
	FileName     string            `doc:"Which file it resumes, when two are partway" json:"fileName,omitempty"`
	Episode      *EpisodePlaceBody `json:"episode,omitempty"`
	// Unavailable says why none of its files can be read now; absent when one can (D241 §3).
	Unavailable *UnavailableBody `json:"unavailable,omitempty"`
	IsWatched   bool             `doc:"Whether the person watched it before, and watches it again (D370)" json:"isWatched,omitempty"` //nolint:lll // one struct tag
}

// HomeOutput is the home screen (D188): every row of it in one answer, so the rows come together and
// one poll keeps them (D476, principles §30). Continue Watching first, then the newest films and the
// newest series, each of the libraries the person keeps on Home (D366 §5). Next Up comes after M3.
type HomeOutput struct {
	Body struct {
		ContinueWatching []InProgressBody `json:"continueWatching"`
		// NewestFilms and NewestSeries are the newest films and series, newest first, at most 18 of
		// each, as GET /items?sort=added lists them.
		NewestFilms  []ItemEntryBody `maxItems:"18" json:"newestFilms"`
		NewestSeries []ItemEntryBody `maxItems:"18" json:"newestSeries"`
	}
}

// HideInput hides a work from Continue Watching, or undoes that.
type HideInput struct {
	AuthorizationHeader
	ItemID int64 `minimum:"1" path:"id"`
}

func (server *Server) registerHome() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getHome", Method: http.MethodGet, Path: "/home",
		Summary:    "The home screen: what the person stopped in the middle of, and the newest films and series",
		Tags:       []string{"Browse"},
		Extensions: callableBy(capabilitySignedIn),
	}, server.getHome)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "markHidden", Method: http.MethodPut, Path: "/items/{id}/hidden",
		Summary:       "Hide a work from Continue Watching until it next has activity; not a completion (D188)",
		Tags:          []string{tagPlayback},
		DefaultStatus: http.StatusNoContent,
		Extensions:    callableBy(capabilitySignedIn),
	}, server.markHidden)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "unmarkHidden", Method: http.MethodDelete, Path: "/items/{id}/hidden",
		Summary: "Undo hiding a work", Tags: []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
		Extensions: callableBy(capabilitySignedIn),
	}, server.unmarkHidden)
	// Marked watched by hand, a work leaves Continue Watching too (D370 §4).
	server.registerWatched()
}

func (server *Server) getHome(ctx context.Context, input *struct{ AuthorizationHeader }) (*HomeOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	reach, err := server.homeReach(ctx, dependencies, principal)
	if err != nil {
		return nil, err
	}
	rows, err := dependencies.Browser.ContinueWatching(ctx, principal.UserID, reach)
	if err != nil {
		return nil, server.libraryError(ctx, "list Continue Watching", err)
	}
	output := &HomeOutput{}
	output.Body.ContinueWatching = make([]InProgressBody, 0, len(rows))
	nodeName := server.nodeNamer(ctx, dependencies)
	for _, row := range rows {
		output.Body.ContinueWatching = append(output.Body.ContinueWatching, InProgressBody{
			ID: row.ID, Name: row.Name, Year: row.Year, PosterSha256: row.PosterSha256,
			FileID: row.FileID, PositionMs: row.PositionMs, DurationMs: row.DurationMs, FileName: row.FileName,
			Episode: episodePlaceBody(row.Episode), Unavailable: unavailableBody(row.Unreadable, nodeName),
			IsWatched: row.IsWatched,
		})
	}
	output.Body.NewestFilms, err = server.homeNewest(ctx, dependencies, principal, reach, library.ListFilms)
	if err != nil {
		return nil, err
	}
	output.Body.NewestSeries, err = server.homeNewest(ctx, dependencies, principal, reach, library.ListSeries)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// homeNewest is a row of Home's newest of kind, ListFilms or ListSeries, of the libraries reach holds.
func (server *Server) homeNewest(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, reach auth.Reach, kind string,
) ([]ItemEntryBody, error) {
	page, err := dependencies.Browser.List(ctx, library.ListQuery{
		Kinds: []string{kind}, Sort: library.SortAdded, Limit: homeNewestShown, Reach: reach, UserID: principal.UserID,
	})
	if err != nil {
		return nil, server.libraryError(ctx, "list Home's newest "+kind, err)
	}
	nodeName := server.nodeNamer(ctx, dependencies)
	entries := make([]ItemEntryBody, 0, len(page.Entries))
	for _, entry := range page.Entries {
		entries = append(entries, itemEntryBody(entry, nodeName))
	}
	return entries, nil
}

func (server *Server) markHidden(ctx context.Context, input *HideInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedInReachingItem(ctx, input.Authorization, input.ItemID,
		server.playbackError)
	if err != nil {
		return nil, err
	}
	if err := dependencies.Playback.Hide(ctx, principal.UserID, input.ItemID); err != nil {
		return nil, server.playbackError(ctx, "hide an item", err)
	}
	return &struct{}{}, nil
}

func (server *Server) unmarkHidden(ctx context.Context, input *HideInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := dependencies.Playback.Unhide(ctx, principal.UserID, input.ItemID); err != nil {
		return nil, server.playbackError(ctx, "unhide an item", err)
	}
	return &struct{}{}, nil
}
