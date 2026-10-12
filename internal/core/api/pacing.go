// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/platform/pacing"
)

// PaceHeader is the header in which a player whose person's voice is live asks for its film's
// downloads to be paced, in kbit/s: a little above the rate the quality it loads plays at, so the
// download never fills the link's queue in bursts ahead of the voice (D262 §2).
const PaceHeader = "Orpheum-Pace-Kbps"

// The pace a player may ask for: no slower than a film's lowest sound alone, no faster than any
// film's own picture. Outside it, the request is unpaced.
const (
	paceLeastKbps = 64
	paceMostKbps  = 200_000
)

// pacedMedia paces the connection of each request for a film's bytes as its PaceHeader asks, and
// marks its packets lower than best effort; one without it sets its connection back, as a player
// whose voice ended. Other requests leave their connection as it is: under HTTP/2 they share it
// with the film's.
func (server *Server) pacedMedia(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, mediaPath) {
			var err error
			if kbps, isPaced := paceAsked(request.Header.Get(PaceHeader)); isPaced {
				err = pacing.Pace(request.Context(), uint32(kbps*1000/8)) //nolint:gosec // bounded above
			} else {
				err = pacing.Unpace(request.Context())
			}
			if err != nil {
				server.logger.DebugContext(request.Context(), "could not pace a film's download", "err", err)
			}
		}
		next.ServeHTTP(server.countedMedia(writer, request), request)
	})
}

// mediaPath is where a film's bytes are asked for: under it, the route's kind, then the playback's
// media token.
const mediaPath = Prefix + "/bytes/media/"

// countedMedia is writer, counting each byte of a film it sends to its playback, by the media token
// in the request's path, for what the server's upload carries (D286 §5); writer itself for any
// other request, or before the server is ready.
func (server *Server) countedMedia(writer http.ResponseWriter, request *http.Request) http.ResponseWriter {
	route, isMedia := strings.CutPrefix(request.URL.Path, mediaPath)
	dependencies := server.dependencies.Load()
	if !isMedia || dependencies == nil || dependencies.Playback == nil {
		return writer
	}
	_, rest, _ := strings.Cut(route, "/")
	token, _, _ := strings.Cut(rest, "/")
	if token == "" {
		return writer
	}
	return &countingWriter{ResponseWriter: writer, sent: func(bytes int) {
		dependencies.Playback.Sent(token, int64(bytes))
	}}
}

// countingWriter is a response writer that says how many bytes each write sent.
type countingWriter struct {
	http.ResponseWriter
	sent func(bytes int)
}

func (counting *countingWriter) Write(bytes []byte) (int, error) {
	written, err := counting.ResponseWriter.Write(bytes)
	counting.sent(written)
	return written, err //nolint:wrapcheck // a writer's error, passed on as it is
}

// Unwrap is the writer counted, for http.ResponseController's flushes and deadlines.
func (counting *countingWriter) Unwrap() http.ResponseWriter { return counting.ResponseWriter }

// Flush flushes the writer counted, where it can: a relayed chunk is streamed as it arrives.
func (counting *countingWriter) Flush() {
	if flusher, isFlusher := counting.ResponseWriter.(http.Flusher); isFlusher {
		flusher.Flush()
	}
}

// paceAsked is the pace a header asks, in kbit/s, and whether it asks one in bounds.
func paceAsked(header string) (int64, bool) {
	kbps, err := strconv.ParseInt(header, 10, 64)
	return kbps, err == nil && kbps >= paceLeastKbps && kbps <= paceMostKbps
}
