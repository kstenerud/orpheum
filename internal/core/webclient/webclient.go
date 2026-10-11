// SPDX-License-Identifier: AGPL-3.0-or-later

package webclient

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// notBuiltInMessage is the page a binary built without the web client serves at `/`.
const notBuiltInMessage = "This Orpheum binary was built without the web client. `make build` builds it in; " +
	"`make dev` serves it from Vite.\n"

// Handler serves files at `/`, answering the client's own addresses with index.html. files is
// nil when the binary has no web client. apiPrefix is the API's path prefix, which is never
// answered with the client.
func Handler(files fs.FS, apiPrefix string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		if isServerPath("/"+name, apiPrefix) {
			http.NotFound(writer, request)
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Allow", "GET, HEAD")
			http.Error(writer, "the web client is read with GET", http.StatusMethodNotAllowed)
			return
		}
		if files == nil {
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(notBuiltInMessage))
			return
		}
		if name == "" || !isFile(files, name) {
			if path.Ext(name) != "" {
				// A missing file with an extension is a missing file, not a screen of the client.
				http.NotFound(writer, request)
				return
			}
			name = "index.html"
		}
		if strings.HasPrefix(name, "assets/") {
			writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			writer.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFileFS(writer, request, files, name)
	})
}

// isServerPath is a path the server answers itself, never with the client.
func isServerPath(cleanPath, apiPrefix string) bool {
	for _, prefix := range []string{apiPrefix, "/debug"} {
		if cleanPath == prefix || strings.HasPrefix(cleanPath, prefix+"/") {
			return true
		}
	}
	return false
}

// isFile reports whether name is a regular file in files.
func isFile(files fs.FS, name string) bool {
	info, err := fs.Stat(files, name)
	return err == nil && info.Mode().IsRegular()
}
