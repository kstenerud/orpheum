// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/playback"
)

// LeaveInput is a player's word that it left the page of the playback its token names, the film's
// or the player's (D258 §2).
type LeaveInput struct {
	AuthorizationHeader
	Token string `doc:"The playback's media token" path:"token"`
	//nolint:lll // one struct tag
	Reason string `doc:"Why: left the page, playing nothing else, or went on to play another film (D309 §7)" enum:"left,replaced" query:"reason" required:"false"`
}

// registerLeave registers a player's words on its playback's work: that it left, and what it wants
// of its film's chunks (registerInterest).
func (server *Server) registerLeave() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "leavePlayback", Method: http.MethodDelete, Path: "/playback/{token}",
		Summary: "Say the player left the film's page or the player: its work is no longer wanted (D258 §2)",
		Tags:    []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
	}, server.leavePlayback)
	server.registerInterest()
}

func (server *Server) leavePlayback(ctx context.Context, input *LeaveInput) (*struct{}, error) {
	reporter, err := server.reporterOf(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	gone := playback.Disposition(input.Reason)
	if err := reporter.dependencies.Playback.Leave(ctx, input.Token, reporter.holder, gone); err != nil {
		return nil, server.playbackError(ctx, "let go of a playback's work", err)
	}
	server.logger.InfoContext(ctx, "a player left its playback", "reason", input.Reason,
		"userId", reporter.holder.UserID, "deviceId", reporter.deviceID, "viewerOfFileId", reporter.holder.OnlyFileID)
	return &struct{}{}, nil
}
