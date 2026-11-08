// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// slowRequestAfter is how long an API request may take before its log says so: a page waits on
// it, and a person notices a second (the owner's production NAS, 2026-10-15: a person's page took 5 to 10
// seconds, and nothing said which request).
const slowRequestAfter = time.Second

// largeResponseAfter is how many bytes an API response may hold before its log says so: a page
// draws what it is sent, and nothing grows without bound but by a fault (the owner's production NAS,
// 2026-10-16: a series' page of 5.3 MB, 80,269 episodes, that nothing named;
// design/research/data-delivery-independent-review-2026-10-16.md's MD1).
const largeResponseAfter = 1 << 20

// loggingSlow logs every API request that took longer than slowRequestAfter, with its method, path,
// how it reached the core, and the database statements and writes it made (D303, DL17), and every
// one whose response held more than largeResponseAfter bytes, with its size; a file's bytes and a
// socket, long and large by nature, are left out.
func (server *Server) loggingSlow(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		dependencies := server.dependencies.Load()
		if dependencies == nil || dependencies.Clock == nil || !isTimedRequest(request) {
			next.ServeHTTP(writer, request)
			return
		}
		ctx, tally := querytally.New(request.Context())
		started := dependencies.Clock.Now()
		size := 0
		next.ServeHTTP(&countingWriter{ResponseWriter: writer, sent: func(bytes int) { size += bytes }},
			request.WithContext(ctx))
		took := dependencies.Clock.Now().Sub(started)
		if size > largeResponseAfter {
			server.logger.WarnContext(ctx, "a response was large", "method", request.Method,
				"path", request.URL.Path, "bytes", size, "over", pathOf(ctx))
		}
		if took < slowRequestAfter {
			return
		}
		server.logger.InfoContext(ctx, "a request was slow", "method", request.Method,
			"path", request.URL.Path, "tookMs", took.Milliseconds(), "over", pathOf(ctx),
			"statements", tally.String(), "writes", tally.Writes())
	})
}

// isTimedRequest is whether a request is one a page waits on: under the API, not a file's bytes,
// and not a socket.
func isTimedRequest(request *http.Request) bool {
	path := request.URL.Path
	return strings.HasPrefix(path, Prefix+"/") && !strings.HasPrefix(path, Prefix+"/bytes/") &&
		request.Header.Get("Upgrade") == ""
}
