// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/playback"
)

// PlaybackWantsInput is a player's word on what it wants of its film's chunks now (D309 §1): one
// picture and its sound, from a position, paused or not. It replaces what the player wanted before.
type PlaybackWantsInput struct {
	AuthorizationHeader
	Token string `doc:"The playback's media token" path:"token"`
	Body  struct {
		//nolint:lll // one struct tag
		PictureStream string `doc:"The picture's stream, as the master playlist names it; empty keeps the one wanted" json:"pictureStream,omitempty" maxLength:"64"`
		//nolint:lll // one struct tag
		SoundStream string `doc:"The sound's stream, as the master playlist names it; empty keeps the one wanted" json:"soundStream,omitempty" maxLength:"64"`
		PositionMs  int64  `doc:"Where in the film the player plays from" json:"positionMs" minimum:"0"`
		IsPaused    bool   `doc:"Paused, it wants only the minute after its position (D309 §4)" json:"isPaused"`
	}
}

func (server *Server) registerInterest() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setPlaybackWants", Method: http.MethodPut, Path: "/playback/{token}/wants",
		Summary: "Say which quality and sound the player wants, from where, and whether it is paused (D309)",
		Tags:    []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
		Extensions: callableBy(capabilityMember, schemeSession, schemeMembership),
	}, server.setPlaybackWants)
	// The old path, kept outside the document for one release (D425 §3): a player that started
	// before the core's restart keeps saying its wants there until its page reloads. It goes in the
	// next batch.
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "setPlaybackWantsAtItsOldPath", Method: http.MethodPut, Path: "/playback/{token}/interest",
		Hidden: true, DefaultStatus: http.StatusNoContent,
		Extensions: callableBy(capabilityMember, schemeSession, schemeMembership),
	}, server.setPlaybackWantsAtItsOldPath)
	server.registerPageInterest()
}

// setPlaybackWantsAtItsOldPath is setPlaybackWants at the path it had before D425 §2, for one
// release (D425 §3).
func (server *Server) setPlaybackWantsAtItsOldPath(
	ctx context.Context, input *PlaybackWantsInput,
) (*struct{}, error) {
	return server.setPlaybackWants(ctx, input)
}

func (server *Server) setPlaybackWants(ctx context.Context, input *PlaybackWantsInput) (*struct{}, error) {
	reporter, err := server.reporterOf(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	said := playback.Interest{
		PictureStream: input.Body.PictureStream, SoundStream: input.Body.SoundStream,
		PositionMs: input.Body.PositionMs, IsPaused: input.Body.IsPaused,
	}
	if err := reporter.dependencies.Playback.SayInterest(ctx, input.Token, reporter.holder, said); err != nil {
		return nil, server.playbackError(ctx, "take what a player wants", err)
	}
	return &struct{}{}, nil
}
