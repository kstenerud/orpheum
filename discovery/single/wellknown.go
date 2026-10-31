// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
)

// servedFor is how long a cache may keep a signed file it was served: an hour, as an address document
// or a release record changes rarely and a core reads either again within its own refresh.
const servedFor = "public, max-age=3600"

// serveFile serves the signed file at path as it is, JSON: not-found when there is none, a failure
// logged when it cannot be read. Its signature, not this server, is what a core trusts.
func serveFile(writer http.ResponseWriter, request *http.Request, path string, logger *slog.Logger) {
	if path == "" {
		http.NotFound(writer, request)
		return
	}
	contents, err := os.ReadFile(path) //nolint:gosec // the operator's configured file
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(writer, request)
		return
	}
	if err != nil {
		logger.ErrorContext(request.Context(), "discovery: a signed file could not be read", "path", path, "err", err)
		http.Error(writer, "the file could not be read", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", servedFor)
	_, _ = writer.Write(contents) //nolint:errcheck // a client gone is nothing to do
}
