// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/problemreport"
)

// MakeProblemReportInput is a person's problem report, made from the page they met it on (D524 §2):
// their words, the device's facts and the player's state if they played. The core adds its own
// recent log and each node's.
type MakeProblemReportInput struct {
	AuthorizationHeader
	Body MakeProblemReportBody
	ClientFacts
}

// MakeProblemReportBody is what the device sends of a report.
type MakeProblemReportBody struct {
	Words    string                     `doc:"What went wrong, in the person's own words" json:"words" maxLength:"2000" minLength:"1"`                  //nolint:lll // one struct tag
	Page     string                     `doc:"The page's path when the report was made" json:"page" maxLength:"200"`                                    //nolint:lll // one struct tag
	Device   []ProblemReportFactBody    `doc:"What the device says of itself, in its order: its browser, screen, network" json:"device" maxItems:"30"`  //nolint:lll // one struct tag
	Playback *ProblemReportPlaybackBody `doc:"The player's state, when the report was made from the player" json:"playback,omitempty" required:"false"` //nolint:lll // one struct tag
}

// ProblemReportFactBody is one of the device's facts: "Screen", "1920×1080".
type ProblemReportFactBody struct {
	Name  string `json:"name" maxLength:"60" minLength:"1"`
	Value string `json:"value" maxLength:"300"`
}

// ProblemReportPlaybackBody is the player's state as the device knew it.
type ProblemReportPlaybackBody struct {
	PlaybackID int64  `json:"playbackId" minimum:"0" required:"false"`
	ItemID     int64  `json:"itemId" minimum:"0" required:"false"`
	FileID     int64  `json:"fileId" minimum:"0" required:"false"`
	PositionMs int64  `doc:"Where the player was, in milliseconds" json:"positionMs" minimum:"0"`
	Quality    string `doc:"The quality it played at, as the player names it" json:"quality" maxLength:"60" required:"false"` //nolint:lll // one struct tag
	State      string `doc:"What it was doing: playing, stalled, paused" json:"state" maxLength:"60" required:"false"`
}

// MakeProblemReportOutput is POST /problem-reports's answer.
type MakeProblemReportOutput struct {
	Body struct {
		ID int64 `doc:"The report, which the household's admin downloads" json:"id"`
	}
}

// ProblemReportsOutput is GET /problem-reports's answer.
type ProblemReportsOutput struct {
	Body ProblemReportsOutputBody
}

// ProblemReportsOutputBody is ProblemReportsOutput's body, which the System page's first read
// carries too (the client-server audit's finding 10).
type ProblemReportsOutputBody struct {
	Reports []ProblemReportSummaryBody `doc:"Newest first, for the 60 days each is kept" json:"reports"`
}

// ProblemReportSummaryBody is one kept report as the admin's list names it.
type ProblemReportSummaryBody struct {
	ID       int64  `json:"id"`
	UserName string `doc:"Who made it; absent once their account is deleted" json:"userName,omitempty"`
	MadeAt   int64  `doc:"When it was made, in Unix milliseconds" json:"madeAt"`
	Version  string `doc:"The version that made it" json:"version"`
	Words    string `doc:"Their own words" json:"words"`
}

// ProblemReportInput names one kept report.
type ProblemReportInput struct {
	AuthorizationHeader
	ReportID int64 `minimum:"1" path:"id"`
}

// DownloadProblemReportInput is one report in the form the admin chose: there is no default, so
// each download is a choice to share it or to keep it at home (D524 §4).
type DownloadProblemReportInput struct {
	ProblemReportInput
	Form         string `doc:"shareable hides every name; full holds them, with the table of the tags" enum:"shareable,full" query:"form" required:"true"` //nolint:lll // one struct tag
	IncludeOther bool   `doc:"Put an other library's names in the full form too (D349 §10)" query:"includeOther"`                                          //nolint:lll // one struct tag
}

// DownloadProblemReportOutput is GET /problem-reports/{id}'s answer.
type DownloadProblemReportOutput struct {
	Body struct {
		Whole   string `doc:"The whole report, in Markdown, to attach as a file" json:"whole"`
		Summary string `doc:"Its newest lines that fit a GitHub issue's body, in Markdown" json:"summary"`
	}
}

func (server *Server) registerProblemReports() {
	tags := []string{tagRunningIt}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "makeProblemReport", Method: http.MethodPost, Path: "/problem-reports",
		Summary:       "Report a problem to the household's admin, with the server's and nodes' recent logs (D524)",
		Tags:          tags,
		DefaultStatus: http.StatusCreated,
		Extensions:    callableBy(capabilitySignedIn),
	}, server.makeProblemReport)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listProblemReports", Method: http.MethodGet, Path: "/problem-reports",
		Summary: "The problem reports kept, newest first (D524)", Tags: tags,
		Extensions: callableBy(capabilityAdmin),
	}, server.listProblemReports)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "downloadProblemReport", Method: http.MethodGet, Path: "/problem-reports/{id}",
		Summary: "One problem report, shareable or full, as Markdown (D524 §4)", Tags: tags,
		Extensions: callableBy(capabilityAdmin),
	}, server.downloadProblemReport)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "deleteProblemReport", Method: http.MethodDelete, Path: "/problem-reports/{id}",
		Summary: "Delete one problem report", Tags: tags,
		DefaultStatus: http.StatusNoContent,
		Extensions:    callableBy(capabilityAdmin),
	}, server.deleteProblemReport)
}

// errNoProblemReports is a server built without its problem reports: a test's.
var errNoProblemReports = errors.New("this server keeps no problem reports")

// problemReportUserAgentFact is the fact the server adds of the browser's own words for itself.
const problemReportUserAgentFact = "User agent"

func (server *Server) makeProblemReport(
	ctx context.Context, input *MakeProblemReportInput,
) (*MakeProblemReportOutput, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if dependencies.ProblemReports == nil {
		return nil, server.logInternalError(ctx, "make a problem report", errNoProblemReports)
	}
	request := problemreport.Request{
		UserID: principal.UserID, DeviceName: server.problemReportDevice(ctx, dependencies, principal.UserID,
			principal.DeviceID), Words: input.Body.Words, Page: input.Body.Page,
		Device:   make([]problemreport.Fact, 0, len(input.Body.Device)+1),
		Playback: problemReportPlayback(input.Body.Playback),
	}
	for _, fact := range input.Body.Device {
		request.Device = append(request.Device, problemreport.Fact{Name: fact.Name, Value: fact.Value})
	}
	if input.userAgent != "" {
		request.Device = append(request.Device,
			problemreport.Fact{Name: problemReportUserAgentFact, Value: input.userAgent})
	}
	reportID, err := dependencies.ProblemReports.Make(ctx, request)
	if errors.Is(err, problemreport.ErrTooMany) {
		return nil, newError(http.StatusTooManyRequests, CodeRateLimited,
			"you have reported many problems in the last hour; wait and report again")
	}
	if err != nil {
		return nil, server.logInternalError(ctx, "make a problem report", err)
	}
	output := &MakeProblemReportOutput{}
	output.Body.ID = reportID
	return output, nil
}

// problemReportDevice is the name of the device a report is made from, empty for a session with no
// device, or one gone since.
func (*Server) problemReportDevice(ctx context.Context, dependencies *Dependencies, userID, deviceID int64) string {
	if deviceID == 0 {
		return ""
	}
	device, err := dependencies.Auth.DeviceOf(ctx, userID, deviceID)
	if err != nil {
		return ""
	}
	return device.Name
}

// problemReportPlayback is the player's state as the keeper takes it, nil for none.
func problemReportPlayback(body *ProblemReportPlaybackBody) *problemreport.PlaybackRequest {
	if body == nil {
		return nil
	}
	return &problemreport.PlaybackRequest{
		PlaybackID: body.PlaybackID, ItemID: body.ItemID, FileID: body.FileID,
		Position: time.Duration(body.PositionMs) * time.Millisecond, Quality: body.Quality, State: body.State,
	}
}

func (server *Server) listProblemReports(
	ctx context.Context, input *AuthorizationHeader,
) (*ProblemReportsOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.problemReportsListed(ctx, dependencies)
}

// problemReportsListed is GET /problem-reports' answer, which the System page's first read carries
// too.
func (server *Server) problemReportsListed(
	ctx context.Context, dependencies *Dependencies,
) (*ProblemReportsOutput, error) {
	output := &ProblemReportsOutput{}
	output.Body.Reports = []ProblemReportSummaryBody{}
	if dependencies.ProblemReports == nil {
		return output, nil
	}
	summaries, err := dependencies.ProblemReports.List(ctx)
	if err != nil {
		return nil, server.logInternalError(ctx, "list the problem reports", err)
	}
	for _, summary := range summaries {
		output.Body.Reports = append(output.Body.Reports, ProblemReportSummaryBody{
			ID: summary.ID, UserName: summary.UserName, MadeAt: summary.MadeAt.UnixMilli(), Version: summary.Version,
			Words: summary.Words,
		})
	}
	return output, nil
}

func (server *Server) downloadProblemReport(
	ctx context.Context, input *DownloadProblemReportInput,
) (*DownloadProblemReportOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if dependencies.ProblemReports == nil {
		return nil, errNoSuchProblemReport()
	}
	rendered, err := dependencies.ProblemReports.Download(ctx, input.ReportID, problemreport.Options{
		Form: problemreport.Form(input.Form), IncludeOther: input.IncludeOther,
	})
	switch {
	case errors.Is(err, problemreport.ErrNoReport):
		return nil, errNoSuchProblemReport()
	case errors.Is(err, problemreport.ErrNoForm):
		return nil, invalidRequestAt(http.StatusBadRequest, "query.form", err.Error())
	case err != nil:
		return nil, server.logInternalError(ctx, "download a problem report", err)
	}
	output := &DownloadProblemReportOutput{}
	output.Body.Whole, output.Body.Summary = rendered.Whole, rendered.Summary
	return output, nil
}

func (server *Server) deleteProblemReport(ctx context.Context, input *ProblemReportInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if dependencies.ProblemReports == nil {
		return nil, errNoSuchProblemReport()
	}
	deleted, err := dependencies.ProblemReports.Delete(ctx, input.ReportID)
	if err != nil {
		return nil, server.logInternalError(ctx, "delete a problem report", err)
	}
	if !deleted {
		return nil, errNoSuchProblemReport()
	}
	return &struct{}{}, nil
}

// errNoSuchProblemReport is a report there is not, or is no longer: 60 days on, or deleted.
func errNoSuchProblemReport() error {
	return newError(http.StatusNotFound, CodeNotFound, "there is no such problem report, or it is kept no longer")
}
