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
		Summary: "Report where a playback is and what changed; the position never moves back (D217 §7)",
		Tags:    []string{tagPlayback},
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
	// SoundCarriedGainDB is the levelling gain the sound's chunks carry, as a prepared copy's do,
	// which the player does not add again (D226 §4).
	SoundCarriedGainDB float64 `json:"soundCarriedGainDb"`
	// IsReady is whether the quality is made from where the player is, ready to move to; until it
	// is, the player asks again (D253 §1). Always false at a start, whose player waits for its chunks.
	IsReady bool `json:"isReady"`
	// FilmPictureKbps is the film's own picture's average rate, which the player weighs its link
	// against once the quality is ready (D253 §2); 0 when not known.
	FilmPictureKbps int64 `json:"filmPictureKbps"`
}

// lowerQualityBody is a lower quality offered to the playback a media token names, by its chunks'
// master playlist, and whether it is ready.
func ladderBody(token string, offer chunks.Offer, isReady bool) *LadderBody {
	return &LadderBody{
		URL: Prefix + "/bytes/media/chunks/" + token + "/master.m3u8", StartAtBps: offer.StartAtBps,
		QualityKbps: offer.Quality.Kbps, Height: offer.Quality.Height, IsBelowTheLowest: offer.IsBelowTheLowest,
		SoundCarriedGainDB: offer.SoundCarriedGainDB, IsReady: isReady, FilmPictureKbps: offer.FilmPictureKbps,
	}
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
		ClientAddress: input.clientAddress,
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
	output := &ReportOutput{}
	if answer.StopMessage != nil {
		output.Body.Stop = &StopBody{Message: *answer.StopMessage}
	}
	if offer := answer.LowerQuality; offer != nil {
		output.Body.LowerQuality = ladderBody(input.Token, *offer, answer.IsLowerQualityReady)
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
	server.logger.InfoContext(ctx, "a playback started playing", "userId", reporter.holder.UserID,
		"deviceId", reporter.deviceID, "viewerOfFileId", reporter.holder.OnlyFileID,
		"decidedAfterMs", startup.DecidedAfterMs, "firstDataAfterMs", startup.FirstDataAfterMs,
		"playingAfterMs", startup.PlayingAfterMs, "linkKbps", linkKbps)
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

// stallDiagnosis is each stall's video as its player saw it, for the log: its readyState, whether
// it was paused, what its picture's and sound's buffers held, and its last error (D266).
func stallDiagnosis(stalls []StallBody) []any {
	readyStates := make([]int64, 0, len(stalls))
	paused := make([]bool, 0, len(stalls))
	videoBuffered := make([]string, 0, len(stalls))
	audioBuffered := make([]string, 0, len(stalls))
	lastErrors := make([]string, 0, len(stalls))
	for _, stall := range stalls {
		readyStates, paused = append(readyStates, stall.ReadyState), append(paused, stall.IsPaused)
		videoBuffered = append(videoBuffered, stall.VideoBuffered)
		audioBuffered = append(audioBuffered, stall.AudioBuffered)
		lastErrors = append(lastErrors, stall.LastError)
	}
	return []any{
		"stallReadyStates", readyStates, "stallsPaused", paused,
		"stallVideoBuffered", videoBuffered, "stallAudioBuffered", audioBuffered, "stallLastErrors", lastErrors,
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
