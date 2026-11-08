// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/nodes"
)

// UnconfinedLeaveBody is an admin's leave for a node to run FFmpeg without the confinement its kernel
// lacks (D450 §1).
type UnconfinedLeaveBody struct {
	// AcceptedBy is the name of the admin who gave it.
	AcceptedBy string `json:"acceptedBy"`
	// AcceptedAt is when, in Unix milliseconds.
	AcceptedAt int64 `json:"acceptedAt"`
}

// UnconfinedInput names the node an admin lets run unconfined, or no longer.
type UnconfinedInput struct {
	AuthorizationHeader
	Name string `doc:"The node's configured name" minLength:"1" path:"name"`
}

func (server *Server) registerUnconfined() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "allowUnconfinedNode", Method: http.MethodPut, Path: "/nodes/{name}/unconfined",
		Tags: []string{tagRunningIt}, DefaultStatus: http.StatusNoContent,
		Summary: "Let a node whose kernel lacks confinement run FFmpeg without it, accepting the risk; " +
			"recorded with who and when (D450)",
		Extensions: callableBy(capabilityAdmin),
	}, server.allowUnconfinedNode)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "withdrawUnconfinedNode", Method: http.MethodDelete, Path: "/nodes/{name}/unconfined",
		Tags: []string{tagRunningIt}, DefaultStatus: http.StatusNoContent,
		Summary:    "Withdraw a node's leave to run without confinement: it stops and waits locked (D450)",
		Extensions: callableBy(capabilityAdmin),
	}, server.withdrawUnconfinedNode)
}

func (server *Server) allowUnconfinedNode(ctx context.Context, input *UnconfinedInput) (*struct{}, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.unconfinedAnswer(ctx, "let a node run unconfined",
		dependencies.NodeReports.AcceptUnconfined(ctx, input.Name, principal.UserID))
}

func (server *Server) withdrawUnconfinedNode(ctx context.Context, input *UnconfinedInput) (*struct{}, error) {
	dependencies, principal, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	return server.unconfinedAnswer(ctx, "withdraw a node's leave to run unconfined",
		dependencies.NodeReports.WithdrawUnconfined(ctx, input.Name, principal.UserID))
}

// unconfinedAnswer is the answer to a change of a node's leave: 404 for no such node.
func (server *Server) unconfinedAnswer(ctx context.Context, doing string, err error) (*struct{}, error) {
	if errors.Is(err, nodes.ErrNoNode) {
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such node")
	}
	if err != nil {
		return nil, server.logInternalError(ctx, doing, err)
	}
	return &struct{}{}, nil
}
