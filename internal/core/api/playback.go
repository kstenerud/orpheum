// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/providers"
	"github.com/kstenerud/orpheum/internal/core/providers/opensubtitles"
	"github.com/kstenerud/orpheum/internal/core/subtitles"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

const tagPlayback = "Playback"

// NodeBytes reads from a node's byte socket.
type NodeBytes interface {
	Do(ctx context.Context, nodeID int64, method, target string, header http.Header) (*http.Response, error)
}

// PlayableFormatsBody is what a client can play, one list per axis, most preferred first (D21, D217 §1).
type PlayableFormatsBody struct {
	VideoCodecs     []string `doc:"Must list h264-sdr" enum:"h264-sdr,hevc-sdr,hevc-hdr10,av1-sdr,av1-hdr10" json:"videoCodecs"` //nolint:lll // one struct tag
	AudioCodecs     []string `doc:"Must list aac-stereo" enum:"aac-stereo,aac-multi,ac3,eac3,opus,flac" json:"audioCodecs"`      //nolint:lll // one struct tag
	SubtitleFormats []string `enum:"ass,webvtt" json:"subtitleFormats"`
	Containers      []string `doc:"Containers played as they are" enum:"mp4,mkv,mpegts" json:"containers"`
}

// DeliveryKind is how a file plays, documented as one of decide.Deliveries: the decision's
// answer and a person's override name one set, from one list.
type DeliveryKind string

// Schema is a delivery's schema: a string, one of decide.Deliveries.
func (DeliveryKind) Schema(huma.Registry) *huma.Schema {
	deliveries := decide.Deliveries()
	names := make([]any, 0, len(deliveries))
	for _, delivery := range deliveries {
		names = append(names, string(delivery))
	}
	return &huma.Schema{Type: huma.TypeString, Enum: names}
}

// StartPlaybackInput asks to play a file.
type StartPlaybackInput struct {
	AuthorizationHeader
	Body struct {
		FileID            int64               `json:"fileId" minimum:"1"`
		PlayableFormats   PlayableFormatsBody `json:"playableFormats"`
		Network           string              `doc:"Where the client is" enum:"local,remote" json:"network"`
		PreferredDelivery DeliveryKind        `doc:"A person's override of the decision" json:"preferredDelivery,omitempty" required:"false"` //nolint:lll // one struct tag
	}
}

// StartPlaybackOutput is the decision (cut-a §6).
type StartPlaybackOutput struct {
	Body struct {
		Delivery DeliveryKind `json:"delivery"`
		URL      string       `doc:"Where the bytes are: the file, or a rendition's master playlist" json:"url"`
		Profile  string       `enum:"remux,h264-8m" json:"profile,omitempty"`
		// NotDirectReasons say why it is not played directly, to show on screen (D188).
		NotDirectReasons []string `enum:"container,video_codec,audio_codec,bandwidth,dialogue_boost,audio_track,user_choice" json:"notDirectReasons"` //nolint:lll // one struct tag
		// IsPreparing is whether the rendition is still being made; its playlist answers 425 until
		// the first segment exists.
		IsPreparing bool `json:"isPreparing"`
		// AudioRenditions are a rendition's audio renditions, in its master playlist's order.
		AudioRenditions []AudioRenditionBody `json:"audioRenditions,omitempty"`
		// Subtitles are the file's text subtitle tracks, each extracted or on its way; a track's
		// URLs answer 425 until it is (D217 §6).
		Subtitles []SubtitleTrackBody `json:"subtitles"`
		// AudioTracks are the file's own audio tracks, by their place among them, which a person
		// chooses among (D230 §5).
		AudioTracks []AudioTrackBody `json:"audioTracks"`
		// StartingAudioStreamID is the audio track the playback starts with: the person's choice,
		// or the one a player starts with; absent when the file has no sound.
		StartingAudioStreamID int64 `json:"startingAudioStreamId,omitempty"`
		// Preferences are the person's choices for this file, which the player starts with.
		Preferences PlaybackPreferencesBody `json:"preferences"`
		// ResumePositionMs is where the player starts, without asking (D188): where the person
		// left the file, or 0.
		ResumePositionMs int64 `json:"resumePositionMs"`
		// MediaToken names this playback in its reports; it grants nothing alone (D217 §2).
		MediaToken string `json:"mediaToken"`
		// AudioOutput is how the device asking makes sound, absent until it is asked: the player
		// asks before it plays, and asks for the decision again with the answer (D222).
		AudioOutput AudioOutputKind `json:"audioOutput,omitempty"`
		// Levelling brings the main sound to the level: the file's own when played directly, the
		// main track's stereo mix when prepared, where each audio rendition carries its own (D226).
		Levelling LevellingBody `json:"levelling"`
	}
}

// AudioTrackBody is one of a file's own audio tracks.
type AudioTrackBody struct {
	StreamID int64 `json:"streamId"`
	// AudioTrackIndex is its place among the file's audio tracks, as a prepared copy's audio
	// renditions name it.
	AudioTrackIndex int    `json:"audioTrackIndex"`
	Language        string `doc:"BCP 47; empty when nothing names one" json:"language"`
	ContainerTitle  string `json:"containerTitle,omitempty"`
	Kind            string `enum:"plain,commentary,described" json:"kind"`
}

// MasterInput asks for a prepared playback's master playlist.
type MasterInput struct {
	MediaCredentials
	Token string `doc:"The playback's media token" path:"token"`
}

// RenditionFileInput asks for one file of one of a prepared playback's renditions.
type RenditionFileInput struct {
	MediaCredentials
	Token         string `doc:"The playback's media token" path:"token"`
	RenditionName string `doc:"video, audio0, audio0-boost, ..." path:"renditionName"`
	FileName      string `doc:"stream.m3u8, init.mp4 or a segment" path:"fileName"`
	Range         string `header:"Range"`
}

// DirectInput asks for a direct playback's bytes.
type DirectInput struct {
	MediaCredentials
	Token   string `doc:"The playback's media token" path:"token"`
	Range   string `header:"Range"`
	IfRange string `header:"If-Range"`
}

func (server *Server) registerPlayback() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "startPlayback", Method: http.MethodPost, Path: "/playback",
		Summary: "Decide how a file plays on this client, and where its bytes are (cut-a §6)",
		Tags:    []string{tagPlayback},
	}, server.startPlayback)
	directResponses := map[string]*huma.Response{
		"200": {Description: "The file", Content: map[string]*huma.MediaType{"application/octet-stream": {}}},
		"206": {Description: "The range asked for"},
	}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getDirect", Method: http.MethodGet, Path: "/bytes/media/direct/{token}",
		Summary: "A file's own bytes, relayed from its node, honouring Range", Tags: []string{tagPlayback},
		Responses: directResponses,
	}, server.getDirect)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "headDirect", Method: http.MethodHead, Path: "/bytes/media/direct/{token}",
		Summary: "What a file's own bytes would be, without them", Tags: []string{tagPlayback},
		Responses: directResponses,
	}, server.headDirect)
	responses := map[string]*huma.Response{
		"200": {Description: "The file", Content: map[string]*huma.MediaType{
			"application/vnd.apple.mpegurl": {}, "video/iso.segment": {}, "video/mp4": {},
		}},
		"425": {Description: "Not written yet: the rendition is being prepared; ask again after Retry-After"},
	}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getMasterPlaylist", Method: http.MethodGet, Path: "/bytes/media/hls/{token}/master.m3u8",
		Summary: "A prepared playback's master playlist, relayed from its node", Tags: []string{tagPlayback},
		Responses: responses,
	}, server.getMasterPlaylist)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getFileAvailability", Method: http.MethodGet, Path: "/files/{id}/availability",
		Summary: "Whether a file can be read now, and if not why: what a player whose fetch failed asks (D241 §7)",
		Tags:    []string{tagPlayback},
	}, server.getFileAvailability)
	server.registerSubtitles()
	server.registerSubtitleFetch()
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getRenditionFile", Method: http.MethodGet,
		Path:    "/bytes/media/hls/{token}/{renditionName}/{fileName}",
		Summary: "One file of a prepared playback's rendition, relayed from its node", Tags: []string{tagPlayback},
		Responses: responses,
	}, server.getRenditionFile)
}

// hlsFilePattern is the only shape a rendition's file may be asked for by: the master playlist, or
// one rendition's playlist, init segment or media segment. The node checks it again.
var hlsFilePattern = regexp.MustCompile(
	`^(master\.m3u8|(video|audio\d{1,3}(-boost)?)/(stream\.m3u8|init(_\d{1,3})?\.mp4|seg\d{5,9}\.m4s))$`)

// FileAvailabilityOutput is whether a file can be read now.
type FileAvailabilityOutput struct {
	Body struct {
		// Unavailable says why it cannot; absent when it can, and a failed fetch was a passing one.
		Unavailable *UnavailableBody `json:"unavailable,omitempty"`
	}
}

// getFileAvailability answers whether a file can be read now: a player whose fetch failed mid-play
// asks, and tells the viewer why, or tries again (D241 §7).
func (server *Server) getFileAvailability(ctx context.Context, input *ItemInput) (*FileAvailabilityOutput, error) {
	dependencies, _, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	unreadable, err := dependencies.Playback.Unreadable(ctx, input.ID)
	if err != nil {
		return nil, server.playbackError(ctx, "ask whether a file can be read", err)
	}
	output := &FileAvailabilityOutput{}
	output.Body.Unavailable = unavailableBody(unreadable, server.nodeNamer(ctx, dependencies))
	return output, nil
}

func (server *Server) startPlayback(ctx context.Context, input *StartPlaybackInput) (*StartPlaybackOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	request := playback.Request{
		FileID: input.Body.FileID, DeviceID: principal.DeviceID,
		PlayableFormats:   playableFormatsOf(input.Body.PlayableFormats),
		Network:           decide.Network(input.Body.Network),
		PreferredDelivery: decide.Delivery(input.Body.PreferredDelivery),
	}
	started, err := dependencies.Playback.Start(ctx, principal.UserID, principal.SessionKey, request)
	if unreadable, isUnreadable := errors.AsType[*library.UnreadableError](err); isUnreadable {
		return nil, mediaUnreachable(unreadable, server.nodeNamer(ctx, dependencies))
	}
	if err != nil {
		return nil, server.playbackError(ctx, "start a playback", err)
	}
	output := &StartPlaybackOutput{}
	output.Body.Delivery = DeliveryKind(started.Decision.Delivery)
	output.Body.Profile = string(started.Decision.Profile)
	output.Body.NotDirectReasons = make([]string, 0, len(started.Decision.NotDirectReasons))
	for _, reason := range started.Decision.NotDirectReasons {
		output.Body.NotDirectReasons = append(output.Body.NotDirectReasons, string(reason))
	}
	output.Body.URL = Prefix + "/bytes/media/direct/" + started.Token
	if started.Decision.Delivery == decide.HLS {
		output.Body.URL = Prefix + "/bytes/media/hls/" + started.Token + "/master.m3u8"
	}
	output.Body.IsPreparing = started.IsPreparing
	for _, audio := range started.AudioRenditions {
		output.Body.AudioRenditions = append(output.Body.AudioRenditions, AudioRenditionBody{
			AudioTrackIndex: audio.AudioTrackIndex, Language: audio.Language, IsDialogueBoosted: audio.IsBoosted,
			Levelling: levellingBody(audio.Levelling), CarriedGainDB: audio.CarriedGainDB,
		})
	}
	output.Body.Subtitles = subtitleTracks(started)
	output.Body.AudioTracks = make([]AudioTrackBody, 0, len(started.AudioTracks))
	for _, track := range started.AudioTracks {
		output.Body.AudioTracks = append(output.Body.AudioTracks, AudioTrackBody{
			StreamID: track.StreamID, AudioTrackIndex: track.AudioTrackIndex, Language: track.Language,
			ContainerTitle: track.ContainerTitle, Kind: string(track.Kind),
		})
	}
	if started.StartingAudio != nil {
		output.Body.StartingAudioStreamID = started.StartingAudio.StreamID
	}
	output.Body.Preferences = playbackPreferencesBody(started.Preferences)
	output.Body.ResumePositionMs = started.ResumePositionMs
	output.Body.MediaToken = started.Token
	output.Body.AudioOutput = AudioOutputKind(started.AudioOutput)
	output.Body.Levelling = levellingBody(started.Levelling)
	return output, nil
}

// relayDirect relays a file's bytes from its node: the core never opens a media file (D193 §1).
func (server *Server) relayDirect(
	ctx context.Context, method string, input *DirectInput,
) (*huma.StreamResponse, error) {
	dependencies, principal, err := server.requireSignedInForMedia(ctx, input.MediaCredentials)
	if err != nil {
		return nil, err
	}
	source, err := dependencies.Playback.DirectSource(ctx, input.Token, principal.UserID, principal.SessionKey)
	if err != nil {
		return nil, server.playbackError(ctx, "find a playback's bytes", err)
	}
	header := http.Header{}
	if input.Range != "" {
		header.Set("Range", input.Range)
	}
	if input.IfRange != "" {
		header.Set("If-Range", input.IfRange)
	}
	return server.relay(ctx, dependencies, relayRequest{method, source, header, cacheNeverPrivately},
		func() error {
			// The node refuses a file that is gone or no longer the one named (D100).
			return newError(http.StatusNotFound, CodeFileUnavailable, "the file is no longer there as it was")
		})
}

// relayHLSFile relays one file of a prepared playback's rendition. While the rendition is being
// made, a file the node has not written yet is 425 with a Retry-After, and the player asks again.
func (server *Server) relayHLSFile(
	ctx context.Context, credentials MediaCredentials, token, renditionPath, byteRange string,
) (*huma.StreamResponse, error) {
	if !hlsFilePattern.MatchString(renditionPath) {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such rendition file")
	}
	dependencies, principal, err := server.requireSignedInForMedia(ctx, credentials)
	if err != nil {
		return nil, err
	}
	source, err := dependencies.Playback.HLSSourceOf(
		ctx, token, renditionPath, principal.UserID, principal.SessionKey)
	if err != nil {
		return nil, server.playbackError(ctx, "find a rendition's file", err)
	}
	header := http.Header{}
	if byteRange != "" {
		header.Set("Range", byteRange)
	}
	// A playlist grows while the rendition is made; a segment never changes once written.
	cacheControl := cacheForADay
	if strings.HasSuffix(renditionPath, ".m3u8") && source.Readiness != jobs.ReadinessComplete {
		cacheControl = cacheRevalidate
	}
	request := relayRequest{http.MethodGet, source.Source, header, cacheControl}
	return server.relay(ctx, dependencies, request, func() error {
		switch source.Readiness {
		case jobs.ReadinessPreparing:
			return tooEarly()
		case jobs.ReadinessFailed:
			return newError(http.StatusBadGateway, CodePrepareFailed, "this file could not be prepared")
		case jobs.ReadinessComplete:
			// A complete rendition whose playlist its node no longer has was evicted or lost: it is
			// prepared again, and the player waits as for a new one (D217 §5).
			if renditionPath == "master.m3u8" {
				if err := dependencies.Playback.PrepareAgain(ctx, source); err != nil {
					return server.playbackError(ctx, "prepare a lost rendition again", err)
				}
				return tooEarly()
			}
		}
		return newError(http.StatusNotFound, CodeFileUnavailable, "the rendition is no longer there")
	})
}

// playbackError maps the playback package's refusals, and a subtitle provider's that reach the API
// through it, to API errors. Every handler that calls playback answers through it, so a refusal
// always leaves with one code and one message. A subtitle provider's refusal says why in
// detail.reason, in OpenSubtitles' terms, with the provider as the failing leg (D224).
func (server *Server) playbackError(ctx context.Context, action string, err error) error {
	spent, isSpent := errors.AsType[*providers.QuotaSpentError](err)
	limited, isLimited := errors.AsType[*outbound.RateLimitedError](err)
	switch {
	case errors.Is(err, decide.ErrBadPlayableFormats):
		return invalidRequestAt(http.StatusBadRequest, "body.playableFormats", err.Error())
	case errors.Is(err, playback.ErrBadReport):
		return invalidRequestAt(http.StatusBadRequest, "body", err.Error())
	case errors.Is(err, library.ErrNoFile):
		return newError(http.StatusNotFound, CodeNotFound, "no such file to play")
	case errors.Is(err, playback.ErrNoToken):
		return newError(http.StatusNotFound, CodeNotFound, "no such playback; start it again")
	case errors.Is(err, subtitles.ErrNoSubtitle):
		return newError(http.StatusNotFound, CodeNotFound, "no such subtitle")
	case errors.Is(err, playback.ErrNoItem):
		return newError(http.StatusNotFound, CodeNotFound, "no such item")
	case errors.Is(err, subtitles.ErrNothingToSearchBy):
		return newError(http.StatusConflict, CodeConflict,
			"this file is not identified yet, and has not been scanned since it could be hashed for OpenSubtitles")
	case errors.Is(err, subtitles.ErrNoSubtitler):
		return providerRefusal(ReasonNotConfigured,
			"this server fetches no subtitles")
	case errors.Is(err, opensubtitles.ErrNoLogin):
		return providerRefusal(ReasonNoLogin,
			"no OpenSubtitles login is set; an admin can set it in Settings")
	case errors.Is(err, opensubtitles.ErrLoginRefused):
		return providerRefusal(ReasonLoginRefused,
			"OpenSubtitles refused the login; an admin can change it in Settings")
	case errors.Is(err, outbound.ErrUnavailable):
		return server.providerNotAnswering(ctx, jobs.DestinationOpenSubtitles,
			"OpenSubtitles is not answering; try again later")
	case isSpent:
		failure := newDetailedError(http.StatusTooManyRequests,
			"the OpenSubtitles account's downloads for today are spent",
			QuotaExhaustedDetailBody{DownloadsResetAt: spent.ResetAt.UnixMilli()})
		failure.ErrorBody.Error.FailedHop = HopProvider
		return failure
	case isLimited:
		return providerRateLimited(limited)
	}
	if refusal := fetchRefusal(err); refusal != nil {
		return refusal
	}
	return server.logInternalError(ctx, action, err)
}

// fetchRefusal is a subtitle fetch's own refusal, from the conversion its node makes before the
// subtitle is kept (D224 §5), or nil for any other error.
func fetchRefusal(err error) error {
	switch {
	case errors.Is(err, subtitles.ErrUnreadable):
		return providerRefusal(ReasonUnreadable,
			"the subtitle OpenSubtitles sent could not be read, so it was not kept; choose another")
	case errors.Is(err, nodes.ErrNodeSilent):
		failure := newError(http.StatusServiceUnavailable, CodeNodeSilent,
			"the node that holds this file is not answering, so nothing was downloaded or kept")
		failure.ErrorBody.Error.FailedHop = HopNode
		return failure
	}
	return nil
}

// tooEarly is 425 Too Early, with the Retry-After a player waits for.
func tooEarly() error {
	failure := newError(http.StatusTooEarly, CodePreparing, "being prepared; ask again in a moment")
	failure.headers = http.Header{"Retry-After": {"1"}}
	return failure
}

// relayRequest is one request a relay makes of a node's byte socket, and how its answer is cached.
type relayRequest struct {
	method       string
	source       playback.Source
	header       http.Header
	cacheControl string
}

// relay streams what a node's byte socket answers, or the refusal when it answers anything but
// the bytes or a range of them.
func (server *Server) relay(
	ctx context.Context, dependencies *Dependencies, request relayRequest, refuse func() error,
) (*huma.StreamResponse, error) {
	source := request.source
	//nolint:bodyclose // closed by the stream below, or on refusal
	answer, err := dependencies.NodeBytes.Do(ctx, source.NodeID, request.method, source.Target, request.header)
	if errors.Is(err, nodes.ErrNodeSilent) {
		failure := newError(http.StatusServiceUnavailable, CodeNodeSilent,
			"the node that holds this file is not answering")
		failure.ErrorBody.Error.FailedHop = HopNode
		return nil, failure
	}
	if err != nil {
		return nil, server.logInternalError(ctx, "read from the node", err)
	}
	switch answer.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
	default:
		_ = answer.Body.Close() //nolint:errcheck // the answer is refused, unread
		return nil, refuse()
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

func playableFormatsOf(body PlayableFormatsBody) decide.PlayableFormats {
	return decide.PlayableFormats{
		VideoCodecs:     convertNames[decide.Video](body.VideoCodecs),
		AudioCodecs:     convertNames[decide.Audio](body.AudioCodecs),
		SubtitleFormats: convertNames[decide.Subtitle](body.SubtitleFormats),
		Containers:      convertNames[decide.Container](body.Containers),
	}
}

func convertNames[T ~string](listed []string) []T {
	converted := make([]T, 0, len(listed))
	for _, name := range listed {
		converted = append(converted, T(name))
	}
	return converted
}

// getDirect answers GET /bytes/media/direct/{token}: a file's own bytes, relayed from its node.
func (server *Server) getDirect(ctx context.Context, input *DirectInput) (*huma.StreamResponse, error) {
	return server.relayDirect(ctx, http.MethodGet, input)
}

// headDirect answers HEAD /bytes/media/direct/{token}: the headers a GET would send.
func (server *Server) headDirect(ctx context.Context, input *DirectInput) (*huma.StreamResponse, error) {
	return server.relayDirect(ctx, http.MethodHead, input)
}

// getMasterPlaylist answers GET /bytes/media/hls/{token}/master.m3u8, relayed from its node.
func (server *Server) getMasterPlaylist(ctx context.Context, input *MasterInput) (*huma.StreamResponse, error) {
	return server.relayHLSFile(ctx, input.MediaCredentials, input.Token, "master.m3u8", "")
}

// getRenditionFile answers GET /bytes/media/hls/{token}/{renditionName}/{fileName}: one file of a
// prepared playback's rendition, relayed from its node.
func (server *Server) getRenditionFile(ctx context.Context, input *RenditionFileInput) (*huma.StreamResponse, error) {
	return server.relayHLSFile(ctx, input.MediaCredentials, input.Token,
		input.RenditionName+"/"+input.FileName, input.Range)
}
