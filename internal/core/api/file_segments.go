// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// FileSegmentsOutput is a file's segments, as POST /playback gives the player them.
type FileSegmentsOutput struct {
	Body struct {
		Segments []SegmentBody `doc:"The file's segments in order, as POST /playback gives them; empty for a file with none" json:"segments"` //nolint:lll // one struct tag
	}
}

// FileSegmentsInput asks a file's segments.
type FileSegmentsInput struct {
	AuthorizationHeader
	ID int64 `path:"id"`
}

func (server *Server) registerFileSegments() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getFileSegments", Method: http.MethodGet, Path: "/files/{id}/segments",
		Summary: "A file's segments, its intro, recap, preview, credits and commercials, for a player that " +
			"does not start its playback: a cast's remote and its receivers (D546)",
		Tags:       []string{tagPlayback},
		Extensions: callableBy(capabilitySignedInOrGuest),
	}, server.getFileSegments)
}

// getFileSegments is a file's segments, as its playback's decision carries them, for a cast's remote,
// which offers the skips and Up next from the credits as the player does, and its receivers, which
// show Up next with it (D546). It is authorised as GET /files/{id}/up-next is: a guest receiver asks
// it of its own cast's film alone (D471 §1), and anyone else of a file they reach.
func (server *Server) getFileSegments(ctx context.Context, input *FileSegmentsInput) (*FileSegmentsOutput, error) {
	dependencies, principal, err := server.requireSignedInOrGuest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if principal.IsGuestReceiver() && !guestPlays(dependencies, principal, input.ID) {
		return nil, errGuestReceiver
	}
	if err = dependencies.Browser.ReachFile(ctx, principal.Reach, input.ID); err != nil {
		return nil, server.playbackError(ctx, "reach a file", err)
	}
	output := &FileSegmentsOutput{}
	if output.Body.Segments, err = segmentBodies(ctx, dependencies, input.ID); err != nil {
		return nil, server.libraryError(ctx, "read the segments of a file", err)
	}
	return output, nil
}
