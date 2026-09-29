// SPDX-License-Identifier: GPL-3.0-or-later

package byteserver

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// maxRemembered bounds the identities remembered between requests. A player's range requests
// come for one file at a time; the bound only stops a long-running node growing.
const maxRemembered = 1024

// Server answers the node's byte socket.
type Server struct {
	roots map[string]*confine.MediaRoot
	// prepared is the node's prepared directory, read as a media root is: read-only, and nothing
	// outside it. Nil on a node that does not prepare.
	prepared *confine.MediaRoot
	// served is told each rendition served, by its run directory, as its last use (D217 §5).
	served func(renditionDir string)

	mutex      sync.Mutex
	identities map[seen]string // guarded by mutex
}

// seen is what a file looked like when its identity was computed: a file whose size or time
// changed is hashed again before its bytes are served.
type seen struct {
	root, path string
	size       int64
	modified   time.Time
}

// New returns a server over the node's roots and its prepared directory, which may be nil.
func New(roots []*confine.MediaRoot, prepared *confine.MediaRoot) *Server {
	byPath := make(map[string]*confine.MediaRoot, len(roots))
	for _, root := range roots {
		byPath[root.Path()] = root
	}
	return &Server{roots: byPath, prepared: prepared, identities: make(map[seen]string)}
}

// OnRenditionServed has the server tell served each time it serves one of a rendition's files,
// naming the rendition by its run directory, <file_id>/<profile>/<run_id>: the node's record of
// its last use, by which it evicts (D217 §5).
func (server *Server) OnRenditionServed(served func(renditionDir string)) *Server {
	server.served = served
	return server
}

// Handler is the socket's HTTP handler: GET and HEAD only.
func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+nodeproto.PathBytesFile, server.file)
	mux.HandleFunc("GET "+nodeproto.PathBytesPrepared+"{name...}", server.renditionFile)
	return mux
}

// file serves nodeproto.PathBytesFile: a file's bytes, honouring Range and If-Range, once its
// payload hash is the one the core asked for.
func (server *Server) file(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	root, found := server.roots[query.Get("root")]
	rawPath, pathErr := base64.RawURLEncoding.DecodeString(query.Get("path"))
	want, hashErr := hex.DecodeString(query.Get("payloadHash"))
	switch {
	case !found:
		http.Error(writer, "the root is not one of this node's", http.StatusNotFound)
		return
	case pathErr != nil || len(rawPath) == 0 || hashErr != nil || len(want) == 0:
		http.Error(writer, "want root, path (base64url) and payloadHash (hex)", http.StatusBadRequest)
		return
	}
	file, err := root.Open(string(rawPath))
	if err != nil {
		http.Error(writer, "no such file under the root", http.StatusNotFound)
		return
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // read-only
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(writer, "not a regular file", http.StatusNotFound)
		return
	}
	identity, err := server.identity(file, info, seen{root.Path(), string(rawPath), info.Size(), info.ModTime()})
	if err != nil {
		http.Error(writer, "the file cannot be read", http.StatusInternalServerError)
		return
	}
	if identity != hex.EncodeToString(want) {
		// D100: the core named a file by its identity, and this is no longer that file.
		http.Error(writer, "the file at this path is not the one asked for", http.StatusConflict)
		return
	}
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("ETag", `"`+identity+`"`)
	http.ServeContent(writer, request, "", info.ModTime(), file)
}

// identity is the file's payload hash in hex, remembered while its size and time stay the same.
func (server *Server) identity(file *os.File, info fs.FileInfo, key seen) (string, error) {
	server.mutex.Lock()
	known, found := server.identities[key]
	server.mutex.Unlock()
	if found {
		return known, nil
	}
	computed, err := hash.Payload(file, info.Size())
	if err != nil {
		return "", errors.Join(errors.New("hash the file"), err)
	}
	identity := hex.EncodeToString(computed.Hash[:])
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if len(server.identities) >= maxRemembered {
		clear(server.identities)
	}
	server.identities[key] = identity
	return identity, nil
}

// renditionName is the only shape a rendition's file may be named by: "<file_id>/<profile>/<run_id>/" and
// then the master playlist, or one rendition's playlist, init segment or media segment.
var renditionName = regexp.MustCompile(`^[1-9]\d{0,18}/(remux|h264-8m)/[0-9a-f]{16}/` +
	`(master\.m3u8|(video|audio\d{1,3}(-boost)?)/(stream\.m3u8|init(_\d{1,3})?\.mp4|seg\d{5,9}\.m4s))$`)

// renditionFile serves one file of a prepared rendition. A name that is not a rendition's is
// refused before anything is opened; one not written yet is a 404, which the core turns into
// "too early" while the job runs.
func (server *Server) renditionFile(writer http.ResponseWriter, request *http.Request) {
	name := request.PathValue("name")
	if server.prepared == nil || !renditionName.MatchString(name) {
		http.Error(writer, "no such rendition file", http.StatusNotFound)
		return
	}
	file, err := server.prepared.Open(name)
	if err != nil {
		http.Error(writer, "no such rendition file", http.StatusNotFound)
		return
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // read-only
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(writer, "no such rendition file", http.StatusNotFound)
		return
	}
	if server.served != nil {
		parts := strings.SplitN(name, "/", 4)
		server.served(strings.Join(parts[:3], "/"))
	}
	writer.Header().Set("Content-Type", renditionType(name))
	http.ServeContent(writer, request, "", info.ModTime(), file)
}

// renditionType is a rendition file's media type, by its name.
func renditionType(name string) string {
	switch path.Ext(name) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".m4s":
		return "video/iso.segment"
	}
	return "video/mp4"
}
