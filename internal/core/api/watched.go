// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/playback"
)

// WatchedStateBody is a title's watched mark as it stood for the person before a mark by hand, which
// PUT /me/watched puts back (D370 §4): Undo, not a question.
type WatchedStateBody struct {
	ItemID    int64  `json:"itemId"`
	WatchedAt *int64 `doc:"When it became watched, in Unix milliseconds; absent while it was not" json:"watchedAt,omitempty"`         //nolint:lll // one struct tag
	MarkedAt  *int64 `doc:"When the person last marked it by hand, in Unix milliseconds; absent for never" json:"markedAt,omitempty"` //nolint:lll // one struct tag
}

// WatchedMarkOutput is what a mark by hand changed: each title's mark as it stood before, for its Undo.
type WatchedMarkOutput struct {
	Body struct {
		Previous []WatchedStateBody `doc:"Each title's mark as it stood before; PUT /me/watched with them undoes the mark" json:"previous"` //nolint:lll // one struct tag
	}
}

// GroupMarkInput names a group of a series to mark, a season: a group of the ordering the person
// watches it in (D416 §3).
type GroupMarkInput struct {
	AuthorizationHeader
	ID    int64 `path:"id"`
	Group int64 `doc:"A group of the ordering the person watches it in, a season or a disc; aired's 0 for the specials and the episodes with no season" minimum:"0" path:"group"` //nolint:lll // one struct tag
}

// RestoreWatchedInput puts marks back as a mark by hand's answer gave them.
type RestoreWatchedInput struct {
	AuthorizationHeader
	Body struct {
		States []WatchedStateBody `json:"states" maxItems:"10000"`
	}
}

func (server *Server) registerWatched() {
	tags := []string{tagPlayback}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "markWatched", Method: http.MethodPut, Path: "/items/{id}/watched", Tags: tags,
		Summary: "Mark an item watched by hand: a film, episode or other video, or every episode of a series " +
			"with a file, in the ordering the person watches it in; it leaves Continue Watching and plays " +
			"next from its start (D370)",
		Extensions: callableBy(capabilitySignedIn),
	}, server.markWatched)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "unmarkWatched", Method: http.MethodDelete, Path: "/items/{id}/watched", Tags: tags,
		Summary: "Mark an item unwatched by hand, as never watched: a film, episode or other video, or every " +
			"episode of a series with a file, in the ordering the person watches it in (D370)",
		Extensions: callableBy(capabilitySignedIn),
	}, server.unmarkWatched)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "markGroupWatched", Method: http.MethodPut, Path: "/items/{id}/groups/{group}/watched",
		Tags: tags, Summary: "Mark every episode of a season with a file watched by hand (D370)",
		Extensions: callableBy(capabilitySignedIn),
	}, server.markGroupWatched)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "unmarkGroupWatched", Method: http.MethodDelete,
		Path: "/items/{id}/groups/{group}/watched", Tags: tags,
		Summary:    "Mark every episode of a season with a file unwatched by hand (D370)",
		Extensions: callableBy(capabilitySignedIn),
	}, server.unmarkGroupWatched)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "restoreWatched", Method: http.MethodPut, Path: "/me/watched", Tags: tags,
		Summary:       "Put the person's watched marks back as a mark by hand's answer gave them: its Undo (D370)",
		DefaultStatus: http.StatusNoContent,
		Extensions:    callableBy(capabilitySignedIn),
	}, server.restoreWatched)
}

func (server *Server) markWatched(ctx context.Context, input *ItemInput) (*WatchedMarkOutput, error) {
	return server.markItem(ctx, input, true)
}

func (server *Server) unmarkWatched(ctx context.Context, input *ItemInput) (*WatchedMarkOutput, error) {
	return server.markItem(ctx, input, false)
}

func (server *Server) markGroupWatched(ctx context.Context, input *GroupMarkInput) (*WatchedMarkOutput, error) {
	return server.markGroup(ctx, input, true)
}

func (server *Server) unmarkGroupWatched(ctx context.Context, input *GroupMarkInput) (*WatchedMarkOutput, error) {
	return server.markGroup(ctx, input, false)
}

// markItem marks an item of any kind (D425 §8): a title itself, a series every episode of it with a
// file, in the ordering the person watches it in.
func (server *Server) markItem(ctx context.Context, input *ItemInput, isWatched bool) (*WatchedMarkOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	isCollection, err := dependencies.Browser.IsCollection(ctx, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "read an item's kind", err)
	}
	if isCollection {
		return server.markEpisodes(ctx, dependencies, principal, input.ID, nil, isWatched)
	}
	err = dependencies.Browser.TitleToMark(ctx, principal.Reach, input.ID)
	if errors.Is(err, library.ErrNotWatchable) {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "path.id", err.Error())
	}
	if err != nil {
		return nil, server.libraryError(ctx, "find a title to mark", err)
	}
	return server.mark(ctx, dependencies, principal.UserID, []int64{input.ID}, isWatched)
}

func (server *Server) markGroup(
	ctx context.Context, input *GroupMarkInput, isWatched bool,
) (*WatchedMarkOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.markEpisodes(ctx, dependencies, principal, input.ID, &input.Group, isWatched)
}

// markEpisodes marks a series' episodes with a file, in the ordering principal watches it in: those of
// one group, or every one when group is nil.
func (server *Server) markEpisodes(
	ctx context.Context, dependencies *Dependencies, principal auth.Principal, seriesID int64, group *int64,
	isWatched bool,
) (*WatchedMarkOutput, error) {
	ids, err := dependencies.Browser.EpisodesToMark(ctx, principal.Reach, principal.UserID, seriesID, group)
	if err != nil {
		return nil, server.libraryError(ctx, "find a series' episodes to mark", err)
	}
	return server.mark(ctx, dependencies, principal.UserID, ids, isWatched)
}

// mark marks titleIDs for userID, answering with their marks before.
func (server *Server) mark(
	ctx context.Context, dependencies *Dependencies, userID int64, titleIDs []int64, isWatched bool,
) (*WatchedMarkOutput, error) {
	before, err := dependencies.Playback.MarkWatched(ctx, userID, titleIDs, isWatched)
	if err != nil {
		return nil, server.playbackError(ctx, "mark titles watched", err)
	}
	output := &WatchedMarkOutput{}
	output.Body.Previous = make([]WatchedStateBody, 0, len(before))
	for _, state := range before {
		output.Body.Previous = append(output.Body.Previous, WatchedStateBody{
			ItemID: state.TitleID, WatchedAt: state.WatchedAt, MarkedAt: state.MarkedAt,
		})
	}
	return output, nil
}

func (server *Server) restoreWatched(ctx context.Context, input *RestoreWatchedInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	states := make([]playback.WatchedState, 0, len(input.Body.States))
	for _, state := range input.Body.States {
		states = append(states, playback.WatchedState{
			TitleID: state.ItemID, WatchedAt: state.WatchedAt, MarkedAt: state.MarkedAt,
		})
	}
	err = dependencies.Playback.RestoreWatched(ctx, principal.UserID, states)
	if errors.Is(err, playback.ErrBadWatchedState) {
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "body.states", err.Error())
	}
	if err != nil {
		return nil, server.playbackError(ctx, "put watched marks back", err)
	}
	return &struct{}{}, nil
}
