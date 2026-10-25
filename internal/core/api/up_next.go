// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/library"
)

// UpNextOutput is what plays after a file as it ends, absent when nothing does.
type UpNextOutput struct {
	Body struct {
		UpNext *UpNextBody `doc:"What the player offers as the file ends; absent for a film, an episode its person's ordering leaves out, and the last" json:"upNext,omitempty"` //nolint:lll // one struct tag
	}
}

// UpNextBody is the member after a file's title in the ordering its person watches its series in
// (D160; plans/up-next.md), as the player offers it.
type UpNextBody struct {
	Episode NextUpBody `json:"episode"`
	// Ordering is the ordering it is in, which labels it: aired, or a provider's.
	Ordering    string `json:"ordering"`
	IsOnlyGroup bool   `doc:"Whether the ordering is one run, whose label names no group" json:"isOnlyGroup,omitempty"`
	StillSha256 string `doc:"Its still's hash: GET /bytes/art/{sourceSha256}/landscape-s.webp" json:"stillSha256,omitempty"` //nolint:lll // one struct tag
}

func (server *Server) registerUpNext() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getUpNext", Method: http.MethodGet, Path: "/files/{id}/up-next",
		Summary: "What plays after a file as it ends: the next episode in the ordering its person watches " +
			"its series in",
		Tags: []string{tagPlayback},
	}, server.getUpNext)
}

func (server *Server) getUpNext(ctx context.Context, input *ItemInput) (*UpNextOutput, error) {
	dependencies, principal, err := server.requireSignedInReachingFile(ctx, input.Authorization, input.ID)
	if err != nil {
		return nil, err
	}
	next, hasNext, err := dependencies.Browser.UpNext(ctx, principal.UserID, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "read what plays after a file", err)
	}
	output := &UpNextOutput{}
	if hasNext {
		output.Body.UpNext = &UpNextBody{
			Episode: server.nextUpBody(ctx, dependencies, next.NextUp), Ordering: next.Ordering,
			IsOnlyGroup: next.IsOnlyGroup, StillSha256: next.StillSha256,
		}
	}
	return output, nil
}

// nextUpBody is a member as the API offers it to play.
func (server *Server) nextUpBody(ctx context.Context, dependencies *Dependencies, next library.NextUp) NextUpBody {
	return NextUpBody{
		TitleID: next.TitleID, Name: next.Name, GroupNumber: next.Group, NumberInGroup: next.NumberInGroup,
		GroupName: next.GroupName,
		FileID:    next.FileID, IsPartway: next.IsPartway, PositionMs: next.PartwayAtMs,
		Unavailable: unavailableBody(next.Unreadable, server.nodeNamer(ctx, dependencies)),
	}
}
