// SPDX-License-Identifier: AGPL-3.0-or-later

package webclient

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// compressedExtensions are the files sent gzipped to a browser that accepts it: text, which gzip
// shrinks about 3.4 times for the client's scripts (980 KB to 290 KB, the audit of 2026-10-25, R1).
// Images and fonts are compressed already. D129 chose gzip for the client channel: every browser,
// and every television's, has it.
var compressedExtensions = map[string]bool{ //nolint:gochecknoglobals // a constant set
	".js": true, ".mjs": true, ".css": true, ".html": true, ".svg": true, ".json": true, ".map": true,
	".wasm": true, ".txt": true, ".webmanifest": true,
}

// smallestCompressed is the smallest file sent gzipped: below about a packet, gzip's header and the
// encoding's headers cost about what they save. A fixed bound, not a setting: it changes how
// many bytes are sent, never what is shown.
const smallestCompressed = 1024

// gzipped are the client's files gzipped, each made the first time a browser that accepts gzip asks
// for it, and kept: the files are embedded in the binary and never change while it runs.
type gzipped struct {
	mutex sync.Mutex
	// byName is each file's gzipped bytes, or nil for one gzip does not shrink; guarded by mutex.
	byName map[string][]byte
}

// of is name's gzipped bytes, gzipped now the first time, or nil when gzip does not shrink it.
func (cache *gzipped) of(files fs.FS, name string) []byte {
	cache.mutex.Lock()
	compressed, isKnown := cache.byName[name]
	cache.mutex.Unlock()
	if isKnown {
		return compressed
	}
	original, err := fs.ReadFile(files, name)
	if err != nil {
		return nil // served as it is, which says why it cannot be
	}
	var buffer bytes.Buffer
	writer, _ := gzip.NewWriterLevel(&buffer, gzip.BestCompression) //nolint:errcheck // a valid level
	if _, err = writer.Write(original); err == nil {
		err = writer.Close()
	}
	if err == nil && buffer.Len() < len(original) {
		compressed = buffer.Bytes()
	}
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	if cache.byName == nil {
		cache.byName = map[string][]byte{}
	}
	cache.byName[name] = compressed
	return compressed
}

// isCompressible is whether name is a kind of file sent gzipped, and big enough to gain by it.
func isCompressible(files fs.FS, name string) bool {
	if !compressedExtensions[strings.ToLower(path.Ext(name))] {
		return false
	}
	info, err := fs.Stat(files, name)
	return err == nil && info.Size() >= smallestCompressed
}

// acceptsGzip is whether an Accept-Encoding header accepts gzip: named, or by "*", with a weight
// above zero.
func acceptsGzip(header string) bool {
	for _, coding := range strings.Split(header, ",") {
		name, parameters, _ := strings.Cut(strings.TrimSpace(coding), ";")
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "gzip" && name != "*" {
			continue
		}
		weight := strings.ReplaceAll(strings.ToLower(parameters), " ", "")
		if !strings.HasPrefix(weight, "q=0") || strings.Trim(strings.TrimPrefix(weight, "q=0"), ".0") != "" {
			return true
		}
	}
	return false
}

// serveFile serves name: gzipped, with Content-Encoding and Vary saying so, to a browser that
// accepts it when gzip shrinks the file; as it is otherwise.
func (cache *gzipped) serveFile(writer http.ResponseWriter, request *http.Request, files fs.FS, name string) {
	if !isCompressible(files, name) {
		http.ServeFileFS(writer, request, files, name)
		return
	}
	writer.Header().Add("Vary", "Accept-Encoding")
	compressed := []byte(nil)
	if acceptsGzip(request.Header.Get("Accept-Encoding")) {
		compressed = cache.of(files, name)
	}
	if compressed == nil {
		http.ServeFileFS(writer, request, files, name)
		return
	}
	writer.Header().Set("Content-Encoding", "gzip")
	// ServeContent types the answer by name's extension, and serves a range of the gzipped bytes.
	http.ServeContent(writer, request, name, time.Time{}, bytes.NewReader(compressed))
}
