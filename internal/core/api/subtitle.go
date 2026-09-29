// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/playback"
)

// SubtitleTrackBody is one text subtitle track, in the two forms it is served in (cut-a §6).
type SubtitleTrackBody struct {
	StreamID int64 `doc:"A track inside the file, or a subtitle file beside it" json:"streamId,omitempty"`
	// FetchedSubtitleID is a subtitle fetched on request for the file; it is unverified (D224).
	FetchedSubtitleID int64  `doc:"A subtitle fetched on request; unverified" json:"fetchedSubtitleId,omitempty"`
	Language          string `doc:"BCP 47; empty when the file names none" json:"language"`
	Title             string `json:"title"`
	IsDefault         bool   `json:"isDefault"`
	IsForced          bool   `json:"isForced"`
	IsHearingImpaired bool   `json:"isHearingImpaired"`
	ASSURL            string `doc:"The track as ASS, for JASSUB" json:"assUrl"`
	VTTURL            string `doc:"The track as WebVTT" json:"vttUrl"`
}

// AudioRenditionBody is one audio rendition: a source track's standard mix, or its boosted one.
type AudioRenditionBody struct {
	SourceAudioIndex  int    `json:"sourceAudioIndex"`
	Language          string `json:"language"`
	IsDialogueBoosted bool   `json:"isDialogueBoosted"`
	// Levelling brings the track's stereo sound to the level (D226).
	Levelling LevellingBody `json:"levelling"`
	// CarriedGainDB is already in the rendition's sound: the player applies the levelling less
	// this, and on its own when levelling is off (D226 §4).
	CarriedGainDB float64 `doc:"dB already in the sound, put in when it was prepared" json:"carriedGainDb"`
}

// LevellingBody is what brings a sound to the level (D226): its gain, once it is measured.
type LevellingBody struct {
	IsMeasured bool    `doc:"Absent a measurement, the sound plays as it is" json:"isMeasured"`
	GainDB     float64 `doc:"Applied after the player's volume; negative lowers a loud film" json:"gainDb"`
}

func levellingBody(levelling playback.Levelling) LevellingBody {
	return LevellingBody{IsMeasured: levelling.IsMeasured, GainDB: levelling.GainDB}
}

// SubtitleInput asks for one of a playback's subtitle tracks, in one form.
type SubtitleInput struct {
	MediaCredentials
	Token string `doc:"The playback's media token" path:"token"`
	File  string `doc:"<streamId>.ass or <streamId>.vtt, or f<fetchedId>.ass or .vtt" path:"file"`
}

func (server *Server) registerSubtitles() {
	huma.Register(server.api, huma.Operation{
		OperationID: "getSubtitle", Method: http.MethodGet, Path: "/bytes/media/sub/{token}/{file}",
		Summary: "One of a playback's subtitle tracks, as ASS or WebVTT (D217 §6)", Tags: []string{tagPlayback},
		Responses: map[string]*huma.Response{
			"200": {Description: "The track", Content: map[string]*huma.MediaType{
				assContentType: {}, vttContentType: {},
			}},
			"425": {Description: "Not extracted yet; ask again after Retry-After"},
		},
	}, server.subtitle)
}

// The subtitles' media types.
const (
	assContentType = "text/x-ssa; charset=utf-8"
	vttContentType = "text/vtt; charset=utf-8"
)

// subtitleFileName is the only shape a subtitle may be asked for by: its stream, or f and its
// fetched id, and its form.
var subtitleFileName = regexp.MustCompile(`^(f?)(\d{1,18})\.(ass|vtt)$`)

// subtitleTracks lists a playback's text tracks, each with its URL in both forms.
func subtitleTracks(started playback.Started) []SubtitleTrackBody {
	tracks := make([]SubtitleTrackBody, 0, len(started.Subtitles))
	for _, track := range started.Subtitles {
		name := strconv.FormatInt(track.StreamID, 10)
		if track.IsFetched() {
			name = "f" + strconv.FormatInt(track.FetchedSubtitleID, 10)
		}
		base := Prefix + "/bytes/media/sub/" + started.Token + "/" + name
		tracks = append(tracks, SubtitleTrackBody{
			StreamID: track.StreamID, FetchedSubtitleID: track.FetchedSubtitleID, Language: track.Language,
			Title:     track.Title,
			IsDefault: track.Default, IsForced: track.Forced, IsHearingImpaired: track.HearingImpaired,
			ASSURL: base + ".ass", VTTURL: base + ".vtt",
		})
	}
	return tracks
}

// subtitle serves one of a playback's subtitle tracks from the core's own copy. While it is being
// extracted it is 425 with a Retry-After, as a rendition's file is.
func (server *Server) subtitle(ctx context.Context, input *SubtitleInput) (*huma.StreamResponse, error) {
	match := subtitleFileName.FindStringSubmatch(input.File)
	if match == nil {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such subtitle")
	}
	id, err := strconv.ParseInt(match[2], 10, 64)
	if err != nil {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such subtitle")
	}
	track := playback.TrackRef{StreamID: id}
	if match[1] == "f" {
		track = playback.TrackRef{FetchedSubtitleID: id}
	}
	format, contentType := playback.FormatASS, assContentType
	if match[3] == string(playback.FormatWebVTT) {
		format, contentType = playback.FormatWebVTT, vttContentType
	}
	authorization := input.authorization()
	dependencies, principal, err := server.signedIn(ctx, authorization)
	if err != nil {
		return nil, err
	}
	text, err := dependencies.Playback.Subtitle(
		ctx, input.Token, track, format, principal.UserID, sessionKey(authorization))
	switch {
	case errors.Is(err, playback.ErrNoToken), errors.Is(err, playback.ErrNoFile):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such playback; start it again")
	case errors.Is(err, playback.ErrNoSubtitle):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such subtitle")
	case err != nil:
		return nil, server.internal(ctx, "find a subtitle", err)
	}
	switch text.Readiness {
	case playback.Preparing:
		return nil, tooEarly()
	case playback.Failed:
		return nil, newError(http.StatusBadGateway, CodePrepareFailed,
			"this subtitle could not be extracted: "+text.Error)
	case playback.Complete:
	}
	return &huma.StreamResponse{Body: func(stream huma.Context) {
		stream.SetHeader("Content-Type", contentType)
		stream.SetHeader("Content-Length", strconv.Itoa(len(text.Text)))
		stream.SetHeader("Cache-Control", "private, max-age=86400")
		stream.SetStatus(http.StatusOK)
		// A player that stops reading ends the copy; there is nothing to report to it.
		_, _ = stream.BodyWriter().Write(text.Text) //nolint:errcheck // see above
	}}, nil
}
