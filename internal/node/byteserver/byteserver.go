// SPDX-License-Identifier: GPL-3.0-or-later

package byteserver

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"os"
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

// New returns a server over the node's roots.
func New(roots []*confine.MediaRoot) *Server {
	byPath := make(map[string]*confine.MediaRoot, len(roots))
	for _, root := range roots {
		byPath[root.Path()] = root
	}
	return &Server{roots: byPath, identities: make(map[seen]string)}
}

// Handler is the socket's HTTP handler: GET and HEAD only.
func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+nodeproto.PathBytesFile, server.file)
	return mux
}

// file serves nodeproto.PathBytesFile: a file's bytes, honouring Range and If-Range, once its
// payload hash is the one the core asked for.
func (server *Server) file(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	root, found := server.roots[query.Get("root")]
	rawPath, pathErr := base64.RawURLEncoding.DecodeString(query.Get("path"))
	want, hashErr := hex.DecodeString(query.Get("hash"))
	switch {
	case !found:
		http.Error(writer, "the root is not one of this node's", http.StatusNotFound)
		return
	case pathErr != nil || len(rawPath) == 0 || hashErr != nil || len(want) == 0:
		http.Error(writer, "want root, path (base64url) and hash (hex)", http.StatusBadRequest)
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
