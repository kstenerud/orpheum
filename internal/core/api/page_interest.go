// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// PageInterestInput names the film or series whose page a session shows or left.
type PageInterestInput struct {
	AuthorizationHeader
	ID int64 `doc:"The film or series whose page it is" path:"id"`
}

func (server *Server) registerPageInterest() {
	tags := []string{tagBrowse}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "putInterest", Method: http.MethodPut, Path: "/interest/{id}",
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Say this session shows a film's or series' page, which raises its work, again every " +
			"interest_said_every_seconds while it does (D324)",
	}, server.putInterest)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "deleteInterest", Method: http.MethodDelete, Path: "/interest/{id}",
		DefaultStatus: http.StatusNoContent, Tags: tags,
		Summary: "Say this session left a film's or series' page, which lets its work fall back (D324)",
	}, server.deleteInterest)
}

// putInterest answers PUT /interest/{id}: the session shows the item's page, which it reaches.
func (server *Server) putInterest(ctx context.Context, input *PageInterestInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Browser.ReachItem(ctx, principal.Reach, input.ID); err != nil {
		return nil, server.identifyError(ctx, "reach an item", err)
	}
	dependencies.Interest.Look(principal.SessionKey, principal.UserID, input.ID)
	return &struct{}{}, nil
}

// deleteInterest answers DELETE /interest/{id}: the session left the item's page. Leaving one it
// was not on changes nothing.
func (server *Server) deleteInterest(ctx context.Context, input *PageInterestInput) (*struct{}, error) {
	dependencies, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	dependencies.Interest.Leave(principal.SessionKey, input.ID)
	return &struct{}{}, nil
}
