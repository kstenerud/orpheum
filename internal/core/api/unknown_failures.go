// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/unexpected"
)

// UnknownFailureAlertsOutput is GET /unknown-failures/alerts's answer.
type UnknownFailureAlertsOutput struct {
	Body UnknownFailureAlertsBody
}

// UnknownFailureAlertsBody is what an admin's banner says: each kind of work whose failures of
// unknown cause passed the alert setting within the last hour (D509 §2).
type UnknownFailureAlertsBody struct {
	Alerts []UnknownFailureAlertBody `json:"alerts"`
}

// UnknownFailureAlertBody is one kind's alert.
type UnknownFailureAlertBody struct {
	Kind     string `json:"kind"`
	LastHour int64  `doc:"How many of its failures of unknown cause were in the last hour" json:"lastHour"`
}

// UnknownFailuresOutput is GET /unknown-failures's answer.
type UnknownFailuresOutput struct {
	Body UnknownFailuresBody
}

// UnknownFailuresBody is every failure of unknown cause the server keeps, by kind, with everything
// each said, for the Needs attention page and to be taken out as a report (D509 §3, D511 §5).
type UnknownFailuresBody struct {
	Version string                   `doc:"The server's version" json:"version"`
	StartID string                   `doc:"This start of the server" json:"startId"`
	At      int64                    `doc:"When the report was made, in Unix milliseconds" json:"at"`
	Kinds   []UnknownFailureKindBody `json:"kinds"`
}

// UnknownFailureKindBody is one kind of work's failures of unknown cause.
type UnknownFailureKindBody struct {
	Kind       string               `json:"kind"`
	LastHour   int64                `doc:"How many failed in the last hour" json:"lastHour"`
	SinceStart int64                `doc:"How many failed since the server started" json:"sinceStart"`
	LastAt     int64                `doc:"When one last failed, in Unix milliseconds" json:"lastAt"`
	IsRaised   bool                 `doc:"Whether its alert is raised" json:"isRaised"`
	Failures   []UnknownFailureBody `json:"failures"`
}

// UnknownFailureBody is one failure kept, in full.
type UnknownFailureBody struct {
	Subject     string `doc:"What the work was for, as \"item 12\"" json:"subject"`
	Error       string `json:"error"`
	Output      string `json:"output"`
	Version     string `doc:"The server's version when it last failed" json:"version"`
	FirstAt     int64  `doc:"When it first failed, in Unix milliseconds" json:"firstAt"`
	LastAt      int64  `doc:"When it last failed, in Unix milliseconds" json:"lastAt"`
	Count       int64  `doc:"How many times it failed" json:"count"`
	IsThisStart bool   `doc:"Whether it failed since the server started" json:"isThisStart"`
}

func (server *Server) registerUnknownFailures() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getUnknownFailureAlerts", Method: http.MethodGet, Path: "/unknown-failures/alerts",
		Summary:    "The kinds of work whose failures of unknown cause raise the admin's alert (D509)",
		Tags:       []string{tagRunningIt},
		Extensions: callableBy(capabilityAdmin),
	}, server.getUnknownFailureAlerts)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getUnknownFailures", Method: http.MethodGet, Path: "/unknown-failures",
		Summary:    "Every failure of unknown cause, by kind, in full: the summary and the export (D509, D511)",
		Tags:       []string{tagRunningIt},
		Extensions: callableBy(capabilityAdmin),
	}, server.getUnknownFailures)
}

func (server *Server) getUnknownFailureAlerts(
	ctx context.Context, input *AuthorizationHeader,
) (*UnknownFailureAlertsOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	output := &UnknownFailureAlertsOutput{Body: UnknownFailureAlertsBody{Alerts: []UnknownFailureAlertBody{}}}
	if dependencies.UnknownFailures == nil {
		return output, nil
	}
	alerts, err := dependencies.UnknownFailures.Alerts(ctx)
	if err != nil {
		return nil, server.logInternalError(ctx, "read the unknown failures' alerts", err)
	}
	for _, alert := range alerts {
		output.Body.Alerts = append(output.Body.Alerts,
			UnknownFailureAlertBody{Kind: alert.Kind, LastHour: alert.LastHour})
	}
	return output, nil
}

func (server *Server) getUnknownFailures(
	ctx context.Context, input *AuthorizationHeader,
) (*UnknownFailuresOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	output := &UnknownFailuresOutput{Body: UnknownFailuresBody{Kinds: []UnknownFailureKindBody{}}}
	if dependencies.UnknownFailures == nil {
		return output, nil
	}
	report, err := dependencies.UnknownFailures.Report(ctx)
	if err != nil {
		return nil, server.logInternalError(ctx, "report the unknown failures", err)
	}
	output.Body.Version, output.Body.StartID, output.Body.At = report.Version, report.StartID, report.At.UnixMilli()
	for _, kind := range report.Kinds {
		output.Body.Kinds = append(output.Body.Kinds, unknownFailureKindBody(kind))
	}
	return output, nil
}

// unknownFailureKindBody is one kind of the recorder's report as the API says it.
func unknownFailureKindBody(kind unexpected.Kind) UnknownFailureKindBody {
	body := UnknownFailureKindBody{
		Kind: kind.Kind, LastHour: kind.LastHour, SinceStart: kind.SinceStart, LastAt: kind.LastAt.UnixMilli(),
		IsRaised: kind.IsRaised, Failures: make([]UnknownFailureBody, 0, len(kind.Failures)),
	}
	for _, failure := range kind.Failures {
		body.Failures = append(body.Failures, UnknownFailureBody{
			Subject: failure.Subject, Error: failure.Error, Output: failure.Output, Version: failure.Version,
			FirstAt: failure.FirstAt.UnixMilli(), LastAt: failure.LastAt.UnixMilli(), Count: failure.Count,
			IsThisStart: failure.IsThisStart,
		})
	}
	return body
}
