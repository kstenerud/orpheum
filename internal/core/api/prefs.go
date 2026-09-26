// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/playback"
)

// FilePrefsBody are a person's choices for one file (cut-a §5).
type FilePrefsBody struct {
	SubtitleOffsetMs int64 `doc:"Shows the subtitles this much later, or earlier when negative" json:"subtitleOffsetMs" maximum:"600000" minimum:"-600000"` //nolint:lll // one struct tag
}

// SetFilePrefsInput keeps a person's choices for one file.
type SetFilePrefsInput struct {
	AuthorizationHeader
	FileID int64 `minimum:"1" path:"id"`
	Body   FilePrefsBody
}

func (server *Server) registerFilePrefs() {
	huma.Register(server.api, huma.Operation{
		OperationID: "setFilePrefs", Method: http.MethodPut, Path: "/files/{id}/prefs",
		Summary: "Keep a person's choices for one file: its subtitle offset", Tags: []string{tagPlayback},
		DefaultStatus: http.StatusNoContent,
	}, server.setFilePrefs)
}

func (server *Server) setFilePrefs(ctx context.Context, input *SetFilePrefsInput) (*struct{}, error) {
	dependencies, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	prefs := playback.FilePrefs{SubtitleOffsetMs: input.Body.SubtitleOffsetMs}
	err = dependencies.Playback.SetFilePrefs(ctx, principal.UserID, input.FileID, prefs)
	switch {
	case errors.Is(err, playback.ErrNoFile):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such file to play")
	case err != nil:
		return nil, server.internal(ctx, "keep a file's preferences", err)
	}
	return &struct{}{}, nil
}
