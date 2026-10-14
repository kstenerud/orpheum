// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"
	"strings"
)

// What a page on the discovery's origin may send to the API and read from its answers (the
// discovery protocol's §3.4; discovery-core.md K10): its sign-in by header, never a cookie, so no
// credentials mode; the click's key; a byte path's range and the player's pacing; and back, a wait,
// the player's ceiling and a range's extent. X-Orpheum-Client is not exposed: it names the core's
// own client, which a page the discovery serves is not, and would take for an update.
const (
	crossOriginAllowedHeaders = "Authorization, Content-Type, Idempotency-Key, Range, Orpheum-Pace-Kbps, " +
		"Orpheum-Sought"
	crossOriginExposedHeaders = "Retry-After, Orpheum-Ceiling-Kbps, Content-Range, Accept-Ranges"
	crossOriginMethods        = "GET, HEAD, POST, PUT, PATCH, DELETE"
	crossOriginMaxAgeSeconds  = "600"
)

// crossOrigin lets a page on the client origin the core's discovery names call the API from a
// browser: that origin exactly, never a wildcard and never "null", no credentials, and Vary: Origin on
// every answer (the protocol's §3.4; S18). A preflight from any other origin, or none, is answered
// without permission; the core's own page, on its own origin, needs none.
func (server *Server) crossOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, Prefix+"/") {
			next.ServeHTTP(writer, request)
			return
		}
		writer.Header().Add("Vary", "Origin")
		origin := request.Header.Get("Origin")
		isAllowed := origin != "" && origin != "null" && origin == server.clientOriginOf()
		if isAllowed {
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Access-Control-Expose-Headers", crossOriginExposedHeaders)
		}
		if request.Method == http.MethodOptions && request.Header.Get("Access-Control-Request-Method") != "" {
			if isAllowed {
				writer.Header().Set("Access-Control-Allow-Methods", crossOriginMethods)
				writer.Header().Set("Access-Control-Allow-Headers", crossOriginAllowedHeaders)
				writer.Header().Set("Access-Control-Max-Age", crossOriginMaxAgeSeconds)
			}
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

// clientOriginOf is the origin crossOrigin allows: discoveryClientOrigin, unless a test names one.
func (server *Server) clientOriginOf() string {
	if named := server.namedClientOrigin.Load(); named != nil {
		return *named
	}
	return server.discoveryClientOrigin()
}

// discoveryClientOrigin is where the core's discovery serves its client, as the discovery named it and
// the core checked it (D293 §4); empty while there is no discovery, or it named none.
func (server *Server) discoveryClientOrigin() string {
	dependencies, err := server.readyDependencies()
	if err != nil || dependencies.Discovery == nil {
		return ""
	}
	return dependencies.Discovery.Status().ClientOrigin
}
