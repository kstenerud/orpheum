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
	StreamID int64 `doc:"The chosen track, a stream of the file's; absent when off" json:"streamId,omitempty"`
	// FetchedID is the chosen track when it is a subtitle fetched for the file (D224).
	FetchedID int64 `doc:"The chosen track, a subtitle fetched for the file" json:"fetchedId,omitempty"`
	IsOff     bool  `json:"isOff,omitempty"`
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
	}
}

func (server *Server) registerFilePrefs() {
	huma.Register(server.api, huma.Operation{
		OperationID: "setFilePrefs", Method: http.MethodPut, Path: "/files/{id}/prefs",
		Summary: "Keep a person's choices for one file: its subtitle offset, its subtitle track",
		Tags:    []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
	}, server.setFilePrefs)
	huma.Register(server.api, huma.Operation{
		OperationID: "setMyPrefs", Method: http.MethodPut, Path: "/me/prefs",
		Summary: "Keep a person's own choices, which hold in every file they play: the dialogue boost",
		Tags:    []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
	}, server.setMyPrefs)
}

func filePrefsBody(prefs playback.FilePrefs) FilePrefsBody {
	body := FilePrefsBody{SubtitleOffsetMs: prefs.SubtitleOffsetMs, IsDialogueBoost: prefs.IsDialogueBoost}
	if choice := prefs.Subtitle; choice != nil {
		body.Subtitle = &SubtitleChoiceBody{
			StreamID: choice.StreamID, FetchedID: choice.FetchedID, IsOff: choice.IsOff, IsCarried: choice.IsCarried,
		}
	}
	return body
}

// SetMyPrefsInput changes a person's own choices, which hold in every file: what it names only.
type SetMyPrefsInput struct {
	AuthorizationHeader
	Body struct {
		// IsDialogueBoost, when given, is whether the person plays with the dialogue boost (D222 §3).
		IsDialogueBoost *bool `json:"isDialogueBoost,omitempty" required:"false"`
	}
}

func (server *Server) setMyPrefs(ctx context.Context, input *SetMyPrefsInput) (*struct{}, error) {
	dependencies, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if isOn := input.Body.IsDialogueBoost; isOn != nil {
		if err := dependencies.Playback.SetDialogueBoost(ctx, principal.UserID, *isOn); err != nil {
			return nil, server.internal(ctx, "keep a person's dialogue boost", err)
		}
	}
	return &struct{}{}, nil
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
		named := 0
		for _, isNamed := range []bool{choice.IsOff, choice.StreamID != 0, choice.FetchedID != 0} {
			if isNamed {
				named++
			}
		}
		if named != 1 {
			return nil, newError(http.StatusBadRequest, CodeInvalidRequest,
				"a subtitle choice is one track, a stream or a fetched one, or off")
		}
		err = dependencies.Playback.ChooseSubtitle(ctx, principal.UserID, input.FileID, playback.SubtitleChoice{
			TrackRef: playback.TrackRef{StreamID: choice.StreamID, FetchedID: choice.FetchedID}, IsOff: choice.IsOff,
		})
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
