// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/playback"
)

const tagPlayback = "Playback"

// HopNode names the node as the leg that failed.
const HopNode Hop = "node"

// HopProvider is the failing leg when a provider, such as OpenSubtitles, is (cut-a §5).
const HopProvider Hop = "provider"

// NodeBytes reads from a node's byte socket.
type NodeBytes interface {
	Do(ctx context.Context, nodeID int64, method, target string, header http.Header) (*http.Response, error)
}

// MenuBody is what a client can play, one list per axis, most preferred first (D21, D217 §1).
type MenuBody struct {
	VideoCodecs     []string `doc:"Must list h264-sdr" enum:"h264-sdr,hevc-sdr,hevc-hdr10,av1-sdr,av1-hdr10" json:"videoCodecs"` //nolint:lll // one struct tag
	AudioCodecs     []string `doc:"Must list aac-stereo" enum:"aac-stereo,aac-multi,ac3,eac3,opus,flac" json:"audioCodecs"`      //nolint:lll // one struct tag
	SubtitleFormats []string `enum:"ass,webvtt" json:"subtitleFormats"`
	Containers      []string `doc:"Containers played as they are" enum:"mp4,mkv,mpegts" json:"containers"`
}

// StartPlaybackInput asks to play a file.
type StartPlaybackInput struct {
	AuthorizationHeader
	Body struct {
		FileID            int64    `json:"fileId" minimum:"1"`
		PlayableFormats   MenuBody `json:"playableFormats"`
		Network           string   `doc:"Where the client is" enum:"local,remote" json:"network"`
		PreferredDelivery string   `doc:"A person's override" enum:"direct,prepare" json:"preferredDelivery,omitempty" required:"false"` //nolint:lll // one struct tag
	}
}

// StartPlaybackOutput is the decision (cut-a §6).
type StartPlaybackOutput struct {
	Body struct {
		Delivery string `enum:"direct,hls" json:"delivery"`
		URL      string `doc:"Where the bytes are: the file, or a rendition's master playlist" json:"url"`
		Profile  string `enum:"remux,h264-8m" json:"profile,omitempty"`
		// NotDirectReasons say why it is not played directly, to show on screen (D188).
		NotDirectReasons []string `enum:"container,video_codec,audio_codec,bandwidth,dialogue_boost,user_choice" json:"notDirectReasons"` //nolint:lll // one struct tag
		// Preparing is whether the rendition is still being made; its playlist answers 425 until
		// the first segment exists.
		IsPreparing bool `json:"isPreparing"`
		// Audio are a rendition's audio renditions, in its master playlist's order.
		Audio []AudioRenditionBody `json:"audio,omitempty"`
		// Subtitles are the file's text subtitle tracks, each extracted or on its way; a track's
		// URLs answer 425 until it is (D217 §6).
		Subtitles []SubtitleTrackBody `json:"subtitles"`
		// Prefs are the person's choices for this file, which the player starts with.
		Prefs FilePrefsBody `json:"prefs"`
		// ResumePositionMs is where the player starts, without asking (D188): where the person
		// left the file, or 0.
		ResumePositionMs int64 `json:"resumePositionMs"`
		// MediaToken names this playback in its reports; it grants nothing alone (D217 §2).
		MediaToken string `json:"mediaToken"`
		// AudioOutput is how the device asking makes sound, absent until it is asked: the player
		// asks before it plays, and asks for the decision again with the answer (D222).
		AudioOutput AudioOutput `json:"audioOutput,omitempty"`
		// Levelling brings the main sound to the level: the file's own when played directly, the
		// main track's stereo mix when prepared, where each audio rendition carries its own (D226).
		Levelling LevellingBody `json:"levelling"`
	}
}

// MasterInput asks for a prepared playback's master playlist.
type MasterInput struct {
	MediaCredentials
	Token string `doc:"The playback's media token" path:"token"`
}

// RenditionFileInput asks for one file of one of a prepared playback's renditions.
type RenditionFileInput struct {
	MediaCredentials
	Token     string `doc:"The playback's media token" path:"token"`
	Rendition string `doc:"video, audio0, audio0-boost, ..." path:"rendition"`
	File      string `doc:"stream.m3u8, init.mp4 or a segment" path:"file"`
	Range     string `header:"Range"`
}

// DirectInput asks for a direct playback's bytes.
type DirectInput struct {
	MediaCredentials
	Token   string `doc:"The playback's media token" path:"token"`
	Range   string `header:"Range"`
	IfRange string `header:"If-Range"`
}

func (server *Server) registerPlayback() {
	huma.Register(server.api, huma.Operation{
		OperationID: "startPlayback", Method: http.MethodPost, Path: "/playback",
		Summary: "Decide how a file plays on this client, and where its bytes are (cut-a §6)",
		Tags:    []string{tagPlayback},
	}, server.startPlayback)
	for method, id := range map[string]string{http.MethodGet: "getDirect", http.MethodHead: "headDirect"} {
		huma.Register(server.api, huma.Operation{
			OperationID: id, Method: method, Path: "/bytes/media/direct/{token}",
			Summary: "A file's own bytes, relayed from its node, honouring Range", Tags: []string{tagPlayback},
			Responses: map[string]*huma.Response{
				"200": {Description: "The file", Content: map[string]*huma.MediaType{"application/octet-stream": {}}},
				"206": {Description: "The range asked for"},
			},
		}, func(ctx context.Context, input *DirectInput) (*huma.StreamResponse, error) {
			return server.direct(ctx, method, input)
		})
	}
	responses := map[string]*huma.Response{
		"200": {Description: "The file", Content: map[string]*huma.MediaType{
			"application/vnd.apple.mpegurl": {}, "video/iso.segment": {}, "video/mp4": {},
		}},
		"425": {Description: "Not written yet: the rendition is being prepared; ask again after Retry-After"},
	}
	huma.Register(server.api, huma.Operation{
		OperationID: "getMasterPlaylist", Method: http.MethodGet, Path: "/bytes/media/hls/{token}/master.m3u8",
		Summary: "A prepared playback's master playlist, relayed from its node", Tags: []string{tagPlayback},
		Responses: responses,
	}, func(ctx context.Context, input *MasterInput) (*huma.StreamResponse, error) {
		return server.hls(ctx, input.MediaCredentials, input.Token, "master.m3u8", "")
	})
	server.registerSubtitles()
	server.registerSubtitleFetch()
	huma.Register(server.api, huma.Operation{
		OperationID: "getRenditionFile", Method: http.MethodGet, Path: "/bytes/media/hls/{token}/{rendition}/{file}",
		Summary: "One file of a prepared playback's rendition, relayed from its node", Tags: []string{tagPlayback},
		Responses: responses,
	}, func(ctx context.Context, input *RenditionFileInput) (*huma.StreamResponse, error) {
		return server.hls(ctx, input.MediaCredentials, input.Token, input.Rendition+"/"+input.File, input.Range)
	})
}

// hlsName is the only shape a rendition's file may be asked for by: the master playlist, or one
// rendition's playlist, init segment or media segment. The node checks it again.
var hlsName = regexp.MustCompile(
	`^(master\.m3u8|(video|audio\d{1,3}(-boost)?)/(stream\.m3u8|init(_\d{1,3})?\.mp4|seg\d{5,9}\.m4s))$`)

func (server *Server) startPlayback(ctx context.Context, input *StartPlaybackInput) (*StartPlaybackOutput, error) {
	dependencies, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	request := playback.Request{
		FileID: input.Body.FileID, DeviceID: principal.DeviceID, Menu: menuOf(input.Body.PlayableFormats),
		Link: decide.Link(input.Body.Network), Prefer: decide.Prefer(input.Body.PreferredDelivery),
	}
	started, err := dependencies.Playback.Start(ctx, principal.UserID, sessionKey(input.Authorization), request)
	switch {
	case errors.Is(err, decide.ErrMenu):
		return nil, newError(http.StatusBadRequest, CodeInvalidRequest, err.Error())
	case errors.Is(err, playback.ErrNoFile):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such file to play")
	case err != nil:
		return nil, server.internal(ctx, "start a playback", err)
	}
	output := &StartPlaybackOutput{}
	output.Body.Delivery = string(started.Decision.Kind)
	output.Body.Profile = string(started.Decision.Profile)
	output.Body.NotDirectReasons = make([]string, 0, len(started.Decision.Reasons))
	for _, reason := range started.Decision.Reasons {
		output.Body.NotDirectReasons = append(output.Body.NotDirectReasons, string(reason))
	}
	output.Body.URL = Prefix + "/bytes/media/direct/" + started.Token
	if started.Decision.Kind == decide.HLS {
		output.Body.URL = Prefix + "/bytes/media/hls/" + started.Token + "/master.m3u8"
	}
	output.Body.IsPreparing = started.Preparing
	for _, audio := range started.Audio {
		output.Body.Audio = append(output.Body.Audio, AudioRenditionBody{
			SourceAudioIndex: audio.Track, Language: audio.Language, IsDialogueBoosted: audio.Boost,
			Levelling: levellingBody(audio.Levelling), CarriedGainDB: audio.CarriedGainDB,
		})
	}
	output.Body.Subtitles = subtitleTracks(started)
	output.Body.Prefs = filePrefsBody(started.Prefs)
	output.Body.ResumePositionMs = started.ResumePositionMs
	output.Body.MediaToken = started.Token
	output.Body.AudioOutput = AudioOutput(started.AudioOutput)
	output.Body.Levelling = levellingBody(started.Levelling)
	return output, nil
}

// direct relays a file's bytes from its node: the core never opens a media file (D193 §1).
func (server *Server) direct(ctx context.Context, method string, input *DirectInput) (*huma.StreamResponse, error) {
	authorization := input.authorization()
	dependencies, principal, err := server.signedIn(ctx, authorization)
	if err != nil {
		return nil, err
	}
	source, err := dependencies.Playback.Direct(ctx, input.Token, principal.UserID, sessionKey(authorization))
	switch {
	case errors.Is(err, playback.ErrNoToken), errors.Is(err, playback.ErrNoFile):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such playback; start it again")
	case err != nil:
		return nil, server.internal(ctx, "find a playback's bytes", err)
	}
	header := http.Header{}
	if input.Range != "" {
		header.Set("Range", input.Range)
	}
	if input.IfRange != "" {
		header.Set("If-Range", input.IfRange)
	}
	return server.relay(ctx, dependencies, relayed{method, source, header, "private, no-store"},
		func() error {
			// The node refuses a file that is gone or no longer the one named (D100).
			return newError(http.StatusNotFound, CodeFileUnavailable, "the file is no longer there as it was")
		})
}

// hls relays one file of a prepared playback's rendition. While the rendition is being made, a
// file the node has not written yet is 425 with a Retry-After, and the player asks again.
func (server *Server) hls(
	ctx context.Context, credentials MediaCredentials, token, name, byteRange string,
) (*huma.StreamResponse, error) {
	if !hlsName.MatchString(name) {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such rendition file")
	}
	authorization := credentials.authorization()
	dependencies, principal, err := server.signedIn(ctx, authorization)
	if err != nil {
		return nil, err
	}
	source, err := dependencies.Playback.HLS(ctx, token, name, principal.UserID, sessionKey(authorization))
	switch {
	case errors.Is(err, playback.ErrNoToken):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such playback; start it again")
	case err != nil:
		return nil, server.internal(ctx, "find a rendition's file", err)
	}
	header := http.Header{}
	if byteRange != "" {
		header.Set("Range", byteRange)
	}
	// A playlist grows while the rendition is made; a segment never changes once written.
	cache := "private, max-age=86400"
	if strings.HasSuffix(name, ".m3u8") && source.Readiness != playback.Complete {
		cache = "no-cache"
	}
	return server.relay(ctx, dependencies, relayed{http.MethodGet, source.Source, header, cache}, func() error {
		switch source.Readiness {
		case playback.Preparing:
			return tooEarly()
		case playback.Failed:
			return newError(http.StatusBadGateway, CodePrepareFailed, "this file could not be prepared")
		case playback.Complete:
			// A complete rendition whose playlist its node no longer has was evicted or lost: it is
			// prepared again, and the player waits as for a new one (D217 §5).
			if name == "master.m3u8" {
				if err := dependencies.Playback.Lost(ctx, source); err != nil {
					return server.internal(ctx, "prepare a lost rendition again", err)
				}
				return tooEarly()
			}
		}
		return newError(http.StatusNotFound, CodeFileUnavailable, "the rendition is no longer there")
	})
}

// tooEarly is 425 Too Early, with the Retry-After a player waits for.
func tooEarly() error {
	failure := newError(http.StatusTooEarly, CodePreparing, "being prepared; ask again in a moment")
	failure.headers = http.Header{"Retry-After": {"1"}}
	return failure
}

// relayed is one request a relay makes of a node's byte socket, and how its answer is cached.
type relayed struct {
	method       string
	source       playback.Source
	header       http.Header
	cacheControl string
}

// relay streams what a node's byte socket answers, or the refusal when it answers anything but
// the bytes or a range of them.
func (server *Server) relay(
	ctx context.Context, dependencies *Dependencies, request relayed, refusal func() error,
) (*huma.StreamResponse, error) {
	source := request.source
	//nolint:bodyclose // closed by the stream below, or on refusal
	answer, err := dependencies.NodeBytes.Do(ctx, source.NodeID, request.method, source.Target, request.header)
	if errors.Is(err, nodes.ErrNodeSilent) {
		failure := newError(http.StatusServiceUnavailable, CodeNodeSilent,
			"the node that holds this file is not answering")
		failure.ErrorBody.Error.Hop = HopNode
		return nil, failure
	}
	if err != nil {
		return nil, server.internal(ctx, "read from the node", err)
	}
	switch answer.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
	default:
		_ = answer.Body.Close() //nolint:errcheck // the answer is refused, unread
		return nil, refusal()
	}
	return &huma.StreamResponse{Body: func(stream huma.Context) {
		defer func() { _ = answer.Body.Close() }() //nolint:errcheck // read-only
		stream.SetHeader("Content-Type", source.ContentType)
		for _, name := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
			if value := answer.Header.Get(name); value != "" {
				stream.SetHeader(name, value)
			}
		}
		stream.SetHeader("Cache-Control", request.cacheControl)
		stream.SetStatus(answer.StatusCode)
		// A player that stops reading ends the copy; there is nothing to report to it.
		_, _ = io.Copy(stream.BodyWriter(), answer.Body) //nolint:errcheck // see above
	}}, nil
}

func menuOf(body MenuBody) decide.Menu {
	return decide.Menu{
		Video: names[decide.Video](body.VideoCodecs), Audio: names[decide.Audio](body.AudioCodecs),
		Subtitle: names[decide.Subtitle](body.SubtitleFormats), Containers: names[decide.Container](body.Containers),
	}
}

func names[T ~string](listed []string) []T {
	converted := make([]T, 0, len(listed))
	for _, name := range listed {
		converted = append(converted, T(name))
	}
	return converted
}

// sessionKey names the session a request came under without keeping its credential: a media
// token is bound to it, so signing out ends the playbacks it started (D217 §2).
func sessionKey(authorization string) string {
	sum := sha256.Sum256([]byte(bearerToken(authorization)))
	return hex.EncodeToString(sum[:])
}
