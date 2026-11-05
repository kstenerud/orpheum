// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/settings"
)

const tagJobs = "Jobs"

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
	// What it works on, one field by kind (D227 §2): an item for fetch_metadata and
	// make_artwork_sizes, a file for extract_subtitle, a fetched subtitle
	// for convert_fetched_subtitle, a file for read_through, a scan for scan, a scan's batch for
	// examine, a chunk store, one stream of a film on one grid, for make_chunks.
	ItemID            int64 `json:"itemId,omitempty"`
	StreamID          int64 `json:"streamId,omitempty"`
	FetchedSubtitleID int64 `json:"fetchedSubtitleId,omitempty"`
	FileID            int64 `json:"fileId,omitempty"`
	ScanID            int64 `json:"scanId,omitempty"`
	ScanBatchID       int64 `json:"scanBatchId,omitempty"`
	ChunkStoreID      int64 `json:"chunkStoreId,omitempty"`
	// State is a job given to a node (assigned) as running: to a person, its work runs on the
	// node, and OnNode says where it is there.
	State string `enum:"queued,running,done,failed,cancelled" json:"state"`
	// ProgressFraction is how far it is, 0 to 1, absent when not known.
	ProgressFraction *float64 `json:"progressFraction,omitempty"`
	Reason           string   `doc:"Why it exists" json:"reason"`
	LastFailure      string   `json:"lastFailure,omitempty"`
	AttemptCount     int64    `json:"attemptCount"`
	CreatedAt        int64    `doc:"Unix milliseconds" json:"createdAt"`
	FinishedAt       *int64   `doc:"Unix milliseconds" json:"finishedAt,omitempty"`
	// Destination is what it needs that can be down, when known: "tmdb-api", "node:3" (D231 §5).
	Destination string `json:"destination,omitempty"`
	// WaitingFor is that destination while the job waits on it, down: the job view says "waiting
	// for TMDB — unreachable since 14:02, next check 14:10" (D231 §12).
	WaitingFor *DestinationBody `json:"waitingFor,omitempty"`
	// OnNode is where the node running it last said it is: waiting for a free slot, running, or
	// paused while play-driven work reads its disk (D240 §6); absent for work no node holds.
	OnNode string `enum:"waiting,running,paused" json:"onNode,omitempty"`
}

// JobsInput asks for a page of the job view.
type JobsInput struct {
	AuthorizationHeader
	PageInput
}

// JobsOutput is a page of the job view: the newest jobs first, the pages ending at the newest
// job_view_max_jobs.
type JobsOutput struct {
	Body struct {
		Jobs       []JobBody `json:"jobs"`
		NextCursor string    `doc:"The next page's cursor; absent on the last" json:"nextCursor,omitempty"`
	}
}

// JobInput names a job.
type JobInput struct {
	AuthorizationHeader
	JobID int64 `minimum:"1" path:"id"`
}

func (server *Server) registerJobs() {
	tags := []string{tagJobs}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listJobs", Method: http.MethodGet, Path: "/jobs",
		Summary: "The job view: the newest jobs, what each works on, how far it is and why", Tags: tags,
		Extensions: callableBy(capabilityAdmin),
	}, server.listJobs)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "cancelJob", Method: http.MethodPost, Path: "/jobs/{id}/cancel",
		Summary: "Cancel a queued or running job; a node's work is stopped", Tags: tags,
		DefaultStatus: http.StatusNoContent,
		Extensions:    callableBy(capabilityAdmin),
	}, server.cancelJob)
}

func (server *Server) listJobs(ctx context.Context, input *JobsInput) (*JobsOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	// The cursor is the count listed so far and the last job's id: the pages end at the newest
	// job_view_max_jobs, as the view always did.
	key, beforeID, err := decodeCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	var listedBefore int64
	if input.Cursor != "" {
		var isCount bool
		if listedBefore, isCount = library.KeyInt64(key); !isCount || listedBefore < 1 || beforeID < 1 {
			return nil, errBadCursor()
		}
	}
	maxJobs, err := dependencies.Settings.Int(ctx, settings.JobViewMaxJobs)
	if err != nil {
		return nil, server.settingsError(ctx, "read job_view_max_jobs", err)
	}
	limit := min(input.Limit, maxJobs-listedBefore)
	output := &JobsOutput{}
	output.Body.Jobs = []JobBody{}
	if limit < 1 {
		return output, nil
	}
	views, err := dependencies.Jobs.List(ctx, beforeID, limit)
	if err != nil {
		return nil, server.logInternalError(ctx, "list jobs", err)
	}
	if listedBefore+int64(len(views)) < maxJobs {
		output.Body.NextCursor = nextCursorOf(len(views), limit, func() string {
			return library.EncodeCursor(library.Int64Key(listedBefore+int64(len(views))), views[len(views)-1].ID)
		})
	}
	nodeName := server.nodeNamer(ctx, dependencies)
	for _, view := range views {
		body := JobBody{
			ID: view.ID, Kind: JobKind(view.Kind), ItemID: view.ItemID,
			StreamID: view.StreamID, FetchedSubtitleID: view.FetchedSubtitleID, FileID: view.FileID,
			ScanID: view.ScanID, ScanBatchID: view.ScanBatchID, ChunkStoreID: view.ChunkStoreID,
			State: view.State, OnNode: view.OnNode, ProgressFraction: view.ProgressFraction,
			Reason: view.Reason, LastFailure: view.LastFailure, AttemptCount: view.AttemptCount,
			CreatedAt: view.CreatedAt.UnixMilli(), Destination: string(view.Destination),
		}
		if view.FinishedAt != nil {
			body.FinishedAt = new(view.FinishedAt.UnixMilli())
		}
		if view.WaitingFor != nil {
			waiting := destinationBody(*view.WaitingFor, nodeName)
			body.WaitingFor = &waiting
		}
		output.Body.Jobs = append(output.Body.Jobs, body)
	}
	return output, nil
}

func (server *Server) cancelJob(ctx context.Context, input *JobInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	err = dependencies.Jobs.Cancel(ctx, input.JobID)
	switch {
	case errors.Is(err, jobs.ErrNotOpen):
		return nil, newError(http.StatusConflict, CodeConflict, "no such job queued or running")
	case err != nil:
		return nil, server.logInternalError(ctx, "cancel a job", err)
	}
	return &struct{}{}, nil
}
