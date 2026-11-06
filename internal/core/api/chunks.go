// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/chunks"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/playback"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
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
	StreamName string `doc:"picture, a quality such as q1200k480p, or a sound such as sound0-96k or sound0-boost-96k" path:"streamName"` //nolint:lll // one struct tag
	FileName   string `doc:"stream.m3u8, init.mp4 or a chunk, NNNNN.m4s" path:"fileName"`
	// IsSought is the player saying it loads where a seek took it, its person's or its group's, never
	// a catch-up's: a run starts there sooner (D284 §2).
	IsSought bool `doc:"The player loads where a seek took it (D284 §2)" header:"Orpheum-Sought" required:"false"`
}

// streamPlaylistName is the name of a stream's playlist, which the core writes itself.
const streamPlaylistName = "stream.m3u8"

// chunkAskEvery is how often a request held for a chunk its node has not made asks the node again
// (chunks.md §3): under a tenth of a chunk, so a chunk is relayed soon after it is made. How long it
// is held is chunk_hold_ms, by default not at all: hls.js counts a hold as the chunk's download,
// and on a held chunk took a link of megabits for one of kilobits (D250 §5).
const chunkAskEvery = 250 * time.Millisecond

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
		Extensions: callableBy(capabilityMedia, schemeSession, schemeMedia, schemeMembership),
	}, server.getChunksMasterPlaylist)
	huma.Register(server.humaAPI, huma.Operation{
		OperationID: "getChunkFile", Method: http.MethodGet,
		Path: "/bytes/media/chunks/{token}/{streamName}/{fileName}",
		Summary: "One stream's playlist, or its init or a chunk, relayed from the film's node and made " +
			"there when first asked for (D247 §10)",
		Tags: []string{tagPlayback}, Responses: responses,
		Extensions: callableBy(capabilityMedia, schemeSession, schemeMedia, schemeMembership),
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
// grid it is read through, and the player is told to ask again (425); a playback first offered its
// film cut on time keeps it so (D269 §3), and one first offered its sounds unlevelled keeps them so.
func (server *Server) getChunksMasterPlaylist(
	ctx context.Context, input *ChunksMasterInput,
) (*huma.StreamResponse, error) {
	dependencies, holder, err := server.mediaHolder(ctx, input.MediaCredentials)
	if err != nil {
		return nil, err
	}
	chunked, err := dependencies.Playback.ChunksOf(ctx, input.Token, holder)
	if err != nil {
		return nil, server.playbackError(ctx, "find a playback's chunks", err)
	}
	text, kept, err := dependencies.Chunks.MasterPlaylist(ctx, chunked)
	if err != nil {
		return nil, server.chunksError(ctx, "write a master playlist of chunks", err)
	}
	ladder, err := dependencies.Chunks.Ladder(ctx, chunked)
	if err != nil {
		return nil, server.chunksError(ctx, "find a playback's ladder", err)
	}
	dependencies.Playback.KeepLadder(input.Token, ladder)
	if kept != chunked.Kept() {
		// The playback keeps the grid it began on, though a read-through finds the film's keyframes
		// before it ends (D269 §3), and its sounds unlevelled, though the loudness is measured.
		dependencies.Playback.Keep(input.Token, holder, kept)
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
	if input.IsSought {
		file = file.AsSought()
	}
	dependencies.Playback.Fetched(input.Token, file)
	if sound, hasSound := dependencies.Playback.SoundOf(input.Token); hasSound && !file.IsSound() {
		if err = dependencies.Chunks.WantAlongside(ctx, file, sound); err != nil {
			return nil, server.chunksError(ctx, "have a jump's sound made with its picture", err)
		}
	}
	return server.relayChunkFile(ctx, dependencies, file)
}

// errNotMadeYet is a store's file its node does not have: not made yet.
var errNotMadeYet = errors.New("the node has not made it yet")

// relayChunkFile relays a store's init or chunk from its node. One the node has not made is asked
// for (chunks.Service.Want), and the node asked again every chunkAskEvery until it has it, for up to
// chunk_hold_ms, after which the player is told to ask again (425). It is asked for again on each
// look: a run asked for while another of its store waits queued, after a seek, is folded into that
// one, which makes other chunks, and only a later ask queues it once that one has started. A file
// the node served is known made. Neither ever changes once made, so it is cached for a day. A file
// waited for is logged (logChunkWait); one the node had at once is not.
func (server *Server) relayChunkFile(
	ctx context.Context, dependencies *Dependencies, file chunks.StoreFile,
) (*huma.StreamResponse, error) {
	source := playback.Source{NodeID: file.NodeID, Target: file.Target, ContentType: file.ContentType}
	request := relayRequest{http.MethodGet, source, http.Header{}, cacheForADay}
	hold, err := dependencies.Settings.Int(ctx, settings.ChunkHoldMs)
	if err != nil {
		return nil, server.settingsError(ctx, "read "+string(settings.ChunkHoldMs), err)
	}
	askedAt := dependencies.Clock.Now()
	giveUpAt := askedAt.Add(time.Duration(hold) * time.Millisecond)
	var wait *chunkWait
	for {
		answer, err := server.relay(ctx, dependencies, request, func(int) error { return errNotMadeYet })
		if !errors.Is(err, errNotMadeYet) {
			return server.relayed(ctx, dependencies, file, wait, answer, err)
		}
		if err = dependencies.Chunks.Want(ctx, file); err != nil {
			server.logChunkWait(ctx, dependencies, file, wait, chunkWaitFailed)
			return nil, server.chunksError(ctx, "have a chunk made", err)
		}
		if wait == nil {
			if unreachable := server.unreachableFilm(ctx, dependencies, file.FileID()); unreachable != nil {
				server.logChunkWait(ctx, dependencies, file, &chunkWait{askedAt: askedAt}, chunkWaitFailed)
				return nil, unreachable
			}
			wait = &chunkWait{askedAt: askedAt}
			wait.run, wait.isCovered = dependencies.Chunks.CoveringRun(file)
		}
		if !dependencies.Clock.Now().Before(giveUpAt) {
			server.logChunkWait(ctx, dependencies, file, wait, chunkWaitAskAgain)
			return nil, tooEarly()
		}
		if err = dependencies.Clock.Sleep(ctx, chunkAskEvery); err != nil {
			server.logChunkWait(ctx, dependencies, file, wait, chunkWaitPlayerGone)
			return nil, tooEarly()
		}
	}
}

// relayed ends a wait for a store's file its node answered: served, and known made, or failed.
func (server *Server) relayed(
	ctx context.Context, dependencies *Dependencies, file chunks.StoreFile, wait *chunkWait,
	answer *huma.StreamResponse, err error,
) (*huma.StreamResponse, error) {
	if err == nil {
		if servedErr := dependencies.Chunks.Served(ctx, file); servedErr != nil {
			return nil, server.logInternalError(ctx, "record a chunk served", servedErr)
		}
	}
	server.logChunkWait(ctx, dependencies, file, wait, chunkWaitOutcomeOf(err))
	return answer, err
}

// unreachableFilm is why a chunk not made yet will not be: its film's root unavailable, its node
// away or its disk not answering, so the player is told now, not "being prepared" for as long as it
// asks (D241 §7; staging, 2026-10-21, nas.example off). Nil when its film can be read.
func (server *Server) unreachableFilm(ctx context.Context, dependencies *Dependencies, fileID int64) error {
	unreadable, err := dependencies.Playback.Unreadable(ctx, fileID)
	if err != nil {
		return server.playbackError(ctx, "ask whether a chunk's film can be read", err)
	}
	if unreadable == nil {
		return nil
	}
	return mediaUnreachable(&library.UnreadableError{Unreadable: *unreadable}, server.nodeNamer(ctx, dependencies))
}

// chunkWait is a store's file a player waits for, its node not having made it when first asked:
// when the player asked, and the run that covered it just after, as far as that run had made it.
type chunkWait struct {
	askedAt   time.Time
	run       chunks.RunSpan
	isCovered bool
}

// How a wait for a file not made yet ended: relayed once made; the player told to ask again after
// chunk_hold_ms (425); the player gone before either; or failed.
const (
	chunkWaitServed     = "served"
	chunkWaitAskAgain   = "askAgain"
	chunkWaitPlayerGone = "playerGone"
	chunkWaitFailed     = "failed"
)

// chunkWaitOutcomeOf is how a wait ended whose node answered with err: served, or failed.
func chunkWaitOutcomeOf(err error) string {
	if err == nil {
		return chunkWaitServed
	}
	return chunkWaitFailed
}

// logChunkWait logs, at debug, a file a player waited for, and nothing for one not waited for (a
// nil wait): its store, the chunk it needs made, how the wait ended and how long it held the
// request, and the run that covered the chunk when it was asked for, so a chunk asked for far past
// what its run had made shows: hls.js counts a hold as transfer time and switches down, and a viewer on
// a slow link found playback unwatchable and the log could not say why (playback-in-practice.md §3,
// 2026-10-06).
func (server *Server) logChunkWait(
	ctx context.Context, dependencies *Dependencies, file chunks.StoreFile, wait *chunkWait, outcome string,
) {
	if wait == nil {
		return
	}
	attributes := []any{
		"storeDir", file.StoreDir(), "wantedChunk", file.WantedChunk(), "isInit", file.IsInit(),
		"isSought", file.IsSought(),
		"outcome", outcome, "heldMs", dependencies.Clock.Now().Sub(wait.askedAt).Milliseconds(),
		"isCovered", wait.isCovered,
	}
	if wait.isCovered {
		attributes = append(attributes, "runFirstChunk", wait.run.First, "runLastChunk", wait.run.Last,
			"runMadeThrough", wait.run.MadeThrough)
	}
	server.logger.DebugContext(ctx, "waited for a chunk not made yet",
		append(attributes, slogx.EventKey, "chunkWaited")...)
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
	case errors.Is(err, chunks.ErrCannotBeMade):
		// Asking again would fail the same way: the player stops, as for a film that cannot be
		// converted at all, and the core's log has FFmpeg's word.
		return newError(http.StatusUnprocessableEntity, CodeCannotConvert,
			"this film cannot be converted here: the server failed on it the same way each time it tried")
	case errors.Is(err, chunks.ErrNoRoomToMake):
		// 429, which nothing else on a chunk's path answers: the player stops, and says why (D250 §6).
		return newError(http.StatusTooManyRequests, CodeNoRoomToMake,
			"the machine has no room to make this quality as fast as it plays, for the other films it makes")
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
