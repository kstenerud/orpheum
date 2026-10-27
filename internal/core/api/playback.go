// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

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
		PreferredDelivery DeliveryKind        `doc:"A person's override of the decision" json:"preferredDelivery,omitempty" required:"false"` //nolint:lll // one struct tag
		// Cause and PageStarts are for the log alone: why the page asked, and how many playbacks it
		// has asked for since it loaded, so a playback started again by itself says what started it
		// (staging, 2026-10-14, where a phone's playback started over and nothing said why).
		Cause      string `doc:"Why the page asked for this playback, for the log" enum:"filmPage,playPage,warmUpStale,warmUpOffLadder,warmUpFailed,subtitleFetched,audioChosen,originalChosen,boostChosen,playbackGone,ownPictureGivenUp,mediaBack,castOver" json:"cause,omitempty" required:"false"` //nolint:lll // one struct tag
		PageStarts int    `doc:"How many playbacks the page has asked for since it loaded, this one included" json:"pageStarts,omitempty" minimum:"0" required:"false"`                                                                                                                                //nolint:lll // one struct tag
		// HasGivenUpOwnPicture is the page's word that its player gave the film's own picture up as
		// unplayable on this device (D266), so the playback starts at its twin (D323 §2); the core
		// keeps it before it decides, whether or not the player's report reached it first.
		HasGivenUpOwnPicture bool `doc:"Whether the player gave the film's own picture up as unplayable here" json:"hasGivenUpOwnPicture,omitempty" required:"false"` //nolint:lll // one struct tag
		// StartsAtMs is for the log alone: where the page starts the playback when it asks again from
		// where its video stood, which it does in place of the resume position answered, so a playback
		// asked for again says where it went on from. A film whose audio track was changed three
		// minutes in started again from its beginning, and the log said only resumePositionMs=0
		// (staging, 2026-10-26).
		StartsAtMs *int64 `doc:"Where the page starts this playback, in ms, when it asks again from where its video stood, for the log; absent where it starts at the resume position" json:"startsAtMs,omitempty" minimum:"0" required:"false"` //nolint:lll // one struct tag
	}
	ClientFacts
}

// StartPlaybackOutput is the decision (cut-a §6).
type StartPlaybackOutput struct {
	Body StartPlaybackOutputBody
}

// StartPlaybackOutputBody is the decision, and what the player starts with.
type StartPlaybackOutputBody struct {
	Delivery DeliveryKind `json:"delivery"`
	URL      string       `doc:"Where the bytes are: the file, or its ladder's master playlist" json:"url"`
	// Profile is, for a film converted, whether its own picture plays where it can (remux) or the
	// player cannot decode it (h264); absent for one played as it is.
	Profile string `enum:"remux,h264" json:"profile,omitempty"`
	// NotDirectReasons say why it is not played directly, to show on screen (D188).
	NotDirectReasons []string `enum:"container,video_codec,audio_codec,bandwidth,dialogue_boost,audio_track,user_choice" json:"notDirectReasons"` //nolint:lll // one struct tag
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
	// SoundOffsetMs is how far the device asking moves its sound from its picture, later when
	// positive and earlier when negative: its person sets it there (changeMyDevice); 0 for none.
	SoundOffsetMs int64 `json:"soundOffsetMs" maximum:"2000" minimum:"-2000"`
	// Levelling brings the main sound to the level, as the file plays it directly; a film on its
	// ladder carries its sound's own gain (LadderBody, D226).
	Levelling LevellingBody `json:"levelling"`
	// FileBitRateKbps is the file's own average bit rate, by which a player playing it as it is
	// measures its link from how fast its buffer grows (D247 §1); 0 when it is not known.
	FileBitRateKbps int64 `json:"fileBitRateKbps"`
	// TitleName is the name of the title the file is, which the player shows above the picture;
	// absent for a file filed as no title. Never the file's path: a viewer sees it too (D246 §3).
	TitleName string `json:"titleName,omitempty"`
	// OnLadder is the film's ladder and where on it the playback starts, its player moving along it
	// by itself (D260); absent for a film not cut yet, or one that cannot be, which plays as Delivery
	// says.
	OnLadder *LadderBody `json:"onLadder,omitempty"`
	// Tracks are what the playback may play and what the file holds, each named original or
	// converted, and from what.
	Tracks TracksBody `json:"tracks"`
}

// AudioTrackBody is one of a file's own audio tracks.
type AudioTrackBody struct {
	StreamID int64 `json:"streamId"`
	// AudioTrackIndex is its place among the file's audio tracks, as the sounds of its chunks
	// name it.
	AudioTrackIndex int    `json:"audioTrackIndex"`
	Language        string `doc:"BCP 47; empty when nothing names one" json:"language"`
	ContainerTitle  string `json:"containerTitle,omitempty"`
	Kind            string `enum:"plain,commentary,described" json:"kind"`
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
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getFileAvailability", Method: http.MethodGet, Path: "/files/{id}/availability",
		Summary: "Whether a file can be read now, and if not why: what a player whose fetch failed asks (D241 §7)",
		Tags:    []string{tagPlayback},
	}, server.getFileAvailability)
	server.registerSubtitles()
	server.registerSubtitleFetch()
	server.registerChunks()
	server.registerUpNext()
}

// FileAvailabilityOutput is whether a file can be read now.
type FileAvailabilityOutput struct {
	Body struct {
		// Unavailable says why it cannot; absent when it can, and a failed fetch was a passing one.
		Unavailable *UnavailableBody `json:"unavailable,omitempty"`
	}
}

// FileAvailabilityInput asks whether a file can be read now, held while it cannot for up to
// WaitSeconds.
type FileAvailabilityInput struct {
	ItemInput
	WaitSeconds int `doc:"Hold the answer up to this long while the file cannot be read, and answer as soon as it can (D327 §3); 0 answers at once" maximum:"30" minimum:"0" query:"waitSeconds"` //nolint:lll // one struct tag
}

// getFileAvailability answers whether a file can be read now: a player whose fetch failed mid-play
// asks, and tells the viewer why, or tries again (D241 §7). Asked to wait, it holds the answer while
// the file's root is unavailable, its node asked whether it is back, and answers once it is.
func (server *Server) getFileAvailability(
	ctx context.Context, input *FileAvailabilityInput,
) (*FileAvailabilityOutput, error) {
	dependencies, _, err := server.requireSignedInReachingFile(ctx, input.Authorization, input.ID)
	if err != nil {
		return nil, err
	}
	unreadable, err := dependencies.Playback.AwaitReadable(ctx, input.ID,
		time.Duration(input.WaitSeconds)*time.Second)
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
		PreferredDelivery: decide.Delivery(input.Body.PreferredDelivery), ClientAddress: input.clientAddress,
		HasGivenUpOwnPicture: input.Body.HasGivenUpOwnPicture,
	}
	started, err := dependencies.Playback.Start(ctx, playback.HolderOf(principal), request)
	who := []any{
		logKeyUserID, principal.UserID, logKeyDeviceID, principal.DeviceID, "fileId", input.Body.FileID,
		logKeyUserAgent, input.userAgent, "over", input.over, "host", input.host, "clientAddress", input.clientAddress,
		"cause", input.Body.Cause, "pageStarts", input.Body.PageStarts,
	}
	if input.Body.StartsAtMs != nil {
		who = append(who, "pageStartsAtMs", *input.Body.StartsAtMs)
	}
	if unreadable, isUnreadable := errors.AsType[*library.UnreadableError](err); isUnreadable {
		server.logPlaybackRefused(ctx, who, err)
		return nil, mediaUnreachable(unreadable, server.nodeNamer(ctx, dependencies))
	}
	if err != nil {
		server.logPlaybackRefused(ctx, who, err)
		return nil, server.playbackError(ctx, "start a playback", err)
	}
	server.logPlaybackStarted(ctx, who, started)
	return server.namedOutput(ctx, dependencies, input.Body.FileID, started)
}

// namedOutput is a playback started, as the API says it, with the name of the title it plays.
func (server *Server) namedOutput(
	ctx context.Context, dependencies *Dependencies, fileID int64, started playback.Started,
) (*StartPlaybackOutput, error) {
	work, err := dependencies.Browser.WorkOfFile(ctx, fileID)
	if err != nil {
		return nil, server.libraryError(ctx, "read what a playback plays", err)
	}
	output := startedOutput(started)
	output.Body.TitleName = work.Name
	return output, nil
}

// logPlaybackStarted says a playback started: whose, which file, and how it is delivered and why,
// so a film that will not play says what was chosen for it (staging, 2026-10-06). The playback's
// token is a credential to its bytes, so it is never logged.
func (server *Server) logPlaybackStarted(ctx context.Context, who []any, started playback.Started) {
	reasons := make([]string, 0, len(started.Decision.NotDirectReasons))
	for _, each := range started.Decision.NotDirectReasons {
		reasons = append(reasons, string(each))
	}
	server.logger.InfoContext(ctx, "a playback started", append(who,
		"delivery", string(started.Decision.Delivery), "profile", string(started.Decision.Profile),
		"notDirectReasons", reasons,
		"resumePositionMs", started.ResumePositionMs, "isDialogueBoostOn", started.Preferences.IsDialogueBoostOn,
		"startsOnLadderKbps", startsOnLadderKbps(started), "startsAtSteadiness", started.Steadiness,
		"notOnLadderBecause", started.NotOnLadderBecause)...)
}

// startsOnLadderKbps is the rate of the picture on its film's ladder a playback starts at, 0 when it
// plays as its decision says (D260).
func startsOnLadderKbps(started playback.Started) int {
	if started.OnLadder == nil {
		return 0
	}
	return started.OnLadder.Quality.Kbps
}

// logPlaybackRefused says a playback was refused, with why.
func (server *Server) logPlaybackRefused(ctx context.Context, who []any, err error) {
	server.logger.InfoContext(ctx, "a playback was refused", append(who, "err", err)...)
}

// startedOutput is a playback started, as the API says it.
func startedOutput(started playback.Started) *StartPlaybackOutput {
	output := &StartPlaybackOutput{}
	output.Body.Delivery = DeliveryKind(started.Decision.Delivery)
	output.Body.Profile = string(started.Decision.Profile)
	output.Body.NotDirectReasons = make([]string, 0, len(started.Decision.NotDirectReasons))
	for _, reason := range started.Decision.NotDirectReasons {
		output.Body.NotDirectReasons = append(output.Body.NotDirectReasons, string(reason))
	}
	output.Body.URL = Prefix + "/bytes/media/direct/" + started.Token
	if started.OnLadder != nil {
		// Its player waits for the chunks it asks for itself: a start does not say whether they are made.
		output.Body.OnLadder = ladderBody(started.Token, *started.OnLadder, false, started.Steadiness)
		// A film converted plays from its ladder, the one way to convert it (D344 §1).
		if started.Decision.Delivery == decide.HLS {
			output.Body.URL = output.Body.OnLadder.URL
		}
	}
	output.Body.FileBitRateKbps = started.FileBitRateBps / 1000
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
	output.Body.SoundOffsetMs = started.SoundOffsetMs
	output.Body.Levelling = levellingBody(started.Levelling)
	output.Body.Tracks = tracksBody(started.Tracks)
	return output
}

// relayDirect relays a file's bytes from its node: the core never opens a media file (D193 §1).
func (server *Server) relayDirect(
	ctx context.Context, method string, input *DirectInput,
) (*huma.StreamResponse, error) {
	dependencies, holder, err := server.mediaHolder(ctx, input.MediaCredentials)
	if err != nil {
		return nil, err
	}
	source, err := dependencies.Playback.DirectSource(ctx, input.Token, holder)
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

// playbackError maps the playback package's refusals, and a subtitle provider's that reach the API
// through it, to API errors. Every handler that calls playback answers through it, so a refusal
// always leaves with one code and one message. A subtitle provider's refusal says why in
// detail.reason, in OpenSubtitles' terms, with the provider as the failing leg (D224).
func (server *Server) playbackError(ctx context.Context, action string, err error) error {
	if refusal := playbackRefusal(err); refusal != nil {
		return refusal
	}
	spent, isSpent := errors.AsType[*providers.QuotaSpentError](err)
	limited, isLimited := errors.AsType[*outbound.RateLimitedError](err)
	switch {
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

// playbackRefusal is the playback package's own refusal of a request, or nil for any other error.
func playbackRefusal(err error) error {
	switch {
	case errors.Is(err, decide.ErrBadPlayableFormats):
		return invalidRequestAt(http.StatusBadRequest, "body.playableFormats", err.Error())
	case errors.Is(err, playback.ErrBadReport):
		return invalidRequestAt(http.StatusBadRequest, "body", err.Error())
	case errors.Is(err, library.ErrNoFile):
		return newError(http.StatusNotFound, CodeNotFound, "no such file to play")
	case errors.Is(err, playback.ErrCannotConvert):
		return newError(http.StatusUnprocessableEntity, CodeCannotConvert,
			"this device cannot play this file as it is, and it cannot be converted: it has no picture, or "+
				"neither a known length nor keyframes to cut it by")
	case errors.Is(err, playback.ErrNoToken):
		return newError(http.StatusNotFound, CodePlaybackGone,
			"no such playback: the server restarted since it began, or it ended; start it again")
	case errors.Is(err, subtitles.ErrNoSubtitle):
		return newError(http.StatusNotFound, CodeNotFound, "no such subtitle")
	case errors.Is(err, playback.ErrNoItem), errors.Is(err, library.ErrNoItem): // one not reached too
		return newError(http.StatusNotFound, CodeNotFound, "no such item")
	}
	return nil
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

// nodeReadError is a read from a node's byte socket that failed. One whose client went away first,
// as a player abandons a segment when it seeks, failed only in the asking: it is no silent node,
// which a 503 and the log said it was (the owner's log, 2026-10-05). A node that did not answer is
// logged with why, which the 503's log line does not say.
func (server *Server) nodeReadError(ctx context.Context, err error) error {
	if ctx.Err() == nil && errors.Is(err, nodes.ErrNodeSilent) {
		server.logger.WarnContext(ctx, "a node did not answer a read of its bytes", "err", err)
		failure := newError(http.StatusServiceUnavailable, CodeNodeSilent,
			"the node that holds this file is not answering")
		failure.ErrorBody.Error.FailedHop = HopNode
		return failure
	}
	return server.logInternalError(ctx, "read from the node", err)
}

// relay streams what a node's byte socket answers, or the refusal when it answers anything but
// the bytes or a range of them.
func (server *Server) relay(
	ctx context.Context, dependencies *Dependencies, request relayRequest, refuse func() error,
) (*huma.StreamResponse, error) {
	source := request.source
	//nolint:bodyclose // closed by the stream below, or on refusal
	answer, err := dependencies.NodeBytes.Do(ctx, source.NodeID, request.method, source.Target, request.header)
	if stalled, isStalled := errors.AsType[*nodes.StalledRootError](err); isStalled {
		// Its root's disk does not answer: the root is unavailable now, and the player told so (D241 §7).
		return nil, mediaUnreachable(&library.UnreadableError{Unreadable: library.Unreadable{
			Reason: library.RootMissing, NodeID: stalled.NodeID, Path: stalled.RootPath, Since: stalled.Since,
		}}, server.nodeNamer(ctx, dependencies))
	}
	if err != nil {
		return nil, server.nodeReadError(ctx, err)
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
