// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/jobs"
)

const tagJobs = "Jobs"

// jobViewLimit is how many of the newest jobs the job view lists.
const jobViewLimit = 200

// JobKind is a job's kind, documented as one of jobs.Kinds, so the API names every kind the
// queue has without a second list to keep in step.
type JobKind string

// Schema is a job kind's schema: a string, one of jobs.Kinds.
func (JobKind) Schema(huma.Registry) *huma.Schema {
	kinds := jobs.Kinds()
	names := make([]any, 0, len(kinds))
	for _, kind := range kinds {
		names = append(names, string(kind))
	}
	return &huma.Schema{Type: huma.TypeString, Enum: names}
}

// JobBody is one job as the job view shows it (cut-a §5.3).
type JobBody struct {
	ID   int64   `json:"id"`
	Kind JobKind `json:"kind"`
	// What it works on, one field by kind (D227 §2): an item for fetch and artwork, a rendition
	// for prepare, a stream for extract_subtitle, a fetched subtitle for convert_fetched_subtitle,
	// a file for measure_loudness.
	ItemID            int64  `json:"itemId,omitempty"`
	RenditionID       int64  `json:"renditionId,omitempty"`
	StreamID          int64  `json:"streamId,omitempty"`
	FetchedSubtitleID int64  `json:"fetchedSubtitleId,omitempty"`
	FileID            int64  `json:"fileId,omitempty"`
	State             string `enum:"queued,running,done,failed,cancelled" json:"state"`
	// ProgressFraction is how far it is, 0 to 1, absent when not known.
	ProgressFraction *float64 `json:"progressFraction,omitempty"`
	Reason           string   `doc:"Why it exists" json:"reason"`
	LastError        string   `json:"lastError,omitempty"`
	Attempts         int64    `json:"attempts"`
	CreatedAt        int64    `doc:"Unix milliseconds" json:"createdAt"`
	FinishedAt       *int64   `doc:"Unix milliseconds" json:"finishedAt,omitempty"`
}

// JobsOutput is the job view: the newest jobs first.
type JobsOutput struct {
	Body struct {
		Jobs []JobBody `json:"jobs"`
	}
}

// JobInput names a job.
type JobInput struct {
	AuthorizationHeader
	JobID int64 `minimum:"1" path:"id"`
}

func (server *Server) registerJobs() {
	tags := []string{tagJobs}
	huma.Register(server.api, huma.Operation{
		OperationID: "listJobs", Method: http.MethodGet, Path: "/jobs",
		Summary: "The job view: the newest jobs, what each works on, how far it is and why (admin)", Tags: tags,
	}, server.listJobs)
	huma.Register(server.api, huma.Operation{
		OperationID: "cancelJob", Method: http.MethodPost, Path: "/jobs/{id}/cancel",
		Summary: "Cancel a queued or running job; a node's work is stopped (admin)", Tags: tags,
		DefaultStatus: http.StatusNoContent,
	}, server.cancelJob)
}

func (server *Server) listJobs(ctx context.Context, input *AuthorizationHeader) (*JobsOutput, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	views, err := dependencies.Jobs.List(ctx, jobViewLimit)
	if err != nil {
		return nil, server.internal(ctx, "list jobs", err)
	}
	output := &JobsOutput{}
	output.Body.Jobs = make([]JobBody, 0, len(views))
	for _, view := range views {
		body := JobBody{
			ID: view.ID, Kind: JobKind(view.Kind), ItemID: view.ItemID, RenditionID: view.RenditionID,
			StreamID: view.StreamID, FetchedSubtitleID: view.FetchedSubtitleID, FileID: view.FileID,
			State: view.State, ProgressFraction: view.Progress,
			Reason: view.Reason, LastError: view.LastError, Attempts: view.Attempts,
			CreatedAt: view.CreatedAt.UnixMilli(),
		}
		if view.FinishedAt != nil {
			body.FinishedAt = new(view.FinishedAt.UnixMilli())
		}
		output.Body.Jobs = append(output.Body.Jobs, body)
	}
	return output, nil
}

func (server *Server) cancelJob(ctx context.Context, input *JobInput) (*struct{}, error) {
	dependencies, _, err := server.signedInAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Jobs.Cancel(ctx, input.JobID)
	switch {
	case errors.Is(err, jobs.ErrNotOpen):
		return nil, newError(http.StatusConflict, CodeConflict, "no such job queued or running")
	case err != nil:
		return nil, server.internal(ctx, "cancel a job", err)
	}
	return &struct{}{}, nil
}
