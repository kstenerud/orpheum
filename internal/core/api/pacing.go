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
		if strings.HasPrefix(request.URL.Path, Prefix+"/bytes/media/") {
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
		next.ServeHTTP(writer, request)
	})
}

// paceAsked is the pace a header asks, in kbit/s, and whether it asks one in bounds.
func paceAsked(header string) (int64, bool) {
	kbps, err := strconv.ParseInt(header, 10, 64)
	return kbps, err == nil && kbps >= paceLeastKbps && kbps <= paceMostKbps
}
