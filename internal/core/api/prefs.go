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
	SubtitleOffsetMs int64 `doc:"Shows the subtitles this much later, or earlier when negative" json:"subtitleOffsetMs"`
	// Subtitle is the file's subtitle choice, absent when it has none (D219).
	Subtitle *SubtitleChoiceBody `json:"subtitle,omitempty"`
	// IsDialogueBoost is whether the person plays with the dialogue boost, in every file (D83).
	IsDialogueBoost bool `json:"isDialogueBoost"`
}

// SubtitleChoiceBody is a subtitle choice for one file: a track in it, or off (D219).
type SubtitleChoiceBody struct {
	StreamID int64 `doc:"The chosen track; absent when off" json:"streamId,omitempty"`
	IsOff    bool  `json:"isOff,omitempty"`
	// IsCarried is a choice made in another file of the work and carried here.
	IsCarried bool `json:"isCarried,omitempty" readOnly:"true"`
}

// SetFilePrefsInput changes a person's choices for one file: what it names, and nothing else.
type SetFilePrefsInput struct {
	AuthorizationHeader
	FileID int64 `minimum:"1" path:"id"`
	Body   struct {
		SubtitleOffsetMs *int64 `json:"subtitleOffsetMs,omitempty" maximum:"600000" minimum:"-600000" required:"false"`
		// Subtitle, when given, is the file's own choice, carried to the work's other files that
		// have none of their own (D219).
		Subtitle *SubtitleChoiceBody `json:"subtitle,omitempty" required:"false"`
		// IsDialogueBoost, when given, is the person's, and holds in every file they play (D83).
		IsDialogueBoost *bool `json:"isDialogueBoost,omitempty" required:"false"`
	}
}

func (server *Server) registerFilePrefs() {
	huma.Register(server.api, huma.Operation{
		OperationID: "setFilePrefs", Method: http.MethodPut, Path: "/files/{id}/prefs",
		Summary: "Keep a person's choices for one file: its subtitle offset, its subtitle track; " +
			"and the dialogue boost, theirs in every file",
		Tags: []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
	}, server.setFilePrefs)
}

func filePrefsBody(prefs playback.FilePrefs) FilePrefsBody {
	body := FilePrefsBody{SubtitleOffsetMs: prefs.SubtitleOffsetMs, IsDialogueBoost: prefs.IsDialogueBoost}
	if choice := prefs.Subtitle; choice != nil {
		body.Subtitle = &SubtitleChoiceBody{StreamID: choice.StreamID, IsOff: choice.IsOff, IsCarried: choice.IsCarried}
	}
	return body
}

func (server *Server) setFilePrefs(ctx context.Context, input *SetFilePrefsInput) (*struct{}, error) {
	dependencies, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if offset := input.Body.SubtitleOffsetMs; offset != nil {
		err = dependencies.Playback.SetSubtitleOffset(ctx, principal.UserID, input.FileID, *offset)
	}
	if choice := input.Body.Subtitle; err == nil && choice != nil {
		if choice.IsOff == (choice.StreamID != 0) {
			return nil, newError(http.StatusBadRequest, CodeInvalidRequest,
				"a subtitle choice is a track or off, not both")
		}
		err = dependencies.Playback.ChooseSubtitle(ctx, principal.UserID, input.FileID, playback.SubtitleChoice{
			StreamID: choice.StreamID, IsOff: choice.IsOff,
		})
	}
	if isOn := input.Body.IsDialogueBoost; err == nil && isOn != nil {
		err = dependencies.Playback.SetDialogueBoost(ctx, principal.UserID, *isOn)
	}
	switch {
	case errors.Is(err, playback.ErrNoFile):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such file to play")
	case errors.Is(err, playback.ErrNoTrack):
		return nil, newError(http.StatusBadRequest, CodeInvalidRequest, "this file has no such subtitle track")
	case err != nil:
		return nil, server.internal(ctx, "keep a file's preferences", err)
	}
	return &struct{}{}, nil
}
