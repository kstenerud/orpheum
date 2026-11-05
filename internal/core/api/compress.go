// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/platform/httpgzip"
)

// compressedOperations are the operations whose answers are sent gzipped to a client that accepts
// it (D342): lists, titles, search and the admin's views, which carry no secret. An answer that
// holds a credential (a session, a device's, a pairing's, a link's, a token) is never compressed,
// so that no request-controlled text beside it lets an observer of the compressed size guess it
// (BREACH). TestArch_NoCompressedOperationAnswersWithACredential holds every one listed here to it.
var compressedOperations = []string{ //nolint:gochecknoglobals // a constant list
	// Browsing.
	"getTitles", "getItem", "getGroup", "listNotInOrder", "search", "getHome",
	"getLibraries", "getUpNext",
	// Identification and the scanner's view.
	"getItemsByIdentification", "getItemCandidates", "getFiles", "getSubtitleCandidates",
	// The admin's views.
	"getSystem", "getDoctor", "getNodes", "listJobs", "getConfig", "getSettings", "getNowPlaying",
	"listFilingReports", "getFilingReport", "getNeedsAttention", "listUsers", "listRoles",
	// What every client reads.
	"getClientSettings",
}

// documentedOperation is one operation the API document holds.
type documentedOperation struct{ id, method, path string }

// operationsIn is every operation document holds, its path without Prefix.
func operationsIn(document *huma.OpenAPI) []documentedOperation {
	var operations []documentedOperation
	for path, item := range document.Paths {
		for method, operation := range map[string]*huma.Operation{
			http.MethodGet: item.Get, http.MethodPost: item.Post, http.MethodPut: item.Put,
			http.MethodDelete: item.Delete, http.MethodHead: item.Head, http.MethodPatch: item.Patch,
		} {
			if operation != nil {
				operations = append(operations, documentedOperation{operation.OperationID, method, path})
			}
		}
	}
	return operations
}

// smallestCompressedAnswer is the smallest answer sent gzipped: below about a packet, gzip's header
// and the encoding's headers cost about what they save. A fixed bound, not a setting: it changes how
// many bytes are sent, never what is shown.
const smallestCompressedAnswer = 1024

// compressedRoutes match the requests whose answers are sent gzipped, the routes of
// compressedOperations, as the mux matches them.
func (server *Server) compressedRoutes() *http.ServeMux {
	routes := http.NewServeMux()
	for _, operation := range operationsIn(server.humaAPI.OpenAPI()) {
		if slices.Contains(compressedOperations, operation.id) {
			routes.Handle(operation.method+" "+Prefix+operation.path, http.NotFoundHandler())
		}
	}
	return routes
}

// compressing sends the answer of an operation of compressedOperations gzipped, when the client
// accepts gzip, the answer succeeded and is JSON of at least smallestCompressedAnswer bytes; every
// such answer varies by Accept-Encoding. A HEAD, which has no body, and any other request pass
// through untouched.
func (server *Server) compressing(next http.Handler) http.Handler {
	routes := server.compressedRoutes()
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, pattern := routes.Handler(request); pattern == "" {
			next.ServeHTTP(writer, request)
			return
		}
		writer.Header().Add("Vary", "Accept-Encoding")
		if request.Method == http.MethodHead || !httpgzip.Accepts(request.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(writer, request)
			return
		}
		held := &heldAnswer{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(held, request)
		if err := held.send(); err != nil {
			server.logger.DebugContext(request.Context(), "a compressed answer's client went away", "err", err)
		}
	})
}

// heldAnswer holds an answer whole, to send it gzipped or as it is once it is known.
type heldAnswer struct {
	http.ResponseWriter

	status      int
	wroteHeader bool
	body        bytes.Buffer
}

// WriteHeader holds the status.
func (held *heldAnswer) WriteHeader(status int) {
	if !held.wroteHeader {
		held.status, held.wroteHeader = status, true
	}
}

// Write holds the body.
func (held *heldAnswer) Write(data []byte) (int, error) {
	held.wroteHeader = true
	return held.body.Write(data) //nolint:wrapcheck // a buffer's write never fails
}

// send sends the answer held: gzipped when it is a success, JSON, and big enough to gain by it.
func (held *heldAnswer) send() error {
	header := held.Header()
	body := held.body.Bytes()
	if held.status == http.StatusOK && len(body) >= smallestCompressedAnswer && isJSON(header.Get("Content-Type")) &&
		header.Get("Content-Encoding") == "" {
		if compressed := gzipped(body); compressed != nil {
			header.Set("Content-Encoding", "gzip")
			header.Del("Content-Length")
			body = compressed
		}
	}
	held.ResponseWriter.WriteHeader(held.status)
	_, err := held.ResponseWriter.Write(body)
	return err //nolint:wrapcheck // the connection's own error, only logged
}

// isJSON is whether a Content-Type is JSON's, huma's "application/json" or a problem's.
func isJSON(contentType string) bool {
	media, _, _ := strings.Cut(contentType, ";")
	media = strings.TrimSpace(strings.ToLower(media))
	return media == "application/json" || strings.HasSuffix(media, "+json")
}

// gzipped is body gzipped at the fastest level, or nil when gzip does not shrink it.
func gzipped(body []byte) []byte {
	var buffer bytes.Buffer
	writer, _ := gzip.NewWriterLevel(&buffer, gzip.BestSpeed) //nolint:errcheck // a valid level
	if _, err := writer.Write(body); err != nil || writer.Close() != nil || buffer.Len() >= len(body) {
		return nil
	}
	return buffer.Bytes()
}
