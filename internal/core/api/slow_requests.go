// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/querytally"
)

// slowRequestAfter is how long an API request may take before its log says so: a page waits on
// it, and a person notices a second (the owner's nas.example, 2026-10-15: a person's page took 5 to 10
// seconds, and nothing said which request).
const slowRequestAfter = time.Second

// loggingSlow logs every API request that took longer than slowRequestAfter, with its method, path,
// how it reached the core, and the database statements and writes it made (D303, DL17); a file's
// bytes and a socket, long by nature, are left out.
func (server *Server) loggingSlow(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		dependencies := server.dependencies.Load()
		if dependencies == nil || dependencies.Clock == nil || !isTimedRequest(request) {
			next.ServeHTTP(writer, request)
			return
		}
		ctx, tally := querytally.New(request.Context())
		started := dependencies.Clock.Now()
		next.ServeHTTP(writer, request.WithContext(ctx))
		took := dependencies.Clock.Now().Sub(started)
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
