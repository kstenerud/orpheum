// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

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
	}
}

// StartupBody is how long a playback took to start, each from when the player asked how the file
// plays, in milliseconds: to the answer, to the first frame's data, and to playing.
type StartupBody struct {
	DecisionMs  int64 `json:"decisionMs"  minimum:"0"`
	FirstDataMs int64 `json:"firstDataMs" minimum:"0"`
	PlayingMs   int64 `json:"playingMs"   minimum:"0"`
}

// StallBody is one stall: a wait for data, while the player played, where it was.
type StallBody struct {
	PositionMs int64 `json:"positionMs" minimum:"0"`
}

// EventBody is one change of play state.
type EventBody struct {
	Kind           string `enum:"started,paused,seeked,completed,abandoned" json:"kind"`
	PositionMs     int64  `json:"positionMs" minimum:"0"`
	HappenedMonoMs int64  `json:"happenedMonoMs"     minimum:"0"`
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
		LowerQuality *LowerQualityBody `doc:"The lower quality the first stall started, ready or not (D253 §1)" json:"lowerQuality,omitempty"`
	}
}

// LowerQualityBody is a lower quality a stalled player moves to: its film cut into chunks, at every
// quality, starting at the one chosen for its link (D247 §2, §6, §10).
type LowerQualityBody struct {
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
	// is, the player asks again (D253 §1).
	IsReady bool `json:"isReady"`
	// FilmPictureKbps is the film's own picture's average rate, which the player weighs its link
	// against once the quality is ready (D253 §2); 0 when not known.
	FilmPictureKbps int64 `json:"filmPictureKbps"`
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
	answer, err := reporter.dependencies.Playback.Report(ctx, input.Token, reporter.holder, reporter.deviceID, report)
	if err != nil {
		return nil, server.playbackError(ctx, "keep a play report", err)
	}
	server.logReport(ctx, reporter, report, answer)
	server.logStartup(ctx, reporter, input.Body.Startup, input.Body.LinkKbps)
	output := &ReportOutput{}
	if answer.StopMessage != nil {
		output.Body.Stop = &StopBody{Message: *answer.StopMessage}
	}
	if offer := answer.LowerQuality; offer != nil {
		output.Body.LowerQuality = &LowerQualityBody{
			URL: Prefix + "/bytes/media/chunks/" + input.Token + "/master.m3u8", StartAtBps: offer.StartAtBps,
			QualityKbps: offer.Quality.Kbps, Height: offer.Quality.Height, IsBelowTheLowest: offer.IsBelowTheLowest,
			SoundCarriedGainDB: offer.SoundCarriedGainDB, IsReady: answer.IsLowerQualityReady,
			FilmPictureKbps: offer.FilmPictureKbps,
		}
	}
	return output, nil
}

// logReport says what a report brought that a person would ask about: stalls, with where and the
// link's speed, and the lower quality offered for them, ready or not (D253 §1). A viewer in Singapore saw a still
// frame every few seconds, and nothing said what her player reported or was offered (staging,
// 2026-10-06). Who is the holder's user, 0 for a viewer, and the file they may play; never the
// session's key.
func (server *Server) logReport(
	ctx context.Context, reporter reporter, report playback.Report, answer playback.Answer,
) {
	who := []any{
		"userId", reporter.holder.UserID, "deviceId", reporter.deviceID, "viewerOfFileId", reporter.holder.OnlyFileID,
		"positionMs", report.PositionMs, "linkKbps", report.LinkKbps,
	}
	if len(report.Stalls) == 0 {
		return
	}
	positions := make([]int64, 0, len(report.Stalls))
	for _, stall := range report.Stalls {
		positions = append(positions, stall.PositionMs)
	}
	server.logger.InfoContext(ctx, "a player reported stalls", append(who, "stallPositionsMs", positions)...)
	if offer := answer.LowerQuality; offer != nil {
		server.logger.InfoContext(ctx, "a stalling player was offered a lower quality", append(who,
			"qualityKbps", offer.Quality.Kbps, "height", offer.Quality.Height,
			"isBelowTheLowest", offer.IsBelowTheLowest, "isReady", answer.IsLowerQualityReady)...)
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
		"deviceId", reporter.deviceID, "viewerOfFileId", reporter.holder.OnlyFileID, "decisionMs", startup.DecisionMs,
		"firstDataMs", startup.FirstDataMs, "playingMs", startup.PlayingMs, "linkKbps", linkKbps)
}

// reporter is who reports on a playback, and from which device: 0 for none.
type reporter struct {
	dependencies *Dependencies
	holder       playback.Holder
	deviceID     int64
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
