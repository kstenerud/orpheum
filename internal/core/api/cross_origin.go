// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import "net/http"

// What a page on the discovery's origin may send to the API and read from its answers (the
// discovery protocol's §3.4; discovery-core.md K10): its sign-in by header, never a cookie, so no
// credentials mode; a Watch Together viewer's membership, by header beside it; the click's key; a byte
// path's range and the player's pacing; and back, a wait,
// the player's ceiling, a range's extent and the request's ID. X-Orpheum-Client is not exposed: it names the core's
// own client, which a page the discovery serves is not, and would take for an update.
const (
	crossOriginAllowedHeaders = "Authorization, Content-Type, Idempotency-Key, Range, Orpheum-Pace-Kbps, " +
		"Orpheum-Sought, " + ViewerHeaderName
	crossOriginExposedHeaders = "Retry-After, Orpheum-Ceiling-Kbps, Content-Range, Accept-Ranges, " +
		RequestIDHeader
	crossOriginMethods       = "GET, HEAD, POST, PUT, PATCH, DELETE"
	crossOriginMaxAgeSeconds = "600"
)

// crossOrigin lets a page on the client origin the core's discovery names call the API from a
// browser, and read the core's client itself, its page and files, which its loader checks and runs
// from there over whichever path reaches the core (D308 §6): that origin exactly, never a wildcard
// and never "null", no credentials, and Vary: Origin on every answer (the protocol's §3.4; S18).
// Over the core's own names the client's page was refused, and a browser whose LAN waited on its
// local-network prompt ran out of paths (staging, 2026-10-17). The loader's origin, where no core's
// client runs (D308), may call only what finding the core takes (isOpenToTheLoader). A preflight
// from any other origin, or none, is answered without permission; the core's own page, on its own
// origin, needs none.
func (server *Server) crossOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add("Vary", "Origin")
		origin := request.Header.Get("Origin")
		isAllowed := origin != "" && origin != "null" && (origin == server.clientOriginOf() ||
			origin == server.loaderOriginOf() && isOpenToTheLoader(request.URL.Path))
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

// isOpenToTheLoader is whether the loader's origin may call path: the health its path race probes,
// and the hand-off of a sign-in, which answers a code and never a session (D308 §4).
func isOpenToTheLoader(path string) bool {
	return path == Prefix+"/health" || path == Prefix+handOffPath
}

// loaderOriginOf is the loader's origin crossOrigin allows for pathsOpenToTheLoader: the discovery's,
// unless a test names one.
func (server *Server) loaderOriginOf() string {
	if named := server.namedLoaderOrigin.Load(); named != nil {
		return *named
	}
	dependencies, err := server.readyDependencies()
	if err != nil || dependencies.Discovery == nil {
		return ""
	}
	return dependencies.Discovery.Status().LoaderOrigin
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
