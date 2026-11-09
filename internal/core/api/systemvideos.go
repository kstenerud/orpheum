// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/library"
)

// SystemVideoInput asks for one of the server's own videos by what it is (D435 §6).
type SystemVideoInput struct {
	AuthorizationHeader
	Purpose string `doc:"What the video is for"             enum:"soundTiming" path:"purpose"`
	Codec   string `doc:"The video and audio codecs it is in" enum:"h264-aac"   path:"codec"`
}

// SystemVideoBody is one of the server's own videos: its file, which plays as any file through
// startPlayback, once made.
type SystemVideoBody struct {
	Purpose  string `doc:"What the video is for"                                          json:"purpose"`
	Codec    string `doc:"The video and audio codecs it is in"                            json:"codec"`
	FileName string `doc:"Its file's name in the system folder"                           json:"fileName"`
	IsReady  bool   `doc:"Whether it is made and found; false while it is being made"     json:"isReady"`
	FileID   int64  `doc:"The file to play through startPlayback; 0 while it is not ready" json:"fileId"`
	// SizeBytes is the file's size, 0 while it is not ready.
	SizeBytes int64 `doc:"The file's size; 0 while it is not ready" json:"sizeBytes"`
}

// SystemVideoOutput is the video asked for.
type SystemVideoOutput struct {
	Body SystemVideoBody
}

// SystemVideosInput is the debug view's ask for the system library (D435 §7).
type SystemVideosInput struct {
	AuthorizationHeader
}

// SystemVideosOutput is the system library as the debug view shows it.
type SystemVideosOutput struct {
	Body struct {
		NodeName string            `doc:"The node whose system folder holds them" json:"nodeName"`
		Folder   string            `doc:"That folder"                             json:"folder"`
		Videos   []SystemVideoBody `doc:"Each of the server's own videos"         json:"videos"`
	}
}

func (server *Server) registerSystemVideos() {
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getSystemVideo", Method: http.MethodGet, Path: "/system-videos/{purpose}/{codec}",
		Summary:    "One of the server's own videos, by what it is, made on its first use (D435)",
		Tags:       []string{tagPlayback},
		Extensions: callableBy(capabilitySignedIn),
	}, server.getSystemVideo)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "listSystemVideos", Method: http.MethodGet, Path: "/system/videos",
		Summary:    "The server's own videos and the node that holds them, for the debug view (D435 §7)",
		Tags:       []string{tagRunningIt},
		Extensions: callableBy(capabilityAdmin),
	}, server.listSystemVideos)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "rebuildSystemVideos", Method: http.MethodPost, Path: "/system/videos/rebuild",
		Summary:       "Make the server's own videos again, over the files there (D435 §7)",
		Tags:          []string{tagRunningIt},
		DefaultStatus: http.StatusNoContent,
		Extensions:    callableBy(capabilityAdmin),
	}, server.rebuildSystemVideos)
}

func (server *Server) getSystemVideo(ctx context.Context, input *SystemVideoInput) (*SystemVideoOutput, error) {
	dependencies, _, err := server.requireSignedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	state, err := dependencies.SystemVideos.Find(ctx, input.Purpose, input.Codec)
	if err != nil {
		return nil, server.systemVideoError(ctx, "find a system video", err)
	}
	return &SystemVideoOutput{Body: systemVideoBody(state)}, nil
}

func (server *Server) listSystemVideos(ctx context.Context, input *SystemVideosInput) (*SystemVideosOutput, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	view, err := dependencies.SystemVideos.View(ctx)
	if err != nil {
		return nil, server.systemVideoError(ctx, "list the system videos", err)
	}
	output := &SystemVideosOutput{}
	output.Body.NodeName, output.Body.Folder = view.NodeName, view.Folder
	output.Body.Videos = make([]SystemVideoBody, 0, len(view.Videos))
	for _, state := range view.Videos {
		output.Body.Videos = append(output.Body.Videos, systemVideoBody(state))
	}
	return output, nil
}

func (server *Server) rebuildSystemVideos(ctx context.Context, input *SystemVideosInput) (*struct{}, error) {
	dependencies, _, err := server.requireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := dependencies.SystemVideos.Rebuild(ctx); err != nil {
		return nil, server.systemVideoError(ctx, "rebuild the system videos", err)
	}
	return &struct{}{}, nil
}

// systemVideoBody is a system video as the API answers it.
func systemVideoBody(state library.SystemVideoState) SystemVideoBody {
	return SystemVideoBody{
		Purpose: state.Video.Purpose, Codec: state.Video.Codec, FileName: state.Video.FileName,
		IsReady: state.FileID != 0, FileID: state.FileID, SizeBytes: state.SizeBytes,
	}
}

// systemVideoError maps the system videos' refusals: no node to hold them is the feature unavailable
// (D435 §5).
func (server *Server) systemVideoError(ctx context.Context, action string, err error) error {
	switch {
	case errors.Is(err, library.ErrSystemVideosUnavailable):
		return newError(http.StatusServiceUnavailable, CodeNodeSilent,
			"the server's own videos are unavailable: no connected node can make or hold them")
	case errors.Is(err, library.ErrNoSuchSystemVideo):
		return newError(http.StatusNotFound, CodeNotFound, "no such system video")
	}
	return server.libraryError(ctx, action, err)
}
