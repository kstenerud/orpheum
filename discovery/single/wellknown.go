// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// wellKnown is the prefix every signed file is served under.
const wellKnown = "/.well-known/"

// servedFor is how long a cache may keep a signed file: an hour, as an address document or a release
// record changes rarely and a core reads either again within its own look (a root statement of a
// numbered version never changes, but "latest" does).
const servedFor = "public, max-age=3600"

// WellKnownHandler serves the operator's published directory (Parts.Published) under /.well-known/,
// to be mounted at the issuer's root: the address document, the root chain, the release record and
// the publisher's chain, each beside its .sig, as `discovery-sign` wrote them (the root key audit's
// §3.1). Each is read afresh at each asking, so a newly signed one is served from the moment it is
// written, and served as it is: its signature, not this server, is what a core trusts. A path that
// leaves the directory, or names a directory, is not found.
func (discovery *Discovery) WellKnownHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		name, isUnder := strings.CutPrefix(request.URL.Path, wellKnown)
		if request.Method != http.MethodGet || !isUnder || discovery.parts.Published == "" || !fs.ValidPath(name) {
			http.NotFound(writer, request)
			return
		}
		serveFile(writer, request, discovery.parts.Published, path.Clean(name), discovery.parts.Logger)
	})
}

// serveFile serves the file at name under directory as it is, plain text: not-found when there is
// none, a failure logged when it cannot be read.
func serveFile(writer http.ResponseWriter, request *http.Request, directory, name string, logger *slog.Logger) {
	// name is fs.ValidPath's: no element leaves the directory.
	file := filepath.Join(directory, filepath.FromSlash(name))
	info, err := os.Stat(file)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.Mode().IsRegular() {
		http.NotFound(writer, request)
		return
	}
	contents, err := os.ReadFile(file) //nolint:gosec // within the operator's directory, as said
	if err != nil {
		logger.ErrorContext(request.Context(), "discovery: a signed file could not be read", "name", name, "err", err)
		http.Error(writer, "the file could not be read", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.Header().Set("Cache-Control", servedFor)
	//nolint:errcheck,gosec // a client gone is nothing to do; signed text, served as plain text
	_, _ = writer.Write(contents)
}
