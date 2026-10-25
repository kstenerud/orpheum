// SPDX-License-Identifier: AGPL-3.0-or-later

package webclient

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// contentSecurityPolicy is what the client's pages may load and run: everything from the server
// itself and nothing from elsewhere, the subtitle renderer's WebAssembly and the player's workers
// (from blob URLs too), media and images from blob URLs, inline styles, which React sets; no framing,
// no plugins, no <base>. The core sent no policy and no frame-ancestors (the second audit's W14,
// K19, 2026-10-13).
const contentSecurityPolicy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; " +
	"worker-src 'self' blob:; connect-src 'self'; font-src 'self' data:; manifest-src 'self'; " +
	"object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// castSDKOrigin is where Google's Cast SDKs load from, the sender's and the receiver's alike
// (developers.google.com/cast): their scripts, and the framework scripts they load in turn.
const castSDKOrigin = "https://www.gstatic.com"

// castSenderLabPath is the one page that loads Google's Cast sender SDK, the lab's cast probe
// (plans/tv-device-tests.md), opened by its own load, never moved to in place, so the client's
// other pages keep the policy that loads nothing from elsewhere.
const castSenderLabPath = "lab/cast"

// castSenderLabPolicy is contentSecurityPolicy with the Cast sender SDK's scripts and requests
// allowed: the lab's cast probe alone.
const castSenderLabPolicy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval' " + castSDKOrigin + "; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; " +
	"worker-src 'self' blob:; connect-src 'self' " + castSDKOrigin + "; font-src 'self' data:; " +
	"manifest-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// receiverDirectory is where the cast receiver's files are, beside the web client's, served under
// /cast/ (web/apps/cast-receiver).
const receiverDirectory = "cast"

// receiverPolicy is the cast receiver's: its own files and the core's media, the CAF receiver SDK's
// scripts, and the Cast platform's socket on the device itself (ws://localhost:8008), which CAF
// talks to the sender through. Nothing else from elsewhere, and no framing.
const receiverPolicy = "default-src 'self'; script-src 'self' " + castSDKOrigin + "; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; " +
	"worker-src 'self' blob:; connect-src 'self' ws://localhost:8008 " + castSDKOrigin + "; " +
	"font-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// policyFor is the content-security policy of the page or file at name, a path without its leading
// slash: the receiver's under /cast/, the cast probe's at /lab/cast, the client's everywhere else.
func policyFor(name string) string {
	switch {
	case isReceiverPath(name):
		return receiverPolicy
	case name == castSenderLabPath:
		return castSenderLabPolicy
	}
	return contentSecurityPolicy
}

// isReceiverPath is whether name, a path without its leading slash, is the cast receiver's.
func isReceiverPath(name string) bool {
	return name == receiverDirectory || strings.HasPrefix(name, receiverDirectory+"/")
}

// strictTransportSecurity tells a browser that reached the client over HTTPS to come back only over
// HTTPS, for a year: not its subdomains, which may be another's.
const strictTransportSecurity = "max-age=31536000"

// notBuiltInMessage is the page a binary built without the web client serves at `/`.
const notBuiltInMessage = "This Orpheum binary was built without the web client. `make build` builds it in; " +
	"`make dev` serves it from Vite.\n"

// Handler serves files at `/`, answering the client's own addresses with index.html. files is
// nil when the binary has no web client. apiPrefix is the API's path prefix, which is never
// answered with the client.
func Handler(files fs.FS, apiPrefix string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Content-Security-Policy", policyFor(name))
		writer.Header().Set("Referrer-Policy", "no-referrer")
		if request.TLS != nil {
			writer.Header().Set("Strict-Transport-Security", strictTransportSecurity)
		}
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
			name = indexOf(files, name)
		}
		if strings.HasPrefix(name, "assets/") {
			writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			writer.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFileFS(writer, request, files, name)
	})
}

// indexOf is the page that answers name, a screen's address rather than a file: the cast receiver's
// under /cast/ where it is built in, the web client's everywhere else.
func indexOf(files fs.FS, name string) string {
	receiverIndex := receiverDirectory + "/index.html"
	if isReceiverPath(name) && isFile(files, receiverIndex) {
		return receiverIndex
	}
	return "index.html"
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
