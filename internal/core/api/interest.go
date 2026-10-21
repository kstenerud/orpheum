// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/playback"
)

// InterestInput is a player's word on what it wants of its film's chunks now (D309 §1): one picture
// and its sound, from a position, paused or not. It replaces what the player wanted before.
type InterestInput struct {
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
		OperationID: "sayInterest", Method: http.MethodPut, Path: "/playback/{token}/interest",
		Summary: "Say which quality and sound the player wants, from where, and whether it is paused (D309)",
		Tags:    []string{tagPlayback}, DefaultStatus: http.StatusNoContent,
	}, server.sayInterest)
	server.registerPageInterest()
}

func (server *Server) sayInterest(ctx context.Context, input *InterestInput) (*struct{}, error) {
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
