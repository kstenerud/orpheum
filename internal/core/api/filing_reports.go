// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
)

// FilingReportBody is a report of how a work was filed, as it was written (D306 §3).
type FilingReportBody struct {
	Name     string `json:"name"`
	Path     string `doc:"Where it is on the server's disk" json:"path,omitempty"`
	Markdown string `json:"markdown"`
}

// FilingReportOutput is one report.
type FilingReportOutput struct {
	Status int
	Body   FilingReportBody
}

// FilingReportsOutput lists the reports, newest first.
type FilingReportsOutput struct {
	Body struct {
		Reports []FilingReportEntryBody `json:"reports"`
	}
}

// FilingReportEntryBody is one report in the list.
type FilingReportEntryBody struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	WrittenAt int64  `doc:"Unix milliseconds" json:"writtenAt"`
}

// FilingReportInput names a report.
type FilingReportInput struct {
	AuthorizationHeader
	Name string `path:"name"`
}

func (server *Server) registerFilingReports() {
	tags := []string{tagRunningIt}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "postFilingReport", Method: http.MethodPost, Path: "/items/{id}/filing-report", Tags: tags,
		DefaultStatus: http.StatusCreated,
		Summary: "Write a report of how a film, episode or series was filed, as Markdown on the server's disk, for " +
			"an admin to edit and post as an issue; nothing leaves the server (admin; D306 §3)",
	}, server.postFilingReport)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listFilingReports", Method: http.MethodGet, Path: "/filing-reports", Tags: tags,
		Summary: "The reports of works filed wrongly, newest first (admin; D306 §3)",
	}, server.listFilingReports)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getFilingReport", Method: http.MethodGet, Path: "/filing-reports/{name}", Tags: tags,
		Summary: "One report of a work filed wrongly (admin; D306 §3)",
	}, server.getFilingReport)
}

func (server *Server) postFilingReport(ctx context.Context, input *ItemInput) (*FilingReportOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	report, err := dependencies.FilingReports.Report(ctx, input.ID)
	if errors.Is(err, identify.ErrNothingToReport) {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such film, episode or series with a file")
	}
	if err != nil {
		return nil, server.logInternalError(ctx, "write a filing report", err)
	}
	output := &FilingReportOutput{Status: http.StatusCreated}
	output.Body = FilingReportBody{Name: report.Name, Path: report.Path, Markdown: report.Markdown}
	return output, nil
}

func (server *Server) listFilingReports(ctx context.Context, input *AuthorizationHeader) (*FilingReportsOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	listed, err := dependencies.FilingReports.List()
	if err != nil {
		return nil, server.logInternalError(ctx, "list the filing reports", err)
	}
	output := &FilingReportsOutput{}
	output.Body.Reports = make([]FilingReportEntryBody, 0, len(listed))
	for _, report := range listed {
		output.Body.Reports = append(output.Body.Reports, FilingReportEntryBody{
			Name: report.Name, SizeBytes: report.SizeBytes, WrittenAt: report.WrittenAt.UnixMilli(),
		})
	}
	return output, nil
}

func (server *Server) getFilingReport(ctx context.Context, input *FilingReportInput) (*FilingReportOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	markdown, err := dependencies.FilingReports.Read(input.Name)
	switch {
	case errors.Is(err, instancedir.ErrNotFound):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such report")
	case errors.Is(err, instancedir.ErrBadReportName):
		return nil, invalidRequestAt(http.StatusUnprocessableEntity, "path.name", "not a report's name")
	case err != nil:
		return nil, server.logInternalError(ctx, "read a filing report", err)
	}
	output := &FilingReportOutput{Status: http.StatusOK}
	output.Body = FilingReportBody{Name: input.Name, Markdown: markdown}
	return output, nil
}
