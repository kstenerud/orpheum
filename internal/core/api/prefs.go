// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/playback"
)

// FilePrefsBody are a person's choices for one file (cut-a §5).
type FilePrefsBody struct {
	SubtitleOffsetMs int64 `doc:"Shows the subtitles this much later, or earlier when negative" json:"subtitleOffsetMs"`
	// SubtitleChoice is the file's subtitle choice, absent when it has none (D219).
	SubtitleChoice *SubtitleChoiceBody `json:"subtitleChoice,omitempty"`
	// IsDialogueBoostOn is whether the person plays with the dialogue boost, in every file (D83).
	IsDialogueBoostOn bool `json:"isDialogueBoostOn"`
	// IsLevellingOn is whether the person plays with loudness levelled, in every file (D226 §5).
	IsLevellingOn bool `json:"isLevellingOn"`
}

// SubtitleChoiceBody is a subtitle choice for one file: a track in it, or off (D219).
type SubtitleChoiceBody struct {
	StreamID int64 `doc:"The chosen track, a stream of the file's; absent when off" json:"streamId,omitempty"`
	// FetchedSubtitleID is the chosen track when it is a subtitle fetched for the file (D224).
	FetchedSubtitleID int64 `doc:"The chosen track, a subtitle fetched for the file" json:"fetchedSubtitleId,omitempty"`
	IsOff             bool  `json:"isOff,omitempty"`
	// IsCarriedFromWork is a choice made in another file of the work and carried here.
	IsCarriedFromWork bool `json:"isCarriedFromWork,omitempty" readOnly:"true"`
}

// SetFilePrefsInput changes a person's choices for one file: what it names, and nothing else.
type SetFilePrefsInput struct {
	AuthorizationHeader
	FileID int64 `minimum:"1" path:"id"`
	Body   struct {
		SubtitleOffsetMs *int64 `json:"subtitleOffsetMs,omitempty" maximum:"600000" minimum:"-600000" required:"false"`
		// SubtitleChoice, when given, is the file's own choice, carried to the work's other files that
		// have none of their own (D219).
		SubtitleChoice *SubtitleChoiceBody `json:"subtitleChoice,omitempty" required:"false"`
	}
}

func (server *Server) registerFilePrefs() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setFilePreferences", Method: http.MethodPut, Path: "/files/{id}/preferences",
		Summary: "Keep a person's choices for one file: its subtitle offset, its subtitle track",
		Tags:    []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
	}, server.setFilePreferences)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setMyPreferences", Method: http.MethodPut, Path: "/me/preferences",
		Summary: "Keep a person's own choices, which hold in every file they play: the boost, levelling",
		Tags:    []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
	}, server.setMyPreferences)
}

func filePrefsBody(prefs playback.FilePrefs) FilePrefsBody {
	body := FilePrefsBody{
		SubtitleOffsetMs: prefs.SubtitleOffsetMs, IsDialogueBoostOn: prefs.IsDialogueBoostOn,
		IsLevellingOn: prefs.IsLevelling,
	}
	if choice := prefs.SubtitleChoice; choice != nil {
		body.SubtitleChoice = &SubtitleChoiceBody{
			StreamID: choice.StreamID, FetchedSubtitleID: choice.FetchedSubtitleID, IsOff: choice.IsOff,
			IsCarriedFromWork: choice.IsCarriedFromWork,
		}
	}
	return body
}

// SetMyPrefsInput changes a person's own choices, which hold in every file: what it names only.
type SetMyPrefsInput struct {
	AuthorizationHeader
	Body struct {
		// IsDialogueBoostOn, when given, is whether the person plays with the dialogue boost (D222 §3).
		IsDialogueBoostOn *bool `json:"isDialogueBoostOn,omitempty" required:"false"`
		// IsLevellingOn, when given, is whether the person plays with loudness levelled (D226 §5).
		IsLevellingOn *bool `json:"isLevellingOn,omitempty" required:"false"`
	}
}

func (server *Server) setMyPreferences(ctx context.Context, input *SetMyPrefsInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if isOn := input.Body.IsDialogueBoostOn; isOn != nil {
		if err := dependencies.Playback.SetDialogueBoost(ctx, principal.UserID, *isOn); err != nil {
			return nil, server.playbackError(ctx, "keep a person's dialogue boost", err)
		}
	}
	if isOn := input.Body.IsLevellingOn; isOn != nil {
		if err := dependencies.Playback.SetLevelling(ctx, principal.UserID, *isOn); err != nil {
			return nil, server.playbackError(ctx, "keep whether a person's loudness is levelled", err)
		}
	}
	return &struct{}{}, nil
}

func (server *Server) setFilePreferences(ctx context.Context, input *SetFilePrefsInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if offset := input.Body.SubtitleOffsetMs; offset != nil {
		err = dependencies.Playback.SetSubtitleOffset(ctx, principal.UserID, input.FileID, *offset)
	}
	if choice := input.Body.SubtitleChoice; err == nil && choice != nil {
		named := 0
		for _, isNamed := range []bool{choice.IsOff, choice.StreamID != 0, choice.FetchedSubtitleID != 0} {
			if isNamed {
				named++
			}
		}
		if named != 1 {
			return nil, newError(http.StatusBadRequest, CodeInvalidRequest,
				"a subtitle choice is one track, a stream or a fetched one, or off")
		}
		err = dependencies.Playback.ChooseSubtitle(ctx, principal.UserID, input.FileID, playback.SubtitleChoice{
			TrackRef: playback.TrackRef{StreamID: choice.StreamID, FetchedSubtitleID: choice.FetchedSubtitleID},
			IsOff:    choice.IsOff,
		})
	}
	if err != nil {
		return nil, server.playbackError(ctx, "keep a file's preferences", err)
	}
	return &struct{}{}, nil
}
