// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/playback"
)

// cacheFrame is a key frame's Cache-Control: the file as it is gives the same picture at the same
// place, but it is the asker's own, and nothing on the server keeps it (D545 §4, §5).
const cacheFrame = "private, max-age=3600"

// FrameInput asks for the key frame of a file at a place.
type FrameInput struct {
	MediaCredentials
	ID   int64 `doc:"The file" path:"id"`
	AtMs int64 `doc:"The place, in milliseconds from the film's start: the keyframe at or before it is the picture" minimum:"0" query:"atMs" required:"true"` //nolint:lll // one struct tag
}

func (server *Server) registerFrame() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getFileFrame", Method: http.MethodGet, Path: "/bytes/files/{id}/frame",
		Summary: "The keyframe at or before a place of a file's picture, 640 across, made by its node at once " +
			"and kept nowhere: what a cast's remote shows (D545)",
		Tags: []string{tagPlayback},
		Responses: map[string]*huma.Response{
			"200": {Description: "The key frame", Content: map[string]*huma.MediaType{webpType: {}}},
		},
		Extensions: callableBy(capabilityMedia, schemeSession, schemeMedia),
	}, server.getFileFrame)
}

// getFileFrame answers GET /bytes/files/{id}/frame: the key frame at or before atMs of a file the
// person reaches, read from a place they reach (D363 §8) and relayed from its node's byte socket.
// A file they do not reach is one that does not exist (D243 §4); a place outside the film is 422.
func (server *Server) getFileFrame(ctx context.Context, input *FrameInput) (*huma.StreamResponse, error) {
	dependencies, principal, err := server.requireSignedInForMedia(ctx, input.MediaCredentials)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Browser.ReachFile(ctx, principal.Reach, input.ID); err != nil {
		return nil, server.playbackError(ctx, "reach a file", err)
	}
	source, err := dependencies.Playback.FrameSource(ctx, principal.Reach, input.ID, input.AtMs)
	switch {
	case errors.Is(err, playback.ErrPlaceOutsideFilm):
		return nil, huma.Error422UnprocessableEntity("the place is not in the film")
	case errors.Is(err, playback.ErrNoPicture):
		return nil, huma.Error404NotFound("the file has no picture")
	case err != nil:
		return nil, server.playbackError(ctx, "find a file's key frame", err)
	}
	// The node's 404 is a file not there or no picture at the place; either way, no frame.
	return server.relay(ctx, dependencies, relayRequest{http.MethodGet, source, nil, cacheFrame},
		func(int) error { return fileNotThere() })
}
