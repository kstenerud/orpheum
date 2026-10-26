// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"crypto/rand"
	"net/http"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// RequestIDHeader names, on every answer, the ID the core gave its request, which every line the
// request caused in the core's log carries as requestId (D347): a person who reports a failure can
// quote it, and the admin finds its lines.
const RequestIDHeader = "X-Request-ID"

// requestIDLength is how many of crypto/rand's base32 characters a request's ID has: 80 bits, too
// many for two requests in a log to share by chance.
const requestIDLength = 16

// identifyingRequests gives every request an ID of the core's own, never one the client sent, in
// its context, so every line logged with that context carries it, and names it on the answer.
func identifyingRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		id := rand.Text()[:requestIDLength]
		writer.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(writer, request.WithContext(slogx.WithRequestID(request.Context(), id)))
	})
}
