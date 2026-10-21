// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver

import (
	"context"
	"encoding/base64"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/namedfile"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/readonly"
)

// Server answers the node's byte socket.
type Server struct {
	roots map[string]*confine.MediaRoot
	// prepared is the node's prepared directory, read as a media root is: read-only, and nothing
	// outside it. Nil on a node that does not prepare.
	prepared *confine.MediaRoot
	// renditionServedHook is told each rendition served, by its run directory, as its last use
	// (D217 §5).
	renditionServedHook func(renditionDir string)
	// payloadHashes remembers each file's payload hash, so a player's range requests hash it once.
	payloadHashes *namedfile.Hashes
	// results are the results the node holds, too large to post; nil serves none.
	results Results
	// disks is the gate a file's open goes through, the node's agent sharing it, and openWindow how
	// long the open may take; nil opens unwatched (WatchingOpens).
	disks      *diskwatch.Gate
	openWindow time.Duration
	// openFile opens a file a request names: the payload hashes' OpenNamed; a test's may fail as a
	// disk that does not answer does.
	openFile func(roots map[string]*confine.MediaRoot, named nodeproto.BytesFile) (*os.File, fs.FileInfo, error)
	// probes are the probes of the roots running (D327).
	probes probes
	// placeFacts reads the facts of a place inside a root, relative to it: (*confine.MediaRoot).FactsOf;
	// a test's may block as a disk that does not answer does.
	placeFacts func(root *confine.MediaRoot, relative string) (confine.Facts, error)
}

// WatchingOpens has the server open each file through the node's disk gate, answering one that does
// not open within window, its disk not answering, with 504 and the root in HeaderStalledRoot, as one
// whose root holds a stuck reading at once (D233 §10).
func (server *Server) WatchingOpens(disks *diskwatch.Gate, window time.Duration) *Server {
	server.disks, server.openWindow = disks, window
	return server
}

// New returns a server over the node's roots and its prepared directory, which may be nil.
func New(roots []*confine.MediaRoot, prepared *confine.MediaRoot) *Server {
	byPath := make(map[string]*confine.MediaRoot, len(roots))
	for _, root := range roots {
		byPath[root.Path()] = root
	}
	server := &Server{roots: byPath, prepared: prepared, payloadHashes: namedfile.NewHashes()}
	server.openFile = server.payloadHashes.OpenNamed
	server.placeFacts = (*confine.MediaRoot).FactsOf
	return server
}

// Results are the results a node holds, too large to post, by their attempt's token:
// agent.HeldResults.
type Results interface {
	Result(token string) ([]byte, bool)
}

// ServingResults has the server serve the results the node holds (nodeproto.PathBytesResult).
func (server *Server) ServingResults(results Results) *Server {
	server.results = results
	return server
}

// OnRenditionServed has the server tell served each time it serves one of a rendition's files,
// naming the rendition by its run directory, <file_id>/<profile>/<run_id>: the node's record of
// its last use, by which it evicts (D217 §5).
func (server *Server) OnRenditionServed(served func(renditionDir string)) *Server {
	server.renditionServedHook = served
	return server
}

// Handler is the socket's HTTP handler: GET and HEAD only.
func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+nodeproto.PathBytesFile, server.serveFile)
	mux.HandleFunc("GET "+nodeproto.PathBytesPrepared+"{name...}", server.serveRenditionFile)
	mux.HandleFunc("GET "+nodeproto.PathBytesChunks+"{name...}", server.serveStoreFile)
	mux.HandleFunc("GET "+nodeproto.PathBytesResult+"{token}", server.serveResult)
	mux.HandleFunc("GET "+nodeproto.PathBytesProbe, server.serveProbe)
	return mux
}

// serveResult serves a result the node holds, too large to post, by its attempt's token, as the
// node encoded it: JSON, which the core checks against the size and hash the post named.
func (server *Server) serveResult(writer http.ResponseWriter, request *http.Request) {
	var encoded []byte
	isHeld := false
	if server.results != nil {
		encoded, isHeld = server.results.Result(request.PathValue("token"))
	}
	if !isHeld {
		http.Error(writer, "no result is held for this attempt", http.StatusNotFound)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	// The node's own JSON, on its private socket, to the core: no page renders it.
	_, _ = writer.Write(encoded) //nolint:errcheck,gosec // a core that hung up asks again; as above
}

// serveFile serves nodeproto.PathBytesFile: a file's bytes, honouring Range and If-Range, once its
// payload hash is the one the core asked for.
func (server *Server) serveFile(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	rawPath, pathErr := base64.RawURLEncoding.DecodeString(query.Get("path"))
	if pathErr != nil {
		rawPath = nil // named with no path
	}
	wantPayloadHash := query.Get("payloadHash")
	named := nodeproto.BytesFile{RootPath: query.Get("rootPath"), Path: rawPath, PayloadHash: wantPayloadHash}
	file, info, err := server.openNamed(request.Context(), named)
	switch {
	case errors.Is(err, diskwatch.ErrStalled), diskwatch.IsDiskFailure(err):
		writer.Header().Set(nodeproto.HeaderStalledRoot, named.RootPath)
		http.Error(writer, "the disk this file is on is not answering", http.StatusGatewayTimeout)
		return
	case errors.Is(err, namedfile.ErrNoRoot):
		http.Error(writer, "the root is not one of this node's", http.StatusNotFound)
		return
	case errors.Is(err, namedfile.ErrBadName):
		http.Error(writer, "want rootPath, path (base64url) and payloadHash (hex)", http.StatusBadRequest)
		return
	case errors.Is(err, namedfile.ErrChanged):
		// D100: the core named a file by its payload hash, and this is no longer that file.
		http.Error(writer, "the file at this path is not the one asked for", http.StatusConflict)
		return
	case errors.Is(err, namedfile.ErrUnhashable):
		http.Error(writer, "the file cannot be read", http.StatusInternalServerError)
		return
	case errors.Is(err, namedfile.ErrNotRegular):
		http.Error(writer, "not a regular file", http.StatusNotFound)
		return
	case err != nil:
		http.Error(writer, "no such file under the root", http.StatusNotFound)
		return
	}
	defer readonly.Close(file)
	payloadHash := strings.ToLower(wantPayloadHash) // as hex writes it
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("ETag", `"`+payloadHash+`"`)
	http.ServeContent(writer, request, "", info.ModTime(), file)
}

// openNamed opens the file named through the disk gate, when the server has one.
func (server *Server) openNamed(ctx context.Context, named nodeproto.BytesFile) (*os.File, fs.FileInfo, error) {
	if server.disks == nil {
		return server.openFile(server.roots, named)
	}
	type opened struct {
		file *os.File
		info fs.FileInfo
	}
	operation := diskwatch.Operation{
		Root: named.RootPath, Path: named.RootPath + "/" + string(named.Path), Window: server.openWindow,
	}
	got, err := diskwatch.Run(ctx, server.disks, operation, func() (opened, error) {
		file, info, openErr := server.openFile(server.roots, named)
		return opened{file: file, info: info}, openErr
	}, func(late opened) { readonly.Close(late.file) })
	if err != nil {
		return nil, nil, err
	}
	return got.file, got.info, nil
}

// renditionNamePattern is the only shape a rendition's file may be named by: "<file_id>/<profile>/<run_id>/" and
// then the master playlist, or one rendition's playlist, init segment or media segment.
var renditionNamePattern = regexp.MustCompile(`^[1-9]\d{0,18}/(remux|h264)/[0-9a-f]{16}/` +
	`(master\.m3u8|(video|audio\d{1,3}(-boost)?)/(stream\.m3u8|init(_\d{1,3})?\.mp4|seg\d{5,9}\.m4s))$`)

// serveRenditionFile serves one file of a prepared rendition. A name that is not a rendition's is
// refused before anything is opened; one not written yet is a 404, which the core turns into
// "too early" while the job runs.
func (server *Server) serveRenditionFile(writer http.ResponseWriter, request *http.Request) {
	server.servePreparedFile(writer, request, renditionNamePattern, "no such rendition file")
}

// storeNamePattern is the only shape a store's file may be named by: "<file_id>/chunks-<stream>/<grid_id>/"
// and then its init or a chunk (D247 §10).
var storeNamePattern = regexp.MustCompile(`^[1-9]\d{0,18}/chunks-[a-z0-9-]{1,40}/[0-9a-f]{16,64}/` +
	`(init\.mp4|\d{5,9}\.m4s)$`)

// serveStoreFile serves a store's init or one of its chunks. A name that is not a store's is
// refused before anything is opened; a chunk not made yet is a 404, which the core waits on while
// a run makes it.
func (server *Server) serveStoreFile(writer http.ResponseWriter, request *http.Request) {
	server.servePreparedFile(writer, request, storeNamePattern, "no such chunk")
}

// servePreparedFile serves a file of the prepared directory whose name has the pattern's shape,
// counting it a use of its directory, <file_id>/<profile or store>/<run or grid>, which eviction
// reads (D217 §5).
func (server *Server) servePreparedFile(
	writer http.ResponseWriter, request *http.Request, pattern *regexp.Regexp, notFound string,
) {
	name := request.PathValue("name")
	if server.prepared == nil || !pattern.MatchString(name) {
		http.Error(writer, notFound, http.StatusNotFound)
		return
	}
	file, err := server.prepared.Open(name)
	if err != nil {
		http.Error(writer, notFound, http.StatusNotFound)
		return
	}
	defer readonly.Close(file)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(writer, notFound, http.StatusNotFound)
		return
	}
	if server.renditionServedHook != nil {
		parts := strings.SplitN(name, "/", 4)
		server.renditionServedHook(strings.Join(parts[:3], "/"))
	}
	writer.Header().Set("Content-Type", renditionContentType(name))
	http.ServeContent(writer, request, "", info.ModTime(), file)
}

// renditionContentType is a rendition file's media type, by its name.
func renditionContentType(name string) string {
	switch path.Ext(name) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".m4s":
		return "video/iso.segment"
	}
	return "video/mp4"
}
