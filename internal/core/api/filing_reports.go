// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/identify"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/library"
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

// FilingReportsInput asks for a page of the reports.
type FilingReportsInput struct {
	AuthorizationHeader
	PageInput
}

// FilingReportsOutput is a page of the reports, newest first.
type FilingReportsOutput struct {
	Body struct {
		Reports    []FilingReportEntryBody `json:"reports"`
		NextCursor string                  `doc:"The next page's cursor; absent on the last" json:"nextCursor,omitempty"` //nolint:lll // one struct tag
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
		OperationID: "writeFilingReport", Method: http.MethodPost, Path: "/items/{id}/filing-report", Tags: tags,
		DefaultStatus: http.StatusCreated,
		Summary: "Write a report of how a film, episode or series was filed, as Markdown on the server's disk, for " +
			"an admin to edit and post as an issue; nothing leaves the server (D306 §3)",
		Extensions: callableBy(capabilityAdmin),
	}, server.writeFilingReport)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listFilingReports", Method: http.MethodGet, Path: "/filing-reports", Tags: tags,
		Summary:    "The reports of works filed wrongly, newest first (D306 §3)",
		Extensions: callableBy(capabilityAdmin),
	}, server.listFilingReports)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getFilingReport", Method: http.MethodGet, Path: "/filing-reports/{name}", Tags: tags,
		Summary:    "One report of a work filed wrongly (D306 §3)",
		Extensions: callableBy(capabilityAdmin),
	}, server.getFilingReport)
}

func (server *Server) writeFilingReport(ctx context.Context, input *ItemInput) (*FilingReportOutput, error) {
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

func (server *Server) listFilingReports(ctx context.Context, input *FilingReportsInput) (*FilingReportsOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	// The cursor is the last report's name and when it was written, Unix nanoseconds: the reports
	// are files, listed whole and paged here, newest first and by name backwards among equals.
	afterName, afterWritten, err := decodeCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	listed, err := dependencies.FilingReports.List()
	if err != nil {
		return nil, server.logInternalError(ctx, "list the filing reports", err)
	}
	if input.Cursor != "" {
		listed = slices.DeleteFunc(listed, func(report instancedir.ReportFile) bool {
			written := report.WrittenAt.UnixNano()
			return written > afterWritten || (written == afterWritten && report.Name >= string(afterName))
		})
	}
	listed = listed[:min(int64(len(listed)), input.Limit)]
	output := &FilingReportsOutput{}
	output.Body.NextCursor = nextCursorOf(len(listed), input.Limit, func() string {
		last := listed[len(listed)-1]
		return library.EncodeCursor([]byte(last.Name), last.WrittenAt.UnixNano())
	})
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
