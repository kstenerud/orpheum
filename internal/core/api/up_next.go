// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/library"
)

// UpNextOutput is what plays after a file as it ends, and the member before it in its sequence (D407
// §2), each absent when there is none.
type UpNextOutput struct {
	Body struct {
		UpNext   *UpNextBody `doc:"What the player offers as the file ends; absent for a film, an episode its person's ordering leaves out, and the last" json:"upNext,omitempty"`                                                             //nolint:lll // one struct tag
		Previous *NextUpBody `doc:"The episode before it in the ordering its person watches its series in, which the player's previous plays; absent for a film, an episode the ordering leaves out, and the first" json:"previous,omitempty"` //nolint:lll // one struct tag
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
		Summary: "What plays after a file as it ends, and what before it: the next and previous episodes in " +
			"the ordering its person watches its series in",
		Tags:       []string{tagPlayback},
		Extensions: callableBy(capabilitySignedIn),
	}, server.getUpNext)
}

func (server *Server) getUpNext(ctx context.Context, input *ItemInput) (*UpNextOutput, error) {
	dependencies, principal, err := server.requireSignedInReachingFile(ctx, input.Authorization, input.ID)
	if err != nil {
		return nil, err
	}
	neighbours, err := dependencies.Browser.NeighboursOf(ctx, principal.Reach, principal.UserID, input.ID)
	if err != nil {
		return nil, server.libraryError(ctx, "read what plays after a file and before it", err)
	}
	output := &UpNextOutput{}
	if next := neighbours.Next; next != nil {
		output.Body.UpNext = &UpNextBody{
			Episode: server.nextUpBody(ctx, dependencies, next.NextUp), Ordering: next.Ordering,
			IsOnlyGroup: next.IsOnlyGroup, StillSha256: next.StillSha256,
		}
	}
	if previous := neighbours.Previous; previous != nil {
		body := server.nextUpBody(ctx, dependencies, *previous)
		output.Body.Previous = &body
	}
	return output, nil
}

// TogetherUpNextInput asks what plays after the session a viewer's membership is in plays.
type TogetherUpNextInput struct {
	AuthorizationHeader
}

// getTogetherUpNext is what plays after the session's film and before it, in its host's ordering, as
// the host's own player offers it, for a viewer whose link gives canNext (D471 §3); refused to one
// whose link does not, who is not shown Up next. A neighbour that cannot be read now is left out, as
// a guest is never told why the host's file cannot be read, and so is where the host stands in it.
func (server *Server) getTogetherUpNext(ctx context.Context, input *TogetherUpNextInput) (*UpNextOutput, error) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		return nil, err
	}
	viewer, isViewer := dependencies.Together.ViewerOf(bearerToken(input.Authorization))
	if !isViewer || viewer.FileID == 0 {
		return nil, newError(http.StatusNotFound, CodeNotFound, "nothing plays for you in a session now")
	}
	if !viewer.Rights.CanNext {
		return nil, newError(http.StatusForbidden, CodeForbidden, "your link does not let you put on another episode")
	}
	reach, err := dependencies.Auth.ReachOfUser(ctx, viewer.HostUserID)
	if err != nil {
		return nil, server.logInternalError(ctx, "read a host's reach", err)
	}
	neighbours, err := dependencies.Browser.NeighboursOf(ctx, reach, viewer.HostUserID, viewer.FileID)
	if err != nil {
		return nil, server.libraryError(ctx, "read what plays after a session's film and before it", err)
	}
	output := &UpNextOutput{}
	if next := neighbours.Next; next != nil && next.Unreadable == nil {
		output.Body.UpNext = &UpNextBody{
			Episode: guestsNextUpBody(next.NextUp), Ordering: next.Ordering,
			IsOnlyGroup: next.IsOnlyGroup, StillSha256: next.StillSha256,
		}
	}
	if previous := neighbours.Previous; previous != nil && previous.Unreadable == nil {
		body := guestsNextUpBody(*previous)
		output.Body.Previous = &body
	}
	return output, nil
}

// guestsNextUpBody is a member as a session's viewer is told of it: what it is, and nothing of where
// its host stands in it.
func guestsNextUpBody(next library.NextUp) NextUpBody {
	return NextUpBody{
		ItemID: next.TitleID, Name: next.Name, GroupNumber: next.Group, NumberInGroup: next.NumberInGroup,
		GroupName: next.GroupName, FileID: next.FileID,
	}
}

// nextUpBody is a member as the API offers it to play.
func (server *Server) nextUpBody(ctx context.Context, dependencies *Dependencies, next library.NextUp) NextUpBody {
	return NextUpBody{
		ItemID: next.TitleID, Name: next.Name, GroupNumber: next.Group, NumberInGroup: next.NumberInGroup,
		GroupName: next.GroupName,
		FileID:    next.FileID, IsPartway: next.IsPartway, PositionMs: next.PartwayAtMs,
		Unavailable: unavailableBody(next.Unreadable, server.nodeNamer(ctx, dependencies)),
	}
}
