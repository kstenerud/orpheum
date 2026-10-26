// SPDX-License-Identifier: AGPL-3.0-or-later

package webclient_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kstenerud/orpheum/internal/core/webclient"
)

// getAccepting is a GET of target whose Accept-Encoding is acceptEncoding, "" for none.
func getAccepting(t *testing.T, handler http.Handler, target, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	if acceptEncoding != "" {
		request.Header.Set("Accept-Encoding", acceptEncoding)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// A browser that accepts gzip is sent the client's scripts gzipped, about a third of their bytes,
// which it unpacks to the file itself (D129; the audit of 2026-10-25, R1: nothing was compressed).
// One that does not, or refuses it, is sent the file as it is; an image, compressed already, and a
// file too small to gain are sent as they are to every browser.
func TestHandler_SendsTextGzippedToABrowserThatAcceptsIt(t *testing.T) {
	t.Parallel()
	script := []byte(strings.Repeat("function play(film) { return film.chunks.map(chunk => chunk.url); }\n", 100))
	files := builtClientFS()
	files["assets/client-9f8e.js"] = &fstest.MapFile{Data: script}
	files["assets/poster-1c2d.png"] = &fstest.MapFile{Data: bytes.Repeat([]byte{0x89}, 4096)}
	handler := webclient.Handler(files, "/api/v1")
	sent := getAccepting(t, handler, "/assets/client-9f8e.js", "br, gzip, deflate")
	if sent.Header().Get("Content-Encoding") != "gzip" || sent.Header().Get("Vary") != "Accept-Encoding" ||
		!strings.HasPrefix(sent.Header().Get("Content-Type"), "text/javascript") || sent.Body.Len()*3 > len(script) {
		t.Fatalf("a script to a browser accepting gzip: %v, %d bytes of %d; want it gzipped, a third or less",
			sent.Header(), sent.Body.Len(), len(script))
	}
	reader, err := gzip.NewReader(sent.Body)
	if err != nil {
		t.Fatal(err)
	}
	if unpacked, err := io.ReadAll(reader); err != nil || !bytes.Equal(unpacked, script) {
		t.Errorf("the gzipped script unpacks to %d bytes (%v); want the script itself", len(unpacked), err)
	}
	for _, refusing := range []string{"", "identity", "gzip;q=0", "br, gzip; q=0.0"} {
		plain := getAccepting(t, handler, "/assets/client-9f8e.js", refusing)
		if plain.Header().Get("Content-Encoding") != "" || !bytes.Equal(plain.Body.Bytes(), script) ||
			plain.Header().Get("Vary") != "Accept-Encoding" {
			t.Errorf("a script to a browser accepting %q: %v; want it as it is, varying by Accept-Encoding",
				refusing, plain.Header())
		}
	}
	for _, kept := range []string{"/assets/poster-1c2d.png", "/assets/app-1a2b.js"} {
		if got := getAccepting(t, handler, kept, "gzip"); got.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s to a browser accepting gzip was sent %q; want it as it is", kept,
				got.Header().Get("Content-Encoding"))
		}
	}
}
