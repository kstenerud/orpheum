// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/waits"
)

// PageInterestInput names the film or series whose page a session shows or left, and the season of
// a series' page that shows one.
type PageInterestInput struct {
	AuthorizationHeader
	ID     int64 `doc:"The film or series whose page it is" path:"id"`
	Season int64 `default:"-1" doc:"The season a series' page shows, whose episodes it raises, 0 for the specials; -1, the default, for a film's page or a series' own" minimum:"-1" query:"season"` //nolint:lll // one struct tag
}

func (server *Server) registerPageInterest() {
	tags := []string{tagBrowse}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "putInterest", Method: http.MethodPut, Path: "/interest/{id}",
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Say this session shows a film's or series' page, or a series' season's, which raises its " +
			"work, again every interest_said_every_seconds while it does (D324)",
	}, server.putInterest)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "deleteInterest", Method: http.MethodDelete, Path: "/interest/{id}",
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Say this session left a film's or series' page, or a series' season's, which lets its work " +
			"fall back (D324)",
	}, server.deleteInterest)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getFileWaits", Method: http.MethodGet, Path: "/files/{id}/waits", Tags: tags,
		Summary: "The work a file's viewer waits on, its read-through for its loudness and its prepared copy, " +
			"and about how long each will take (D324 §6)",
	}, server.getFileWaits)
}

// WaitBody is one piece of work a viewer waits on.
type WaitBody struct {
	IsRunning   bool   `doc:"Whether it is being done now, rather than waiting its turn" json:"isRunning"`
	SecondsLeft *int64 `doc:"About how long until it is done; absent when nothing says" json:"secondsLeft,omitempty"`
}

// FileWaitsOutput is the work a file's viewer waits on: each absent when none is open.
type FileWaitsOutput struct {
	Body struct {
		ReadThrough *WaitBody `doc:"Its read-through, for its loudness" json:"readThrough,omitempty"`
		Prepare     *WaitBody `doc:"Its prepared copy, for a device that plays one" json:"prepare,omitempty"`
	}
}

// getFileWaits answers GET /files/{id}/waits: the work a viewer of a file it reaches waits on, and
// about how long each will take.
func (server *Server) getFileWaits(ctx context.Context, input *ItemInput) (*FileWaitsOutput, error) {
	dependencies, _, err := server.requireSignedInReachingFile(ctx, input.Authorization, input.ID)
	if err != nil {
		return nil, err
	}
	found, err := dependencies.Waits.Of(ctx, input.ID)
	if err != nil {
		return nil, server.playbackError(ctx, "say how long a file's work will take", err)
	}
	output := &FileWaitsOutput{}
	output.Body.ReadThrough, output.Body.Prepare = waitBody(found.ReadThrough), waitBody(found.Prepare)
	return output, nil
}

// waitBody is a wait as the API says it; nil for none.
func waitBody(wait *waits.Wait) *WaitBody {
	if wait == nil {
		return nil
	}
	body := &WaitBody{IsRunning: wait.IsRunning}
	if wait.IsKnown {
		seconds := int64(wait.Left.Round(time.Second) / time.Second)
		body.SecondsLeft = &seconds
	}
	return body
}

// putInterest answers PUT /interest/{id}: the session shows the item's page, or its season's, which
// it reaches.
func (server *Server) putInterest(ctx context.Context, input *PageInterestInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Browser.ReachItem(ctx, principal.Reach, input.ID); err != nil {
		return nil, server.identifyError(ctx, "reach an item", err)
	}
	dependencies.Interest.Look(principal.SessionKey, principal.UserID, input.ID, input.Season)
	return &struct{}{}, nil
}

// deleteInterest answers DELETE /interest/{id}: the session left the item's page, or its season's.
// Leaving one it was not on changes nothing.
func (server *Server) deleteInterest(ctx context.Context, input *PageInterestInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	dependencies.Interest.Leave(principal.SessionKey, input.ID, input.Season)
	return &struct{}{}, nil
}
