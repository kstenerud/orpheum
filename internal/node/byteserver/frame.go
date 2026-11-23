// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/kstenerud/orpheum/internal/node/artwork"
	"github.com/kstenerud/orpheum/internal/node/disc"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// Films makes the film a run reads from a file the server opened: execx.Runner's FilmOf, which
// streams it when the node's FFmpeg can be streamed one (D330).
type Films interface {
	FilmOf(ctx context.Context, path string, opened execx.OpenedFilm, reading execx.FilmReading) (execx.Film, error)
}

// Frames makes a key frame of a film: artwork.Deriver's KeyFrame.
type Frames interface {
	KeyFrame(ctx context.Context, film execx.Film, streamIndex int, at time.Duration) ([]byte, error)
}

// ServingFrames has the server make a key frame of a video when asked (nodeproto.PathBytesFrame,
// D545), each from a film films makes of the file it opened.
func (server *Server) ServingFrames(films Films, frames Frames) *Server {
	server.films, server.frames = films, frames
	return server
}

// serveFrame serves nodeproto.PathBytesFrame: the keyframe at or before a place of a video's picture
// stream, as a WebP, made at once, outside every queue, as a file's bytes are served (D545). The file
// is opened as serveFile opens one, through its disk's gate and checked to be the one named (D100),
// and read as every run reads a film. One frame is made at a time: another ask waits its turn for as
// long as its asker waits. A video with no picture there is 404.
func (server *Server) serveFrame(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	streamIndex, indexErr := strconv.Atoi(query.Get(nodeproto.QueryFrameStreamIndex))
	atMs, atErr := strconv.ParseInt(query.Get(nodeproto.QueryFrameAtMs), 10, 64)
	if indexErr != nil || atErr != nil || streamIndex < 0 || atMs < 0 {
		http.Error(writer, "want streamIndex and atMs, whole numbers from 0", http.StatusBadRequest)
		return
	}
	if server.frames == nil {
		http.Error(writer, "this node makes no frames", http.StatusNotFound)
		return
	}
	select {
	case server.frameSlot <- struct{}{}:
		defer func() { <-server.frameSlot }()
	case <-request.Context().Done():
		return // the asker stopped waiting; nothing is answered to no one
	}
	named := namedIn(query)
	file, facts, err := server.openNamed(request.Context(), named)
	if refusedOpen(writer, named, err) {
		return
	}
	defer readonly.Close(file)
	// The file opened, so its root path is a root's own or a library folder's real path inside one.
	path := named.RootPath + "/" + string(named.Path)
	isDiscImage := disc.IsImageName(string(named.Path))
	film := execx.Film{Path: path, Size: facts.Size()}
	if server.films != nil {
		// One frame is the index and a stretch to the keyframe: read as a probe reads a film.
		film, err = server.films.FilmOf(request.Context(), path, execx.OpenedFilm{
			File: file, Facts: facts, Disks: server.disks, Root: server.gateRootOf(named.RootPath),
			Window: server.openWindow, IsDiscImage: isDiscImage,
		}, execx.ReadsIndex)
	} else if isDiscImage {
		err = execx.ErrDiscNeedsStreaming
	}
	if err != nil {
		server.warnFrame(request.Context(), "the film could not be read", named, err)
		http.Error(writer, "the film cannot be read", http.StatusInternalServerError)
		return
	}
	frame, err := server.frames.KeyFrame(request.Context(), film, streamIndex, time.Duration(atMs)*time.Millisecond)
	switch {
	case errors.Is(err, artwork.ErrNoKeyFrame):
		http.Error(writer, "the video has no picture at or before the place", http.StatusNotFound)
		return
	case err != nil:
		server.warnFrame(request.Context(), "FFmpeg did not make it", named, err)
		http.Error(writer, "the key frame could not be made", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "image/webp")
	// The node's own WebP (D212 §1), on its private socket, to the core.
	_, _ = writer.Write(frame) //nolint:errcheck // a core that hung up asks again
}

// warnFrame logs a key frame that failed, and what failed, naming the file by its root and payload
// hash alone (principles §12: its path may name a film).
func (server *Server) warnFrame(ctx context.Context, failed string, named nodeproto.BytesFile, err error) {
	if server.logger != nil {
		server.logger.WarnContext(ctx, "a key frame failed", slogx.EventKey, "keyFrameFailed", "failed", failed,
			"root", named.RootPath, "payloadHash", named.PayloadHash, "err", err)
	}
}
