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
	}
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
		LowerQuality *LowerQualityBody `doc:"A lower quality to move to: the player stalled again (D247 §2)" json:"lowerQuality,omitempty"`
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
	output := &ReportOutput{}
	if answer.StopMessage != nil {
		output.Body.Stop = &StopBody{Message: *answer.StopMessage}
	}
	if offer := answer.LowerQuality; offer != nil {
		output.Body.LowerQuality = &LowerQualityBody{
			URL: Prefix + "/bytes/media/chunks/" + input.Token + "/master.m3u8", StartAtBps: offer.StartAtBps,
			QualityKbps: offer.Quality.Kbps, Height: offer.Quality.Height, IsBelowTheLowest: offer.IsBelowTheLowest,
			SoundCarriedGainDB: offer.SoundCarriedGainDB,
		}
	}
	return output, nil
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
