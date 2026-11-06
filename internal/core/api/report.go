// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/playback"
)

// ReportInput is a player's report on one playback (cut-a §5, D165). ReportSeq numbers the playback's
// reports from 1, and one sent again is taken once. Every time is the device's monotonic clock in
// milliseconds, which the server anchors to its own on arrival: SentMonoMs when the report was
// sent, PositionReadMonoMs when PositionMs was read. Events are the changes of play state since the
// last report, and Spans the runs of watching they closed.
type ReportInput struct {
	AuthorizationHeader
	Token string `doc:"The playback's media token" path:"token"`
	Body  struct {
		ReportSeq          int64       `json:"reportSeq"            minimum:"1"`
		SentMonoMs         int64       `json:"sentMonoMs"     minimum:"0"`
		PositionMs         int64       `json:"positionMs"     minimum:"0"`
		PositionReadMonoMs int64       `json:"positionReadMonoMs" minimum:"0"`
		Events             []EventBody `json:"events"`
		Spans              []SpanBody  `json:"spans"`
		// Stalls are where the player stalled since its last report (D247 §2), which the play log
		// does not keep; LinkKbps is its link's speed as it measured it, 0 when it has not.
		Stalls   []StallBody `json:"stalls,omitempty" required:"false"`
		LinkKbps int64       `json:"linkKbps,omitempty" minimum:"0" required:"false"`
		// LinkSpread is the spread of the natural logs of the samples LinkKbps rests on, which the
		// core keeps beside it for the player's next playback on this network
		// (bandwidth-sampling-2026-10-29.md P2); absent from a player that says none.
		LinkSpread float64 `json:"linkSpread,omitempty" maximum:"10" minimum:"0" required:"false"`
		// Steadiness is the steadiness level a chunked player stands at (D352 §3), which the core
		// keeps beside the speed for its next playback on this network (D352 Q2); absent from a
		// player with none.
		Steadiness *int `json:"steadiness,omitempty" maximum:"3" minimum:"0" required:"false"`
		// Startup is how long the playback took to start, sent once, with the report after its
		// first frame: what makes a slow start show in the log.
		Startup *StartupBody `json:"startup,omitempty" required:"false"`
		// Unplayables are the qualities the player gave up as ones its device cannot play since its
		// last report, each sent once (D266).
		Unplayables []UnplayableBody `json:"unplayables,omitempty" required:"false"`
		// Seeks are how long each of the person's seeks since the last report took to play again.
		Seeks []SeekBody `json:"seeks,omitempty" required:"false"`
		// Standstills are where the player, asked to play, kept its position without saying it
		// waited, each told once.
		Standstills []StandstillBody `json:"standstills,omitempty" required:"false"`
		// PathMoves are the paths to the core the page moved to since its last report (PathMoveBody).
		PathMoves []PathMoveBody `json:"pathMoves,omitempty" maxItems:"20" required:"false"`
		// TotalFrames and DroppedFrames are the frames the video showed or dropped, and those it
		// dropped, since the player's last report, as its browser counts them
		// (getVideoPlaybackQuality); absent where the browser counts none. A stutter on a Fire TV
		// showed in no log (the owner, 2026-10-28).
		TotalFrames   *int64 `json:"totalFrames,omitempty"   minimum:"0" required:"false"`
		DroppedFrames *int64 `json:"droppedFrames,omitempty" minimum:"0" required:"false"`
		// LinkNotes are the chunked player's notes on its link since its last report, oldest first,
		// which the core logs: its prior, each sample with the time the estimate before it expected
		// and its bound's, so the estimate's calibration can be read from the log, and each chunk given
		// up or stuck, with the transport and the WebRTC connection that carried it and how long since
		// that connection last delivered bytes (bandwidth-sampling-2026-10-29.md P6; the owner,
		// 2026-10-29: the logs are to show whether stalls are per-connection).
		LinkNotes []string `json:"linkNotes,omitempty" maxItems:"20" required:"false"`
	}
	ClientFacts
}

// SeekBody is how long one of the person's seeks took to play again, as the player measured it:
// from the seek to playing, or, for a paused video, to being able to play. A jump that felt slow
// on staging could only be guessed at, the server seeing none of a player's own seeks (the owner,
// 2026-10-12).
type SeekBody struct {
	FromPositionMs int64 `json:"fromPositionMs" minimum:"0"`
	ToPositionMs   int64 `json:"toPositionMs"   minimum:"0"`
	ReadyAfterMs   int64 `json:"readyAfterMs"   minimum:"0"`
	// IsPaused is whether the video was paused, so ReadyAfterMs is to being able to play.
	IsPaused bool `json:"isPaused,omitempty" required:"false"`
	// PictureAfterMs and SoundAfterMs are when the first chunk of picture and of sound arrived after
	// the seek, absent when none had: a slow seek waited on one of them, or on neither.
	PictureAfterMs *int64 `json:"pictureAfterMs,omitempty" minimum:"0" required:"false"`
	SoundAfterMs   *int64 `json:"soundAfterMs,omitempty"   minimum:"0" required:"false"`
	// Steps are the player's last steps when the seek was ready, its quality's choices among them,
	// each with why: a player that dropped to 360p after a jump on a 77 Mb/s link said nothing of
	// why (the owner's Firefox on nas.example, 2026-10-20).
	Steps string `json:"steps,omitempty" maxLength:"400" required:"false"`
	// LinkSamples are the link's last samples when the seek was ready (LinkSamplesBody).
	LinkSamples string `json:"linkSamples,omitempty" maxLength:"300" required:"false"`
	StartGateBody
}

// StartGateBody is how much film a player had loaded ahead as it played, and, where it waited for
// it before it played, the film loaded ahead it waited for and the next chunk's expected load time
// that was set against, where its player said one: a player that is not a follower starts, and plays
// on after a seek, only once what it loaded outlasts loading its next chunk, with a margin, and a
// Fire TV that played at once with 1.4 s loaded stalled (the owner's cast, staging, 2026-10-28).
type StartGateBody struct {
	LoadedAheadMs    *int64 `json:"loadedAheadMs,omitempty"    minimum:"0" required:"false"`
	WaitedForAheadMs *int64 `json:"waitedForAheadMs,omitempty" minimum:"0" required:"false"`
	NextChunkLoadMs  *int64 `json:"nextChunkLoadMs,omitempty"  minimum:"0" required:"false"`
	// NextChunkLoadFrom is what the next chunk's expected load rested on: its player's estimate of
	// its link from its own samples, the link its device last measured on its network blended in
	// (confident), that remembered link alone before any sample (remembered), or none, so it waited
	// refillAheadMs (bandwidth-sampling-2026-10-29.md P5). firstSample, its first chunk's speed where
	// that was lower than the remembered link, is a player's of before 2026-10-29, still taken from a
	// page of a release before. A lone fast first chunk let a Fire TV start with 1 s loaded (staging,
	// 2026-10-28).
	//nolint:lll // one struct tag
	NextChunkLoadFrom string `enum:"confident,remembered,firstSample,none" json:"nextChunkLoadFrom,omitempty" required:"false"`
}

// attributes are the start gate's for the log: each that was told.
func (gate StartGateBody) attributes() []any {
	var attributes []any
	if gate.LoadedAheadMs != nil {
		attributes = append(attributes, "loadedAheadMs", *gate.LoadedAheadMs)
	}
	if gate.WaitedForAheadMs != nil {
		attributes = append(attributes, "waitedForAheadMs", *gate.WaitedForAheadMs)
	}
	if gate.NextChunkLoadMs != nil {
		attributes = append(attributes, "nextChunkLoadMs", *gate.NextChunkLoadMs)
	}
	if gate.NextChunkLoadFrom != "" {
		attributes = append(attributes, "nextChunkLoadFrom", gate.NextChunkLoadFrom)
	}
	return attributes
}

// StandstillBody is what a video showed when, asked to play and not waiting, it had kept its
// position for ForMs: whether it held the film, decoded a frame, met an error, or played with the page
// hidden. A phone's Brave said "playing" at 0:00 and stood there through every film, direct or
// chunked, and the log said only that it started and paused (the owner's friend, 2026-10-12).
type StandstillBody struct {
	PositionMs int64 `json:"positionMs" minimum:"0"`
	ForMs      int64 `json:"forMs"      minimum:"0"`
	// ReadyState and NetworkState are the element's own.
	ReadyState   int64 `json:"readyState"   minimum:"0" maximum:"4"`
	NetworkState int64 `json:"networkState" minimum:"0" maximum:"3"`
	// Buffered is what the element holds; VideoBuffered and AudioBuffered, a chunked playback's
	// picture and sound, as "from-to" ranges in seconds.
	Buffered      string `json:"buffered"                maxLength:"400"`
	VideoBuffered string `json:"videoBuffered,omitempty" maxLength:"400" required:"false"`
	AudioBuffered string `json:"audioBuffered,omitempty" maxLength:"400" required:"false"`
	LastError     string `json:"lastError"               maxLength:"200"`
	// VideoWidth and VideoHeight are 0 until a frame is decoded; DecodedFrames and DroppedFrames are
	// the browser's count, absent where it keeps none.
	VideoWidth    int64   `json:"videoWidth"              minimum:"0"`
	VideoHeight   int64   `json:"videoHeight"             minimum:"0"`
	DecodedFrames *int64  `json:"decodedFrames,omitempty" minimum:"0" required:"false"`
	DroppedFrames *int64  `json:"droppedFrames,omitempty" minimum:"0" required:"false"`
	IsMuted       bool    `json:"isMuted"`
	Volume        float64 `json:"volume"                  minimum:"0" maximum:"1"`
	IsPageHidden  bool    `json:"isPageHidden"`
}

// UnplayableBody is a quality a player gave up as one its device cannot play: its rate, and where
// the video stood still while its chunks arrived (D266).
type UnplayableBody struct {
	QualityKbps int64 `json:"qualityKbps" minimum:"0"`
	PositionMs  int64 `json:"positionMs"  minimum:"0"`
	// ReadyState, IsPaused, VideoBuffered, AudioBuffered and LastError are the video as it was when
	// the quality was given up, before its decoding was built anew, as a stall's are: an autoplaying
	// video stands paused until it can play (D266 §1).
	ReadyState    int64  `json:"readyState,omitempty"    minimum:"0"       maximum:"4" required:"false"`
	IsPaused      bool   `json:"isPaused,omitempty"      required:"false"`
	VideoBuffered string `json:"videoBuffered,omitempty" maxLength:"400" required:"false"`
	AudioBuffered string `json:"audioBuffered,omitempty" maxLength:"400" required:"false"`
	LastError     string `json:"lastError,omitempty"     maxLength:"200" required:"false"`
	// IsFilmsOwnPicture is whether the quality was the film's own picture, copied as it is: the
	// server then offers it to none of the device's later playbacks of the film.
	IsFilmsOwnPicture bool `json:"isFilmsOwnPicture,omitempty" required:"false"`
}

// StartupBody is how long a playback took to start, each from when the player asked how the file
// plays, in milliseconds: to the answer, to the first frame's data, and to playing.
type StartupBody struct {
	DecidedAfterMs   int64 `json:"decidedAfterMs"  minimum:"0"`
	FirstDataAfterMs int64 `json:"firstDataAfterMs" minimum:"0"`
	PlayingAfterMs   int64 `json:"playingAfterMs"   minimum:"0"`
	// PreloadSteps are the last steps of the film page's hidden player that loaded the playback's
	// first buffer (D260 §3), oldest first, as a stall's are: on the owner's LAN it stepped down to
	// the lowest quality before Play was pressed, and nothing said why (staging, 2026-10-18).
	PreloadSteps string `json:"preloadSteps,omitempty" maxLength:"400" required:"false"`
	// Sound is how the player's sound reaches the speakers, said once it plays: the owner's TCL TV in
	// TV Bro played every film's sound half a second late, YouTube's in time, and nothing in the log
	// said why (2026-10-24).
	Sound *StartupSoundBody `json:"sound,omitempty" required:"false"`
	StartGateBody
}

// StartupSoundBody is how a player's sound reaches the speakers: through Web Audio for its gain or
// straight from the element, the latencies its audio context reports when through Web Audio, the
// codecs its buffers took, and where its player placed each track's first chunk (the mapping).
type StartupSoundBody struct {
	IsThroughWebAudio bool `json:"isThroughWebAudio,omitempty" required:"false"`
	// BaseLatencyMs and OutputLatencyMs are the audio context's baseLatency and outputLatency, absent
	// when the sound is not through Web Audio or the browser does not say.
	BaseLatencyMs   *int64 `json:"baseLatencyMs,omitempty"   minimum:"0" required:"false"`
	OutputLatencyMs *int64 `json:"outputLatencyMs,omitempty" minimum:"0" required:"false"`
	// EarlierMs is how much earlier the player moves the sound's chunks; DelayMs, how long Web Audio
	// holds it back; LatencyCountedMs, Web Audio's latency it counted toward the device's sound offset
	// (the owner's TCL TV, 2026-11-06: the timing changed between runs with Level loudness on).
	EarlierMs        int64  `json:"earlierMs,omitempty"        maximum:"2000" minimum:"0" required:"false"`
	DelayMs          int64  `json:"delayMs,omitempty"          maximum:"2000" minimum:"0" required:"false"`
	LatencyCountedMs int64  `json:"latencyCountedMs,omitempty" maximum:"10000" minimum:"0" required:"false"`
	Codecs           string `json:"codecs,omitempty"          maxLength:"200" required:"false"`
	Mapping          string `json:"mapping,omitempty"         maxLength:"300" required:"false"`
}

// StallBody is one stall: a wait for data, while the player played, where it was; and the video's
// readyState and whether it was paused then, which say whether it lacked data or could not play
// what it had (D266: Flow's video sat paused at readyState 1, which only a guess found).
type StallBody struct {
	PositionMs int64 `json:"positionMs"           minimum:"0"`
	ReadyState int64 `json:"readyState,omitempty" minimum:"0" maximum:"4" required:"false"`
	IsPaused   bool  `json:"isPaused,omitempty"   required:"false"`
	// VideoBuffered and AudioBuffered are what the picture's and the sound's buffers held, as
	// "from-to" ranges in seconds; LastError, the player's last error, recovered or not.
	VideoBuffered string `json:"videoBuffered,omitempty" maxLength:"400" required:"false"`
	AudioBuffered string `json:"audioBuffered,omitempty" maxLength:"400" required:"false"`
	LastError     string `json:"lastError,omitempty"     maxLength:"200" required:"false"`
	// Steps are the player's last steps, oldest first: the quality it chose, each playlist and chunk
	// it loaded, aborted or failed, and its video detached and attached. A Mac's Firefox, its film's
	// own picture given up at 0:00, stood with nothing loaded and asked for nothing more, and no
	// report said where its player had stopped (the owner's test, 2026-10-18).
	Steps string `json:"steps,omitempty" maxLength:"400" required:"false"`
	// LinkSamples are the chunked player's last samples of its link, each its quality, bytes, time
	// to arrive and wait for its first byte (D352 §5), in a field of their own so the steps' 400
	// characters are not shared with them and the log can be searched for them.
	LinkSamples string `json:"linkSamples,omitempty" maxLength:"300" required:"false"`
}

// EventBody is one change of play state.
type EventBody struct {
	Kind           string `enum:"started,paused,seeked,completed,abandoned" json:"kind"`
	PositionMs     int64  `json:"positionMs" minimum:"0"`
	HappenedMonoMs int64  `json:"happenedMonoMs"     minimum:"0"`
	// Context is what was true of the page as it paused or moved (EventContextBody).
	Context *EventContextBody `json:"context,omitempty" required:"false"`
}

// SpanBody is a run of watching, from one position to another between two moments.
type SpanBody struct {
	FromPositionMs int64 `json:"fromPositionMs" minimum:"0"`
	ToPositionMs   int64 `json:"toPositionMs"   minimum:"0"`
	StartedMonoMs  int64 `json:"startedMonoMs"  minimum:"0"`
	EndedMonoMs    int64 `json:"endedMonoMs"    minimum:"0"`
	IsUnattended   bool  `doc:"Played after nobody answered still watching (D188)" json:"isUnattended,omitempty"`
}

func (server *Server) registerReport() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "reportPlayback", Method: http.MethodPost, Path: "/playback/{token}/report",
		Summary:    "Report where a playback is and what changed; the position never moves back (D217 §7)",
		Tags:       []string{tagPlayback},
		Extensions: callableBy(capabilityMember, schemeSession, schemeMembership),
	}, server.reportPlayback)
}

// ReportOutput answers a play report: empty, or the admin's stop, which ends the playback (D243 §9).
type ReportOutput struct {
	Body struct {
		Stop *StopBody `doc:"The admin stopped this playback: stop, and show the message" json:"stop,omitempty"`
		//nolint:lll // one struct tag
		LowerQuality *LadderBody `doc:"The lower quality the first stall started, ready or not (D253 §1)" json:"lowerQuality,omitempty"`
	}
}

// LadderBody is a film's ladder, the film cut into chunks at every quality, and where on it a player
// starts: a playback's start (D260), or the lower quality a stalled player moves to (D247 §2, §6,
// §10).
type LadderBody struct {
	URL string `doc:"The chunked master playlist" json:"url"`
	// StartAtBps is the chosen quality's BANDWIDTH in the master playlist, which names it there.
	StartAtBps  int64 `json:"startAtBps"`
	QualityKbps int   `json:"qualityKbps"`
	Height      int   `json:"height"`
	// IsBelowTheLowest is whether even the lowest quality is beyond the link as measured (D247 §8).
	IsBelowTheLowest bool `json:"isBelowTheLowest"`
	// SoundCarriedGainDB is the levelling gain the sound's chunks carry,
	// which the player does not add again (D226 §4).
	SoundCarriedGainDB float64 `json:"soundCarriedGainDb"`
	// IsReady is whether the quality is made from where the player is, ready to move to; until it
	// is, the player asks again (D253 §1). Always false at a start, whose player waits for its chunks.
	IsReady bool `json:"isReady"`
	// FilmPictureKbps is the film's own picture's average rate, which the player weighs its link
	// against once the quality is ready (D253 §2); 0 when not known.
	FilmPictureKbps int64 `json:"filmPictureKbps"`
	// LinkSharePercent is the share of a measured link a quality may take, by which the server chose
	// this one and the player chooses its own: link_share_for_quality_percent (D247 §6).
	LinkSharePercent int64 `json:"linkSharePercent"`
	// Steadiness is the steadiness level the player starts at (D352 §3): the one its last playback on
	// this network ended at, relaxed by the time since; 0 for none remembered (D352 Q2).
	Steadiness int `json:"steadiness" maximum:"3" minimum:"0"`
	// RememberedLinkKbps is, at a playback's start, the link its device last measured on this
	// network (D257 §1), which its player's estimate starts from as a prior, and its start gate reads
	// before any chunk (bandwidth-sampling-2026-10-29.md P2, P5): one fast first chunk let a Fire TV
	// start with 1 s loaded, its next chunk taking four times what that chunk promised (staging,
	// 2026-10-28). Absent for none, and on a lower quality offered after a stall.
	RememberedLinkKbps *int64 `json:"rememberedLinkKbps,omitempty" minimum:"1" required:"false"`
	// RememberedLinkSpread is the spread of the natural logs of the samples RememberedLinkKbps rested
	// on, absent where its player said none, and RememberedLinkSamples how many of the playback's own
	// samples it counts as in its player's estimate: two while under an hour old, less in step with
	// its age after, none at link_speed_fresh_hours (bandwidth-sampling-2026-10-29.md P2). Each absent
	// with RememberedLinkKbps.
	RememberedLinkSpread  *float64 `json:"rememberedLinkSpread,omitempty"  minimum:"0" required:"false"`
	RememberedLinkSamples *float64 `json:"rememberedLinkSamples,omitempty" minimum:"0" required:"false"`
}

// ladderBody is a place on a film's ladder offered to the playback a media token names, by its
// chunks' master playlist, whether it is ready, the steadiness level its player starts at and the
// link its device last measured on its network, the zero value for none.
func ladderBody(
	token string, offer chunks.Offer, isReady bool, steadiness int, remembered playback.RememberedLink,
) *LadderBody {
	body := &LadderBody{
		URL: Prefix + "/bytes/media/chunks/" + token + "/master.m3u8", StartAtBps: offer.StartAtBps,
		QualityKbps: offer.Quality.Kbps, Height: offer.Quality.Height, IsBelowTheLowest: offer.IsBelowTheLowest,
		SoundCarriedGainDB: offer.SoundCarriedGainDB, IsReady: isReady, FilmPictureKbps: offer.FilmPictureKbps,
		LinkSharePercent: offer.LinkSharePercent, Steadiness: steadiness,
	}
	if remembered.Kbps > 0 {
		body.RememberedLinkKbps = &remembered.Kbps
		body.RememberedLinkSamples = &remembered.CountsAsSamples
		if remembered.LogSpread > 0 {
			body.RememberedLinkSpread = &remembered.LogSpread
		}
	}
	return body
}

// StopBody is the admin's stop of a playback, and what they said.
type StopBody struct {
	Message string `json:"message"`
}

func (server *Server) reportPlayback(ctx context.Context, input *ReportInput) (*ReportOutput, error) {
	reporter, err := server.reporterOf(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	report := playback.Report{
		ReportSeq: input.Body.ReportSeq, SentMonoMs: input.Body.SentMonoMs, PositionMs: input.Body.PositionMs,
		PositionReadMonoMs: input.Body.PositionReadMonoMs, LinkKbps: input.Body.LinkKbps,
		LinkLogSpread: input.Body.LinkSpread,
		Steadiness:    input.Body.Steadiness, ClientAddress: input.clientAddress,
	}
	for _, stall := range input.Body.Stalls {
		report.Stalls = append(report.Stalls, playback.Stall{PositionMs: stall.PositionMs})
	}
	for _, event := range input.Body.Events {
		report.Events = append(report.Events, playback.Event{
			Kind: playback.EventKind(event.Kind), PositionMs: event.PositionMs, HappenedMonoMs: event.HappenedMonoMs,
		})
	}
	for _, span := range input.Body.Spans {
		report.Spans = append(report.Spans, playback.Span{
			FromPositionMs: span.FromPositionMs, ToPositionMs: span.ToPositionMs,
			StartedMonoMs: span.StartedMonoMs, EndedMonoMs: span.EndedMonoMs, IsUnattended: span.IsUnattended,
		})
	}
	for _, unplayable := range input.Body.Unplayables {
		report.IsOwnPictureUnplayable = report.IsOwnPictureUnplayable || unplayable.IsFilmsOwnPicture
	}
	answer, err := reporter.dependencies.Playback.Report(ctx, input.Token, reporter.holder, reporter.deviceID, report)
	if err != nil {
		return nil, server.playbackError(ctx, "keep a play report", err)
	}
	if !answer.IsRepeat {
		server.logEvents(ctx, reporter, input.Body.Events)
		server.logPathMoves(ctx, reporter, input.Body.PathMoves)
	}
	server.logReport(ctx, reporter, report, answer, input.Body.Stalls)
	server.logStartup(ctx, reporter, input.Body.Startup, input.Body.LinkKbps)
	server.logUnplayables(ctx, reporter, input.Body.Unplayables)
	server.logSeeks(ctx, reporter, input.Body.Seeks, input.Body.LinkKbps)
	server.logStandstills(ctx, reporter, input.Body.Standstills, input.Body.LinkKbps)
	server.logDroppedFrames(ctx, reporter, input.Body.TotalFrames, input.Body.DroppedFrames, input.Body.PositionMs)
	if !answer.IsRepeat {
		server.logLinkNotes(ctx, reporter, input.Body.LinkNotes, input.Body.PositionMs)
	}
	output := &ReportOutput{}
	if answer.StopMessage != nil {
		output.Body.Stop = &StopBody{Message: *answer.StopMessage}
	}
	if offer := answer.LowerQuality; offer != nil {
		output.Body.LowerQuality = ladderBody(
			input.Token, *offer, answer.IsLowerQualityReady, answer.LowerQualitySteadiness, playback.RememberedLink{})
	}
	return output, nil
}

// logReport says what a report brought that a person would ask about: stalls, with where and the
// link's speed, and the lower quality offered for them, ready or not (D253 §1). A viewer in Singapore saw a still
// frame every few seconds, and nothing said what her player reported or was offered (staging,
// 2026-10-06). Who is the holder's user, 0 for a viewer, and the file they may play; never the
// session's key.
func (server *Server) logReport(
	ctx context.Context, reporter reporter, report playback.Report, answer playback.Answer, stalls []StallBody,
) {
	who := append(reporter.logAttributes(), "positionMs", report.PositionMs, "linkKbps", report.LinkKbps)
	if len(report.Stalls) == 0 {
		return
	}
	positions := make([]int64, 0, len(report.Stalls))
	for _, stall := range report.Stalls {
		positions = append(positions, stall.PositionMs)
	}
	server.logger.InfoContext(ctx, "a player reported stalls", append(append(who, "stallPositionsMs", positions),
		stallDiagnosis(stalls)...)...)
	if offer := answer.LowerQuality; offer != nil {
		server.logger.InfoContext(ctx, "a stalling player was offered a lower quality", append(who,
			"qualityKbps", offer.Quality.Kbps, "height", offer.Quality.Height,
			"isBelowTheLowest", offer.IsBelowTheLowest, "isReady", answer.IsLowerQualityReady)...)
	}
}

// logEvents says each change of a player's play state a report brought, and where: a start, a pause,
// a seek, the end, or the player closed. A Mac playing alone had paused and closed before the owner
// told of it, and the log said only that it had started (the owner, 2026-10-12: "We should be logging
// that").
func (server *Server) logEvents(ctx context.Context, reporter reporter, events []EventBody) {
	for _, event := range events {
		server.logger.InfoContext(ctx, "a player's play state changed", append(append(reporter.logAttributes(),
			"kind", event.Kind, "positionMs", event.PositionMs), eventContextAttributes(event)...)...)
	}
}

// logStartup says how long a playback took to start, as its player measured it, with its link's
// speed: from asking how the file plays to the answer, to the first frame's data, and to playing.
// A start of a minute or more was told of only by the owner (2026-10-07); the playback's own
// "started" line says how it plays.
func (server *Server) logStartup(ctx context.Context, reporter reporter, startup *StartupBody, linkKbps int64) {
	if startup == nil {
		return
	}
	server.logger.InfoContext(ctx, "a playback started playing", append(append(reporter.logAttributes(),
		"decidedAfterMs", startup.DecidedAfterMs, "firstDataAfterMs", startup.FirstDataAfterMs,
		"playingAfterMs", startup.PlayingAfterMs, "linkKbps", linkKbps, "preloadSteps", startup.PreloadSteps),
		startup.attributes()...)...)
	if sound := startup.Sound; sound != nil {
		server.logger.InfoContext(ctx, "a playback's sound", "userId", reporter.holder.UserID,
			"deviceId", reporter.deviceID, "isThroughWebAudio", sound.IsThroughWebAudio,
			"baseLatencyMs", msOrNone(sound.BaseLatencyMs), "outputLatencyMs", msOrNone(sound.OutputLatencyMs),
			"earlierMs", sound.EarlierMs, "delayMs", sound.DelayMs, "latencyCountedMs", sound.LatencyCountedMs,
			"codecs", sound.Codecs,
			"mapping", sound.Mapping)
	}
}

// msOrNone is a reported latency for the log: its milliseconds, or "none" where the browser said none.
func msOrNone(ms *int64) any {
	if ms == nil {
		return "none"
	}
	return *ms
}

// logUnplayables says each quality a player gave up as one its device cannot play, and where: Flow
// froze in Firefox on macOS at any start but its beginning, and only the owner's test said why
// (D266).
func (server *Server) logUnplayables(ctx context.Context, reporter reporter, unplayables []UnplayableBody) {
	for _, unplayable := range unplayables {
		server.logger.InfoContext(ctx, "a player found a quality its device cannot play",
			"userId", reporter.holder.UserID, "deviceId", reporter.deviceID,
			"viewerOfFileId", reporter.holder.OnlyFileID,
			"qualityKbps", unplayable.QualityKbps, "positionMs", unplayable.PositionMs,
			"readyState", unplayable.ReadyState, "isPaused", unplayable.IsPaused,
			"videoBuffered", unplayable.VideoBuffered, "audioBuffered", unplayable.AudioBuffered,
			"lastError", unplayable.LastError, "isFilmsOwnPicture", unplayable.IsFilmsOwnPicture)
	}
}

// logSeeks says how long each of the person's seeks took to play again, with the link's speed.
func (server *Server) logSeeks(ctx context.Context, reporter reporter, seeks []SeekBody, linkKbps int64) {
	for _, seek := range seeks {
		attributes := append(reporter.logAttributes(),
			"fromPositionMs", seek.FromPositionMs, "toPositionMs", seek.ToPositionMs,
			"readyAfterMs", seek.ReadyAfterMs, "isPaused", seek.IsPaused, "linkKbps", linkKbps)
		if seek.PictureAfterMs != nil {
			attributes = append(attributes, "pictureAfterMs", *seek.PictureAfterMs)
		}
		if seek.SoundAfterMs != nil {
			attributes = append(attributes, "soundAfterMs", *seek.SoundAfterMs)
		}
		if seek.Steps != "" {
			attributes = append(attributes, "steps", seek.Steps)
		}
		if seek.LinkSamples != "" {
			attributes = append(attributes, "linkSamples", seek.LinkSamples)
		}
		attributes = append(attributes, seek.attributes()...)
		server.logger.InfoContext(ctx, "a player's seek was ready to play", attributes...)
	}
}

// logStandstills says each place a player, asked to play, kept its position without waiting, and
// what its video showed there.
func (server *Server) logStandstills(
	ctx context.Context, reporter reporter, standstills []StandstillBody, linkKbps int64,
) {
	for _, still := range standstills {
		attributes := append(reporter.logAttributes(),
			"positionMs", still.PositionMs, "forMs", still.ForMs,
			"readyState", still.ReadyState, "networkState", still.NetworkState,
			"buffered", still.Buffered, "videoBuffered", still.VideoBuffered, "audioBuffered", still.AudioBuffered,
			"lastError", still.LastError, "videoWidth", still.VideoWidth, "videoHeight", still.VideoHeight,
			"isMuted", still.IsMuted, "volume", still.Volume, "isPageHidden", still.IsPageHidden,
			"linkKbps", linkKbps)
		if still.DecodedFrames != nil {
			attributes = append(attributes, "decodedFrames", *still.DecodedFrames)
		}
		if still.DroppedFrames != nil {
			attributes = append(attributes, "droppedFrames", *still.DroppedFrames)
		}
		server.logger.InfoContext(ctx, "a playing player's position stood still", attributes...)
	}
}

// droppedFramesLoggedAbove is the share of a report's frames that, dropped, is logged: a smooth
// playback drops none, and one frame in a ten-second report of 24 fps film (240 frames) is 0.4%,
// which no one sees, while three or more are a visible stutter. A justified constant: it chooses only
// what the log says, nothing the player does.
const droppedFramesLoggedAbove = 0.01

// logDroppedFrames says where a player dropped more than droppedFramesLoggedAbove of the frames it
// showed since its last report, with how many of how many, so a burst of stutter shows in the log
// near when it happened (the owner, 2026-10-28).
func (server *Server) logDroppedFrames(
	ctx context.Context, reporter reporter, totalFrames, droppedFrames *int64, positionMs int64,
) {
	if totalFrames == nil || droppedFrames == nil || *totalFrames <= 0 ||
		float64(*droppedFrames) <= droppedFramesLoggedAbove*float64(*totalFrames) {
		return
	}
	server.logger.InfoContext(ctx, "a player dropped frames", append(reporter.logAttributes(),
		"positionMs", positionMs, "droppedFrames", *droppedFrames, "totalFrames", *totalFrames)...)
}

// linkNoteMostRunes is how much of each of a report's notes on its link the log keeps: the player's
// own bound (linkNoteMost in link-estimate.ts), so a report written past it logs no more.
const linkNoteMostRunes = 200

// logLinkNotes says a chunked player's notes on its link since its last report, where it sent any:
// its samples, each with the time expected and its bound's, and its chunks given up or stuck with
// what carried them (bandwidth-sampling-2026-10-29.md P6).
func (server *Server) logLinkNotes(ctx context.Context, reporter reporter, notes []string, positionMs int64) {
	if len(notes) == 0 {
		return
	}
	kept := make([]string, 0, len(notes))
	for _, note := range notes {
		if runes := []rune(note); len(runes) > linkNoteMostRunes {
			note = string(runes[:linkNoteMostRunes])
		}
		kept = append(kept, note)
	}
	server.logger.InfoContext(ctx, "a player's notes on its link", append(reporter.logAttributes(),
		"positionMs", positionMs, "linkNotes", kept)...)
}

// stallDiagnosis is each stall's video as its player saw it, for the log: its readyState, whether
// it was paused, what its picture's and sound's buffers held, its last error (D266), and its player's
// last steps.
func stallDiagnosis(stalls []StallBody) []any {
	readyStates := make([]int64, 0, len(stalls))
	paused := make([]bool, 0, len(stalls))
	videoBuffered := make([]string, 0, len(stalls))
	audioBuffered := make([]string, 0, len(stalls))
	lastErrors := make([]string, 0, len(stalls))
	steps := make([]string, 0, len(stalls))
	samples := make([]string, 0, len(stalls))
	for _, stall := range stalls {
		steps, samples = append(steps, stall.Steps), append(samples, stall.LinkSamples)
		readyStates, paused = append(readyStates, stall.ReadyState), append(paused, stall.IsPaused)
		videoBuffered = append(videoBuffered, stall.VideoBuffered)
		audioBuffered = append(audioBuffered, stall.AudioBuffered)
		lastErrors = append(lastErrors, stall.LastError)
	}
	return []any{
		"stallReadyStates", readyStates, "stallsPaused", paused,
		"stallVideoBuffered", videoBuffered, "stallAudioBuffered", audioBuffered, "stallLastErrors", lastErrors,
		"stallSteps", steps, "stallLinkSamples", samples,
	}
}

// reporter is who reports on a playback, and from which device: 0 for none.
type reporter struct {
	dependencies *Dependencies
	holder       playback.Holder
	deviceID     int64
}

// logAttributes are who reported, for the log: the holder's user, 0 for a viewer, their device, and
// the file they may play; never the session's key.
func (reporter reporter) logAttributes() []any {
	return []any{
		"userId", reporter.holder.UserID, "deviceId", reporter.deviceID, "viewerOfFileId", reporter.holder.OnlyFileID,
	}
}

// reporterOf is who reports on a playback: a signed-in person, on their device, or a Watch Together
// viewer by their membership, whose reports keep nothing (D243 §2) but whose stalls are answered as
// anyone's (D247 §2).
func (server *Server) reporterOf(ctx context.Context, authorization string) (reporter, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, authorization)
	if err == nil {
		holder := playback.HolderOf(principal)
		return reporter{dependencies: dependencies, holder: holder, deviceID: principal.DeviceID}, nil
	}
	ready, readyErr := server.readyDependencies()
	if readyErr != nil {
		return reporter{}, err
	}
	if viewer, isViewer := server.viewerHolder(ctx, ready, bearerToken(authorization)); isViewer {
		return reporter{dependencies: ready, holder: viewer}, nil
	}
	return reporter{}, err
}
