// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/preferences"
)

// PlaybackPreferencesBody are the choices a person's playback of one file starts with: the file's
// subtitle offset, the subtitle and audio tracks worked out from what was last used in it or the
// person's last choice in its work (D230), and the person's dialogue boost and levelling (cut-a §5,
// D223 §10).
type PlaybackPreferencesBody struct {
	SubtitleOffsetMs int64 `doc:"Shows the subtitles this much later, or earlier when negative" json:"subtitleOffsetMs"`
	// SubtitleChoice is the subtitle track to start with, absent to start without (D223 §3, D230).
	SubtitleChoice *SubtitleChoiceBody `json:"subtitleChoice,omitempty"`
	// AudioChoice is the audio track the person chose, absent when the file's main one plays (D230 §5).
	AudioChoice *AudioChoiceBody `json:"audioChoice,omitempty"`
	// IsDialogueBoostOn is whether the person plays with the dialogue boost, in every file (D83).
	IsDialogueBoostOn bool `json:"isDialogueBoostOn"`
	// IsLevellingOn is whether the person plays with loudness levelled, in every file (D226 §5).
	IsLevellingOn bool `json:"isLevellingOn"`
}

// SubtitleChoiceBody is a subtitle choice in one file: a track in it, or off.
type SubtitleChoiceBody struct {
	StreamID int64 `doc:"The chosen track, a stream of the file's; absent when off" json:"streamId,omitempty"`
	// FetchedSubtitleID is the chosen track when it is a subtitle fetched for the file (D224).
	FetchedSubtitleID int64 `doc:"The chosen track, a subtitle fetched for the file" json:"fetchedSubtitleId,omitempty"`
	IsOff             bool  `json:"isOff,omitempty"`
}

// AudioChoiceBody is an audio track chosen in one file.
type AudioChoiceBody struct {
	StreamID int64 `doc:"The chosen track, an audio stream of the file's" json:"streamId" minimum:"1"`
}

// SetFilePreferencesInput changes a person's choices for one file: what it names, and nothing else.
type SetFilePreferencesInput struct {
	AuthorizationHeader
	FileID int64 `minimum:"1" path:"id"`
	Body   struct {
		SubtitleOffsetMs *int64 `json:"subtitleOffsetMs,omitempty" maximum:"600000" minimum:"-600000" required:"false"`
		// SubtitleChoice, when given, is what the file plays with again, and its language, or off, the
		// person's last choice in the file's work, which its unwatched files follow (D230).
		SubtitleChoice *SubtitleChoiceBody `json:"subtitleChoice,omitempty" required:"false"`
		// AudioChoice, when given, is kept as a subtitle choice is (D230 §5).
		AudioChoice *AudioChoiceBody `json:"audioChoice,omitempty" required:"false"`
	}
}

func (server *Server) registerPreferences() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setFilePreferences", Method: http.MethodPut, Path: "/files/{id}/preferences",
		Summary: "Keep a person's choices in one file: its subtitle offset, its subtitle and audio tracks, " +
			"whose languages its work's unwatched files follow",
		Tags: []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
	}, server.setFilePreferences)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setMyPreferences", Method: http.MethodPut, Path: "/me/preferences",
		Summary: "Keep a person's own choices, which hold in every file they play: the boost, levelling",
		Tags:    []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
	}, server.setMyPreferences)
}

func playbackPreferencesBody(prefs preferences.Preferences) PlaybackPreferencesBody {
	body := PlaybackPreferencesBody{
		SubtitleOffsetMs: prefs.SubtitleOffsetMs, IsDialogueBoostOn: prefs.IsDialogueBoostOn,
		IsLevellingOn: prefs.IsLevellingOn,
	}
	if track := prefs.Subtitle; track != nil {
		body.SubtitleChoice = &SubtitleChoiceBody{StreamID: track.StreamID, FetchedSubtitleID: track.FetchedSubtitleID}
	}
	if track := prefs.Audio; track != nil {
		body.AudioChoice = &AudioChoiceBody{StreamID: track.StreamID}
	}
	return body
}

// SetMyPreferencesInput changes a person's own choices, which hold in every file: what it names only.
type SetMyPreferencesInput struct {
	AuthorizationHeader
	Body struct {
		// IsDialogueBoostOn, when given, is whether the person plays with the dialogue boost (D222 §3).
		IsDialogueBoostOn *bool `json:"isDialogueBoostOn,omitempty" required:"false"`
		// IsLevellingOn, when given, is whether the person plays with loudness levelled (D226 §5).
		IsLevellingOn *bool `json:"isLevellingOn,omitempty" required:"false"`
	}
}

func (server *Server) setMyPreferences(ctx context.Context, input *SetMyPreferencesInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if isOn := input.Body.IsDialogueBoostOn; isOn != nil {
		if err := dependencies.Preferences.SetDialogueBoost(ctx, principal.UserID, *isOn); err != nil {
			return nil, server.playbackError(ctx, "keep a person's dialogue boost", err)
		}
	}
	if isOn := input.Body.IsLevellingOn; isOn != nil {
		if err := dependencies.Preferences.SetLevelling(ctx, principal.UserID, *isOn); err != nil {
			return nil, server.playbackError(ctx, "keep whether a person's loudness is levelled", err)
		}
	}
	return &struct{}{}, nil
}

func (server *Server) setFilePreferences(ctx context.Context, input *SetFilePreferencesInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedInReachingFile(ctx, input.Authorization, input.FileID)
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
			return nil, invalidRequestAt(http.StatusBadRequest, "body.subtitleChoice",
				"a subtitle choice is one track, a stream or a fetched one, or off")
		}
		err = dependencies.Playback.ChooseSubtitle(ctx, principal.UserID, input.FileID, preferences.Used{
			TrackRef: preferences.TrackRef{StreamID: choice.StreamID, FetchedSubtitleID: choice.FetchedSubtitleID},
			IsOff:    choice.IsOff,
		})
		if errors.Is(err, preferences.ErrNoTrack) {
			return nil, invalidRequestAt(http.StatusBadRequest, "body.subtitleChoice", "this file has no such track")
		}
	}
	if choice := input.Body.AudioChoice; err == nil && choice != nil {
		err = dependencies.Playback.ChooseAudio(ctx, principal.UserID, input.FileID,
			preferences.TrackRef{StreamID: choice.StreamID})
		if errors.Is(err, preferences.ErrNoTrack) {
			return nil, invalidRequestAt(http.StatusBadRequest, "body.audioChoice", "this file has no such track")
		}
	}
	if err != nil {
		return nil, server.playbackError(ctx, "keep a file's preferences", err)
	}
	return &struct{}{}, nil
}
