// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// ClientProblemInput is a failure a page met, which it otherwise writes only to the browser's
// console: on a phone no one reads that, and a phone's playback that started over by itself left
// nothing to say why (staging, 2026-10-14). Signed in only, bounded, and only logged, as a player's
// report is; the page sends at most a few each time it loads.
type ClientProblemInput struct {
	AuthorizationHeader
	Body struct {
		Message string `doc:"What failed, in the page's own words" json:"message" maxLength:"500" minLength:"1"`
		Page    string `doc:"The page's path when it failed" json:"page" maxLength:"200"`
	}
	ClientFacts
}

func (server *Server) registerClientProblems() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "reportClientProblem", Method: http.MethodPost, Path: "/client-problems",
		Summary:       "A failure the page met, for the server's log",
		Tags:          []string{tagPlayback},
		DefaultStatus: http.StatusNoContent,
	}, server.reportClientProblem)
}

func (server *Server) reportClientProblem(ctx context.Context, input *ClientProblemInput) (*struct{}, error) {
	_, principal, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	server.logger.InfoContext(ctx, "a page reported a failure", "userId", principal.UserID,
		"deviceId", principal.DeviceID, "page", input.Body.Page, "message", input.Body.Message,
		"userAgent", input.userAgent)
	return &struct{}{}, nil
}
