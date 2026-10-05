// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/playback"
)

// ChunksMasterInput asks for the master playlist of a playback's film cut into chunks.
type ChunksMasterInput struct {
	MediaCredentials
	Token string `doc:"The playback's media token" path:"token"`
}

// ChunkFileInput asks for one file of one stream of a playback's film cut into chunks.
type ChunkFileInput struct {
	MediaCredentials
	Token      string `doc:"The playback's media token" path:"token"`
	StreamName string `doc:"picture, a quality such as q1200k480p, or a sound such as sound0 or sound0-boost" path:"streamName"` //nolint:lll // one struct tag
	FileName   string `doc:"stream.m3u8, init.mp4 or a chunk, NNNNN.m4s" path:"fileName"`
}

// streamPlaylistName is the name of a stream's playlist, which the core writes itself.
const streamPlaylistName = "stream.m3u8"

// How a request for a chunk its node has not made waits for it (chunks.md §3): it asks the node
// again every quarter second, and after 20 seconds the player is told to ask again. A quarter
// second is under a tenth of a chunk, so a chunk is relayed soon after it is made; 20 seconds is
// what a run's FFmpeg needs to start and make its first chunk on a slow machine, and under the time
// hls.js gives a fragment's load before it gives up.
const (
	chunkAskEvery   = 250 * time.Millisecond
	chunkWaitAtMost = 20 * time.Second
)

func (server *Server) registerChunks() {
	responses := map[string]*huma.Response{
		"200": {Description: "A playlist, an init or a chunk", Content: map[string]*huma.MediaType{
			"application/vnd.apple.mpegurl": {}, "video/iso.segment": {}, "video/mp4": {},
		}},
		"404": {Description: "A stream the playback is not offered, or a file a stream has no place for"},
		"425": {Description: "Not made yet: the film is being read for where it can be cut, or the chunk is " +
			"being made; ask again after Retry-After"},
	}
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getChunksMasterPlaylist", Method: http.MethodGet,
		Path:    "/bytes/media/chunks/{token}/master.m3u8",
		Summary: "The master playlist of a playback's film cut into chunks at each quality (D247 §10)",
		Tags:    []string{tagPlayback}, Responses: responses,
	}, server.getChunksMasterPlaylist)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getChunkFile", Method: http.MethodGet,
		Path: "/bytes/media/chunks/{token}/{streamName}/{fileName}",
		Summary: "One stream's playlist, or its init or a chunk, relayed from the film's node and made " +
			"there when first asked for (D247 §10)",
		Tags: []string{tagPlayback}, Responses: responses,
	}, server.getChunkFile)
}

// chunksPlayback is the playback a media token refers to, for its film's chunks: the holder the HLS
// route accepts, a Watch Together viewer of the film among them.
func (server *Server) chunksPlayback(
	ctx context.Context, credentials MediaCredentials, token string,
) (*Dependencies, chunks.Playback, error) {
	dependencies, holder, err := server.mediaHolder(ctx, credentials)
	if err != nil {
		return nil, chunks.Playback{}, err
	}
	chunked, err := dependencies.Playback.ChunksOf(ctx, token, holder)
	if err != nil {
		return nil, chunks.Playback{}, server.playbackError(ctx, "find a playback's chunks", err)
	}
	return dependencies, chunked, nil
}

// getChunksMasterPlaylist answers GET /bytes/media/chunks/{token}/master.m3u8. While the film has no
// grid it is read through, and the player is told to ask again (425).
func (server *Server) getChunksMasterPlaylist(
	ctx context.Context, input *ChunksMasterInput,
) (*huma.StreamResponse, error) {
	dependencies, chunked, err := server.chunksPlayback(ctx, input.MediaCredentials, input.Token)
	if err != nil {
		return nil, err
	}
	text, err := dependencies.Chunks.MasterPlaylist(ctx, chunked)
	if err != nil {
		return nil, server.chunksError(ctx, "write a master playlist of chunks", err)
	}
	return playlistResponse(text), nil
}

// getChunkFile answers GET /bytes/media/chunks/{token}/{streamName}/{fileName}: a stream's playlist,
// which the core writes, or its init or a chunk, relayed from the film's node and waited for while
// it is made (relayChunkFile).
func (server *Server) getChunkFile(ctx context.Context, input *ChunkFileInput) (*huma.StreamResponse, error) {
	dependencies, chunked, err := server.chunksPlayback(ctx, input.MediaCredentials, input.Token)
	if err != nil {
		return nil, err
	}
	if input.FileName == streamPlaylistName {
		text, playlistErr := dependencies.Chunks.MediaPlaylist(ctx, chunked, input.StreamName)
		if playlistErr != nil {
			return nil, server.chunksError(ctx, "write a stream's playlist of chunks", playlistErr)
		}
		return playlistResponse(text), nil
	}
	file, err := dependencies.Chunks.StoreFileOf(ctx, chunked, input.StreamName, input.FileName)
	if err != nil {
		return nil, server.chunksError(ctx, "find a chunk", err)
	}
	return server.relayChunkFile(ctx, dependencies, file)
}

// errNotMadeYet is a store's file its node does not have: not made yet.
var errNotMadeYet = errors.New("the node has not made it yet")

// relayChunkFile relays a store's init or chunk from its node. One the node has not made is asked
// for (chunks.Service.Want), and the node asked again every chunkAskEvery until it has it, for up to
// chunkWaitAtMost, after which the player is told to ask again (425). Asking again for it on each
// look keeps a run on its way however the queue took the last ask. A file the node served is known
// made. Neither ever changes once made, so it is cached for a day.
func (server *Server) relayChunkFile(
	ctx context.Context, dependencies *Dependencies, file chunks.StoreFile,
) (*huma.StreamResponse, error) {
	source := playback.Source{NodeID: file.NodeID, Target: file.Target, ContentType: file.ContentType}
	request := relayRequest{http.MethodGet, source, http.Header{}, cacheForADay}
	giveUpAt := dependencies.Clock.Now().Add(chunkWaitAtMost)
	for {
		answer, err := server.relay(ctx, dependencies, request, func() error { return errNotMadeYet })
		if !errors.Is(err, errNotMadeYet) {
			if err == nil {
				if servedErr := dependencies.Chunks.Served(ctx, file); servedErr != nil {
					return nil, server.logInternalError(ctx, "record a chunk served", servedErr)
				}
			}
			return answer, err
		}
		if err = dependencies.Chunks.Want(ctx, file); err != nil {
			return nil, server.chunksError(ctx, "have a chunk made", err)
		}
		if !dependencies.Clock.Now().Before(giveUpAt) {
			return nil, tooEarly()
		}
		if err = dependencies.Clock.Sleep(ctx, chunkAskEvery); err != nil {
			return nil, tooEarly()
		}
	}
}

// chunksError maps the chunks' refusals to API errors, and the rest as playbackError does.
func (server *Server) chunksError(ctx context.Context, action string, err error) error {
	switch {
	case errors.Is(err, chunks.ErrNoGrid):
		return tooEarly()
	case errors.Is(err, chunks.ErrNoStream):
		return newError(http.StatusNotFound, CodeNotFound, "no such stream of this playback's chunks")
	case errors.Is(err, chunks.ErrCannotBeCut):
		return newError(http.StatusNotFound, CodeNotFound, "this film cannot be cut into chunks")
	}
	return server.playbackError(ctx, action, err)
}

// playlistResponse is a playlist the core wrote. Its grid's playlists do not change, but a film read
// through again may be cut on another grid, so it is checked again each time it is asked for.
func playlistResponse(text string) *huma.StreamResponse {
	return &huma.StreamResponse{Body: func(stream huma.Context) {
		stream.SetHeader("Content-Type", "application/vnd.apple.mpegurl")
		stream.SetHeader("Cache-Control", cacheRevalidate)
		stream.SetStatus(http.StatusOK)
		// A player that stops reading ends the write; there is nothing to report to it.
		_, _ = stream.BodyWriter().Write([]byte(text)) //nolint:errcheck // see above
	}}
}
