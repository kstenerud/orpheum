// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/playback"
)

// NowPlayingBody is one playback heard from lately, for the admin (cut-a §5, Running it).
type NowPlayingBody struct {
	ID         int64  `doc:"Names the playback to stop it" json:"id"`
	UserID     int64  `json:"userId"`
	UserName   string `json:"userName"`
	DeviceName string `doc:"Absent for a session with no device" json:"deviceName,omitempty"`
	FileID     int64  `json:"fileId"`
	TitleID    int64  `doc:"Absent for a file filed as no title" json:"titleId,omitempty"`
	Name       string `doc:"The title's name, or the file's path when it has none" json:"name"`
	Delivery   string `doc:"direct or hls" json:"delivery"`
	Profile    string `doc:"The prepared copy's profile; absent for direct play" json:"profile,omitempty"`
	// NotDirectReasons say why a prepared copy plays (D188).
	NotDirectReasons []string `json:"notDirectReasons"`
	PositionMs       int64    `doc:"Where its last report said it was" json:"positionMs"`
	StartedAt        int64    `doc:"Unix milliseconds" json:"startedAt"`
	LastHeardAt      int64    `doc:"Unix milliseconds" json:"lastHeardAt"`
}

// NowPlayingOutput is every playback heard from lately, oldest first.
type NowPlayingOutput struct {
	Body struct {
		Playbacks []NowPlayingBody `json:"playbacks"`
	}
}

// StopPlaybackInput is the admin stopping a playback, with a message its player shows.
type StopPlaybackInput struct {
	AuthorizationHeader
	PlaybackID int64 `minimum:"1" path:"id"`
	Body       struct {
		Message string `json:"message" maxLength:"500"`
	}
}

func (server *Server) registerNowPlaying() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getNowPlaying", Method: http.MethodGet, Path: "/now-playing", Tags: []string{tagRunningIt},
		Summary: "Every playback heard from lately: who, on which device, what, and how (admin)",
	}, server.getNowPlaying)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "stopPlayback", Method: http.MethodPost, Path: "/now-playing/{id}/stop",
		Tags: []string{tagRunningIt}, DefaultStatus: http.StatusNoContent,
		Summary: "Stop a playback, with a message its player shows within one report interval (admin; D243 §9)",
	}, server.stopPlayback)
}

func (server *Server) getNowPlaying(ctx context.Context, input *AuthorizationHeader) (*NowPlayingOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	accounts, err := dependencies.Auth.Accounts(ctx)
	if err != nil {
		return nil, server.authError(ctx, "list accounts", err)
	}
	nameOf := map[int64]string{}
	for _, account := range accounts {
		nameOf[account.ID] = account.Name
	}
	output := &NowPlayingOutput{}
	output.Body.Playbacks = []NowPlayingBody{}
	for _, playing := range dependencies.Playback.NowPlaying() {
		body, bodyErr := server.nowPlayingBody(ctx, dependencies, playing)
		if bodyErr != nil {
			return nil, bodyErr
		}
		body.UserName = nameOf[playing.UserID]
		output.Body.Playbacks = append(output.Body.Playbacks, body)
	}
	return output, nil
}

// nowPlayingBody is one playback as the admin's list shows it.
func (server *Server) nowPlayingBody(
	ctx context.Context, dependencies *Dependencies, playing playback.NowPlaying,
) (NowPlayingBody, error) {
	body := NowPlayingBody{
		ID: playing.PlaybackID, UserID: playing.UserID, FileID: playing.FileID,
		Delivery: string(playing.Decision.Delivery), Profile: string(playing.Decision.Profile),
		NotDirectReasons: make([]string, 0, len(playing.Decision.NotDirectReasons)),
		PositionMs:       playing.PositionMs, StartedAt: playing.StartedAt.UnixMilli(),
		LastHeardAt: playing.LastHeard.UnixMilli(),
	}
	for _, reason := range playing.Decision.NotDirectReasons {
		body.NotDirectReasons = append(body.NotDirectReasons, string(reason))
	}
	// The admin's now playing, who reaches every library.
	work, err := dependencies.Browser.WorkOfFile(ctx, auth.EveryLibrary(), playing.FileID)
	if err != nil {
		return body, server.libraryError(ctx, "read what a playback plays", err)
	}
	body.TitleID, body.Name = work.TitleID, work.Name
	if body.Name == "" {
		body.Name = work.Path
	}
	if playing.DeviceID != 0 {
		if device, deviceErr := dependencies.Auth.DeviceOf(ctx, playing.UserID, playing.DeviceID); deviceErr == nil {
			body.DeviceName = device.Name
		}
	}
	return body, nil
}

func (server *Server) stopPlayback(ctx context.Context, input *StopPlaybackInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err = dependencies.Playback.Stop(input.PlaybackID, input.Body.Message); err != nil {
		return nil, server.playbackError(ctx, "stop a playback", err)
	}
	return &struct{}{}, nil
}
