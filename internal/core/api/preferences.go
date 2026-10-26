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
	// IsAutoplayOn is whether the next episode plays by itself after a countdown as this one ends
	// (plans/up-next.md).
	IsAutoplayOn bool `json:"isAutoplayOn"`
}

// MyPreferencesBody are a person's own choices, which hold in every file they play.
type MyPreferencesBody struct {
	IsDialogueBoostOn bool `doc:"Whether they play with the dialogue boost (D83)" json:"isDialogueBoostOn"`
	IsLevellingOn     bool `doc:"Whether they play with loudness levelled (D226 §5)" json:"isLevellingOn"`
	IsAutoplayOn      bool `doc:"Whether the next episode plays by itself after a countdown" json:"isAutoplayOn"`
	// LeftOutLibraryIDs are the libraries the person leaves out of Films and Series (D349 §8).
	LeftOutLibraryIDs []int64 `doc:"The libraries left out of Films and Series, which gather every other library of those kinds" json:"leftOutLibraryIds"` //nolint:lll // one struct tag
}

// MyPreferencesOutput are a person's own choices.
type MyPreferencesOutput struct {
	Body MyPreferencesBody
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
		OperationID: "setSeriesPreferences", Method: http.MethodPut, Path: "/collections/{id}/preferences",
		Summary: "Keep a person's choices in one series: the ordering they watch it in, which its pages, " +
			"its episodes' labels and its Next Up follow",
		Tags: []string{tagBrowse}, DefaultStatus: http.StatusNoContent,
	}, server.setSeriesPreferences)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setMyPreferences", Method: http.MethodPut, Path: "/me/preferences",
		Summary: "Keep a person's own choices, which hold in every file they play: the boost, levelling, " +
			"autoplay",
		Tags: []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
	}, server.setMyPreferences)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getMyPreferences", Method: http.MethodGet, Path: "/me/preferences",
		Summary: "A person's own choices, which hold in every file they play, as their settings show them",
		Tags:    []string{tagPlayback},
	}, server.getMyPreferences)
}

func (server *Server) getMyPreferences(ctx context.Context, input *AuthorizationHeader) (*MyPreferencesOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	switches, err := dependencies.Preferences.Switches(ctx, principal.UserID)
	if err != nil {
		return nil, server.playbackError(ctx, "read a person's own choices", err)
	}
	leftOut, err := dependencies.Preferences.LeftOutLibraries(ctx, principal.UserID)
	if err != nil {
		return nil, server.playbackError(ctx, "read the libraries a person left out", err)
	}
	return &MyPreferencesOutput{Body: MyPreferencesBody{
		IsDialogueBoostOn: switches.IsDialogueBoostOn, IsLevellingOn: switches.IsLevellingOn,
		IsAutoplayOn: switches.IsAutoplayOn, LeftOutLibraryIDs: leftOut,
	}}, nil
}

func playbackPreferencesBody(prefs preferences.Preferences) PlaybackPreferencesBody {
	body := PlaybackPreferencesBody{
		SubtitleOffsetMs: prefs.SubtitleOffsetMs, IsDialogueBoostOn: prefs.IsDialogueBoostOn,
		IsLevellingOn: prefs.IsLevellingOn, IsAutoplayOn: prefs.IsAutoplayOn,
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
		// IsAutoplayOn, when given, is whether the next episode plays by itself (plans/up-next.md).
		IsAutoplayOn *bool `json:"isAutoplayOn,omitempty" required:"false"`
		// LeftOutLibraryIDs, when given, are the libraries the person leaves out of Films and Series
		// (D349 §8), all of them: an empty list leaves none out.
		LeftOutLibraryIDs []int64 `json:"leftOutLibraryIds,omitempty" maxItems:"1000" required:"false"`
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
	if isOn := input.Body.IsAutoplayOn; isOn != nil {
		if err := dependencies.Preferences.SetAutoplay(ctx, principal.UserID, *isOn); err != nil {
			return nil, server.playbackError(ctx, "keep whether a person's next episode plays by itself", err)
		}
	}
	if leftOut := input.Body.LeftOutLibraryIDs; leftOut != nil {
		if err := dependencies.Preferences.SetLeftOutLibraries(ctx, principal.UserID, leftOut); err != nil {
			return nil, server.playbackError(ctx, "keep the libraries a person left out", err)
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

// SetSeriesPreferencesInput changes a person's choices in one series: what it names only.
type SetSeriesPreferencesInput struct {
	AuthorizationHeader
	SeriesID int64 `minimum:"1" path:"id"`
	Body     struct {
		// EpisodeOrdering, when given, is the ordering the person watches the series in, by its name,
		// one the series has (D159 §1): aired, or a provider's.
		EpisodeOrdering *string `json:"episodeOrdering,omitempty" maxLength:"200" required:"false"`
	}
}

func (server *Server) setSeriesPreferences(ctx context.Context, input *SetSeriesPreferencesInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	ordering := input.Body.EpisodeOrdering
	if ordering == nil {
		return &struct{}{}, nil
	}
	hasIt, err := dependencies.Browser.HasOrdering(ctx, principal.Reach, input.SeriesID, *ordering)
	if err != nil {
		return nil, server.libraryError(ctx, "read a series' orderings", err)
	}
	if !hasIt {
		return nil, invalidRequestAt(http.StatusBadRequest, "body.episodeOrdering", "this series has no such ordering")
	}
	err = dependencies.Preferences.ChooseEpisodeOrdering(ctx, principal.UserID, input.SeriesID, *ordering)
	if err != nil {
		return nil, server.libraryError(ctx, "keep the ordering a person watches a series in", err)
	}
	// An episode group chosen before anyone fetched it is fetched now, on their act (D170); the page
	// says so until it is.
	if _, err := dependencies.Refresher.QueueChosenOrderings(ctx, input.SeriesID); err != nil {
		return nil, server.libraryError(ctx, "queue the fetch of a series' chosen ordering", err)
	}
	return &struct{}{}, nil
}
