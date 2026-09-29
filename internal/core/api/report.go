// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
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
	}
}

// EventBody is one change of play state.
type EventBody struct {
	Kind       string `enum:"started,paused,seeked,completed,abandoned" json:"kind"`
	PositionMs int64  `json:"positionMs" minimum:"0"`
	AtMonoMs   int64  `json:"atMonoMs"     minimum:"0"`
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
	huma.Register(server.api, huma.Operation{
		OperationID: "reportPlayback", Method: http.MethodPost, Path: "/playback/{token}/report",
		Summary:       "Report where a playback is and what changed; the position never moves back (D217 §7)",
		Tags:          []string{tagPlayback},
		DefaultStatus: http.StatusNoContent,
	}, server.reportPlayback)
}

func (server *Server) reportPlayback(ctx context.Context, input *ReportInput) (*struct{}, error) {
	dependencies, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	report := playback.Report{
		Seq: input.Body.ReportSeq, SentMonoMs: input.Body.SentMonoMs, PositionMs: input.Body.PositionMs,
		PositionMonoMs: input.Body.PositionReadMonoMs,
	}
	for _, event := range input.Body.Events {
		report.Events = append(report.Events, playback.Event{
			Kind: playback.EventKind(event.Kind), PositionMs: event.PositionMs, MonoMs: event.AtMonoMs,
		})
	}
	for _, span := range input.Body.Spans {
		report.Spans = append(report.Spans, playback.Span{
			FromPositionMs: span.FromPositionMs, ToPositionMs: span.ToPositionMs,
			StartedMonoMs: span.StartedMonoMs, EndedMonoMs: span.EndedMonoMs, Unattended: span.IsUnattended,
		})
	}
	err = dependencies.Playback.Report(
		ctx, input.Token, principal.UserID, sessionKey(input.Authorization), principal.DeviceID, report)
	switch {
	case errors.Is(err, playback.ErrNoToken):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such playback; start it again")
	case errors.Is(err, playback.ErrBadReport):
		return nil, newError(http.StatusBadRequest, CodeInvalidRequest, err.Error())
	case err != nil:
		return nil, server.internal(ctx, "keep a play report", err)
	}
	return &struct{}{}, nil
}
