// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/playback"
)

const tagPlayback = "Playback"

// HopNode names the node as the leg that failed.
const HopNode Hop = "node"

// NodeBytes reads from a node's byte socket.
type NodeBytes interface {
	Do(ctx context.Context, nodeID int64, method, target string, header http.Header) (*http.Response, error)
}

// MenuBody is what a client can play, one list per axis, most preferred first (D21, D217 §1).
type MenuBody struct {
	Video      []string `doc:"Must list h264-sdr" enum:"h264-sdr,hevc-sdr,hevc-hdr10,av1-sdr,av1-hdr10" json:"video"`
	Audio      []string `doc:"Must list aac-stereo" enum:"aac-stereo,aac-multi,ac3,eac3,opus,flac" json:"audio"`
	Subtitle   []string `enum:"ass,webvtt" json:"subtitle"`
	Containers []string `doc:"Containers played as they are" enum:"mp4,mkv,mpegts" json:"containers"`
}

// StartPlaybackInput asks to play a file.
type StartPlaybackInput struct {
	AuthorizationHeader
	Body struct {
		FileID int64    `json:"fileId" minimum:"1"`
		Menu   MenuBody `json:"menu"`
		Link   string   `doc:"Where the client is" enum:"local,remote" json:"link"`
		Prefer string   `doc:"A person's override" enum:"direct,prepare" json:"prefer,omitempty" required:"false"`
	}
}

// StartPlaybackOutput is the decision (cut-a §6).
type StartPlaybackOutput struct {
	Body struct {
		Kind    string `enum:"direct,hls" json:"kind"`
		URL     string `doc:"Where the bytes are; absent until prepared renditions are built" json:"url,omitempty"`
		Profile string `enum:"remux,h264-8m" json:"profile,omitempty"`
		// Reasons say why it is not played directly, to show on screen (D188).
		Reasons []string `enum:"container,video_codec,audio_codec,bandwidth,dialogue_boost,user_choice" json:"reasons"`
	}
}

// DirectInput asks for a direct playback's bytes.
type DirectInput struct {
	MediaCredentials
	Token   string `doc:"The playback's media token" path:"token"`
	Range   string `header:"Range"`
	IfRange string `header:"If-Range"`
}

func (server *Server) registerPlayback() {
	huma.Register(server.api, huma.Operation{
		OperationID: "startPlayback", Method: http.MethodPost, Path: "/playback",
		Summary: "Decide how a file plays on this client, and where its bytes are (cut-a §6)",
		Tags:    []string{tagPlayback},
	}, server.startPlayback)
	for method, id := range map[string]string{http.MethodGet: "getDirect", http.MethodHead: "headDirect"} {
		huma.Register(server.api, huma.Operation{
			OperationID: id, Method: method, Path: "/bytes/media/direct/{token}",
			Summary: "A file's own bytes, relayed from its node, honouring Range", Tags: []string{tagPlayback},
			Responses: map[string]*huma.Response{
				"200": {Description: "The file", Content: map[string]*huma.MediaType{"application/octet-stream": {}}},
				"206": {Description: "The range asked for"},
			},
		}, func(ctx context.Context, input *DirectInput) (*huma.StreamResponse, error) {
			return server.direct(ctx, method, input)
		})
	}
}

func (server *Server) startPlayback(ctx context.Context, input *StartPlaybackInput) (*StartPlaybackOutput, error) {
	dependencies, principal, err := server.signedIn(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	request := playback.Request{
		FileID: input.Body.FileID, Menu: menuOf(input.Body.Menu),
		Link: decide.Link(input.Body.Link), Prefer: decide.Prefer(input.Body.Prefer),
	}
	started, err := dependencies.Playback.Start(ctx, principal.UserID, sessionKey(input.Authorization), request)
	switch {
	case errors.Is(err, decide.ErrMenu):
		return nil, newError(http.StatusBadRequest, CodeInvalidRequest, err.Error())
	case errors.Is(err, playback.ErrNoFile):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such file to play")
	case err != nil:
		return nil, server.internal(ctx, "start a playback", err)
	}
	output := &StartPlaybackOutput{}
	output.Body.Kind = string(started.Decision.Kind)
	output.Body.Profile = string(started.Decision.Profile)
	output.Body.Reasons = make([]string, 0, len(started.Decision.Reasons))
	for _, reason := range started.Decision.Reasons {
		output.Body.Reasons = append(output.Body.Reasons, string(reason))
	}
	if started.Decision.Kind == decide.Direct {
		output.Body.URL = Prefix + "/bytes/media/direct/" + started.Token
	}
	return output, nil
}

// direct relays a file's bytes from its node: the core never opens a media file (D193 §1).
func (server *Server) direct(ctx context.Context, method string, input *DirectInput) (*huma.StreamResponse, error) {
	authorization := input.authorization()
	dependencies, principal, err := server.signedIn(ctx, authorization)
	if err != nil {
		return nil, err
	}
	source, err := dependencies.Playback.Direct(ctx, input.Token, principal.UserID, sessionKey(authorization))
	switch {
	case errors.Is(err, playback.ErrNoToken), errors.Is(err, playback.ErrNoFile):
		return nil, newError(http.StatusNotFound, CodeNotFound, "no such playback; start it again")
	case err != nil:
		return nil, server.internal(ctx, "find a playback's bytes", err)
	}
	header := http.Header{}
	if input.Range != "" {
		header.Set("Range", input.Range)
	}
	if input.IfRange != "" {
		header.Set("If-Range", input.IfRange)
	}
	//nolint:bodyclose // closed by the stream below, or on refusal
	answer, err := dependencies.NodeBytes.Do(ctx, source.NodeID, method, source.Target, header)
	if errors.Is(err, nodes.ErrNodeSilent) {
		failure := newError(http.StatusServiceUnavailable, CodeNodeSilent,
			"the node that holds this file is not answering")
		failure.ErrorBody.Error.Hop = HopNode
		return nil, failure
	}
	if err != nil {
		return nil, server.internal(ctx, "read from the node", err)
	}
	switch answer.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
	default:
		_ = answer.Body.Close() //nolint:errcheck // the answer is refused, unread
		// The node refuses a file that is gone or no longer the one named (D100).
		return nil, newError(http.StatusNotFound, CodeFileUnavailable, "the file is no longer there as it was")
	}
	return &huma.StreamResponse{Body: func(stream huma.Context) {
		defer func() { _ = answer.Body.Close() }() //nolint:errcheck // read-only
		stream.SetHeader("Content-Type", source.ContentType)
		for _, name := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
			if value := answer.Header.Get(name); value != "" {
				stream.SetHeader(name, value)
			}
		}
		stream.SetHeader("Cache-Control", "private, no-store")
		stream.SetStatus(answer.StatusCode)
		// A player that stops reading ends the copy; there is nothing to report to it.
		_, _ = io.Copy(stream.BodyWriter(), answer.Body) //nolint:errcheck // see above
	}}, nil
}

func menuOf(body MenuBody) decide.Menu {
	return decide.Menu{
		Video: names[decide.Video](body.Video), Audio: names[decide.Audio](body.Audio),
		Subtitle: names[decide.Subtitle](body.Subtitle), Containers: names[decide.Container](body.Containers),
	}
}

func names[T ~string](listed []string) []T {
	converted := make([]T, 0, len(listed))
	for _, name := range listed {
		converted = append(converted, T(name))
	}
	return converted
}

// sessionKey names the session a request came under without keeping its credential: a media
// token is bound to it, so signing out ends the playbacks it started (D217 §2).
func sessionKey(authorization string) string {
	sum := sha256.Sum256([]byte(bearerToken(authorization)))
	return hex.EncodeToString(sum[:])
}
