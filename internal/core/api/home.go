// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

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
}

// HomeOutput is the home screen (D188): Continue Watching first. Next Up comes after M3.
type HomeOutput struct {
	Body struct {
		ContinueWatching []InProgressBody `json:"continueWatching"`
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
		Summary: "The home screen: what the person stopped in the middle of", Tags: []string{"Browse"},
	}, server.getHome)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "hideProgress", Method: http.MethodPost, Path: "/items/{id}/hide",
		Summary:       "Hide a work from Continue Watching until it next has activity; not a completion (D188)",
		Tags:          []string{tagPlayback},
		DefaultStatus: http.StatusNoContent,
	}, server.hideProgress)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "unhideProgress", Method: http.MethodDelete, Path: "/items/{id}/hide",
		Summary: "Undo hiding a work", Tags: []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
	}, server.unhideProgress)
}

func (server *Server) getHome(ctx context.Context, input *struct{ AuthorizationHeader }) (*HomeOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	rows, err := dependencies.Browser.ContinueWatching(ctx, principal.UserID, principal.Reach)
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
		})
	}
	return output, nil
}

func (server *Server) hideProgress(ctx context.Context, input *HideInput) (*struct{}, error) {
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

func (server *Server) unhideProgress(ctx context.Context, input *HideInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := dependencies.Playback.Unhide(ctx, principal.UserID, input.ItemID); err != nil {
		return nil, server.playbackError(ctx, "unhide an item", err)
	}
	return &struct{}{}, nil
}
