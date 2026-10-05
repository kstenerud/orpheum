// SPDX-License-Identifier: GPL-3.0-or-later

package byteserver_test

import (
	"encoding/hex"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/byteserver"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// film is a root with one file in it, and the file's payload hash.
type film struct {
	root        *confine.MediaRoot
	path        string
	contents    []byte
	payloadHash string
}

func newFilm(t *testing.T, name string) film {
	t.Helper()
	directory := t.TempDir()
	contents := []byte(strings.Repeat("not a real film; hashed whole. ", 200))
	full := filepath.Join(directory, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	root := nodetest.OpenMediaRoot(t, directory)
	file, err := root.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // read-only
	identity, err := hash.PayloadIdentity(file, int64(len(contents)))
	if err != nil {
		t.Fatal(err)
	}
	return film{root: root, path: name, contents: contents, payloadHash: hex.EncodeToString(identity.PayloadHash[:])}
}

func (movie film) target(path, payloadHash string) string {
	return nodeproto.BytesFile{RootPath: movie.root.Path(), Path: []byte(path), PayloadHash: payloadHash}.Target()
}

// get GETs the film's own path, naming payloadHash as its hash.
func (movie film) get(
	t *testing.T, handler http.Handler, payloadHash string, header http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	return request(t, handler, http.MethodGet, movie.target(movie.path, payloadHash), header)
}

func request(t *testing.T, handler http.Handler, method, target string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	built := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	maps.Copy(built.Header, header)
	handler.ServeHTTP(recorder, built)
	return recorder
}

func TestServeFile_ServesTheBytesOfTheFileItsHashNames(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "Films/Film (1999)/Film (1999).mkv")
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	whole := movie.get(t, handler, movie.payloadHash, nil)
	if whole.Code != http.StatusOK || whole.Body.String() != string(movie.contents) {
		t.Fatalf("GET = %d with %d bytes, want 200 with the file's %d",
			whole.Code, whole.Body.Len(), len(movie.contents))
	}
	if got := whole.Header().Get("ETag"); got != `"`+movie.payloadHash+`"` {
		t.Errorf("ETag %q, want the payload hash", got)
	}
	if got := whole.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges %q, want bytes", got)
	}
	head := request(t, handler, http.MethodHead, movie.target(movie.path, strings.ToUpper(movie.payloadHash)), nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Errorf("HEAD with the hash in capitals = %d with %d bytes, want 200 and no body", head.Code, head.Body.Len())
	}
}

func TestServeFile_HonoursRangeAndIfRange(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "film.mkv")
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	ranged := movie.get(t, handler, movie.payloadHash, http.Header{"Range": {"bytes=10-19"}})
	if ranged.Code != http.StatusPartialContent || ranged.Body.String() != string(movie.contents[10:20]) {
		t.Errorf("Range bytes=10-19 = %d %q, want 206 %q", ranged.Code, ranged.Body.String(), movie.contents[10:20])
	}
	stale := request(t, handler, http.MethodGet, movie.target(movie.path, movie.payloadHash),
		http.Header{"Range": {"bytes=10-19"}, "If-Range": {`"another"`}})
	if stale.Code != http.StatusOK || stale.Body.Len() != len(movie.contents) {
		t.Errorf("If-Range with another ETag = %d with %d bytes, want the whole file", stale.Code, stale.Body.Len())
	}
}

func TestServeFile_RefusesAFileThatIsNoLongerTheOneAskedFor(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "film.mkv")
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	if got := movie.get(t, handler, movie.payloadHash, nil); got.Code != http.StatusOK {
		t.Fatalf("the first GET = %d, want 200", got.Code)
	}
	other := strings.Repeat("0", len(movie.payloadHash))
	if got := movie.get(t, handler, other, nil); got.Code != http.StatusConflict {
		t.Errorf("GET naming another hash = %d, want 409", got.Code)
	}
	// Replaced in place after its payload hash was remembered: the new size makes it hashed again.
	full := filepath.Join(movie.root.Path(), movie.path)
	if err := os.WriteFile(full, []byte("a different film"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Unix(2_000_000_000, 0) // any time but the one it was written at
	if err := os.Chtimes(full, later, later); err != nil {
		t.Fatal(err)
	}
	if got := movie.get(t, handler, movie.payloadHash, nil); got.Code != http.StatusConflict {
		t.Errorf("GET after the file was replaced = %d, want 409", got.Code)
	}
}

func TestServeFile_NamesNothingOutsideARoot(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "films/film.mkv")
	outside := filepath.Join(filepath.Dir(movie.root.Path()), "secret.mkv")
	if err := os.WriteFile(outside, movie.contents, 0o600); err != nil {
		t.Fatal(err)
	}
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	cases := map[string]string{
		"a path out of the root": movie.target("../secret.mkv", movie.payloadHash),
		"an absolute path":       movie.target(outside, movie.payloadHash),
		"a directory":            movie.target("films", movie.payloadHash),
		"a root it does not have": nodeproto.BytesFile{
			RootPath: filepath.Dir(movie.root.Path()), Path: []byte("secret.mkv"), PayloadHash: movie.payloadHash,
		}.Target(),
	}
	for name, target := range cases {
		if got := request(t, handler, http.MethodGet, target, nil); got.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", name, got.Code)
		}
	}
	for name, target := range map[string]string{
		"no hash": nodeproto.PathBytesFile + "?" + url.Values{"rootPath": {movie.root.Path()}, "path": {"eA"}}.Encode(),
		"a path not base64": nodeproto.PathBytesFile + "?" + url.Values{
			"rootPath": {movie.root.Path()}, "path": {"%%"}, "payloadHash": {movie.payloadHash},
		}.Encode(),
	} {
		got := request(t, handler, http.MethodGet, target, nil)
		if got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "payloadHash") {
			t.Errorf("%s: %d %q, want 400 naming the parameters, payloadHash among them", name, got.Code,
				got.Body.String())
		}
	}
}

func TestServeFile_OnlyReads(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "film.mkv")
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		got := request(t, handler, method, movie.target(movie.path, movie.payloadHash), nil)
		if got.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s = %d, want 405", method, got.Code)
		}
	}
}

func TestServeFile_ServesANameThatIsNotUTF8(t *testing.T) {
	t.Parallel()
	name := "caf\xe9.mkv" // Latin-1, as an old disk wrote it (D49)
	movie := newFilm(t, name)
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	got := request(t, handler, http.MethodGet, movie.target(name, movie.payloadHash), nil)
	if got.Code != http.StatusOK {
		t.Errorf("GET a Latin-1 name = %d, want 200", got.Code)
	}
}

// preparedRoot is a prepared directory with one rendition's master playlist, a segment and an init.
func preparedRoot(t *testing.T) *confine.MediaRoot {
	t.Helper()
	directory := t.TempDir()
	for name, contents := range map[string]string{
		"7/remux/0123456789abcdef/master.m3u8":                 "#EXTM3U\n",
		"7/remux/0123456789abcdef/audio0-boost/seg00001.m4s":   "segment",
		"7/remux/0123456789abcdef/video/init_0.mp4":            "init",
		"7/remux/0123456789abcdef/notes.txt":                   "not a rendition's",
		"7/chunks-q1200k480p/0123456789abcdef/init.mp4":        "init",
		"7/chunks-q1200k480p/0123456789abcdef/00012.m4s":       "chunk",
		"7/chunks-q1200k480p/0123456789abcdef/store.json":      "the store's manifest",
		"7/chunks-q1200k480p/0123456789abcdef/raw-1/00013.mp4": "a raw file",
	} {
		full := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	prepared := nodetest.OpenMediaRoot(t, directory)
	return prepared
}

func TestServeRenditionFile_ServesARenditionsFilesAndNothingElse(t *testing.T) {
	t.Parallel()
	handler := byteserver.New(nil, preparedRoot(t)).Handler()
	for name, want := range map[string]string{
		"7/remux/0123456789abcdef/master.m3u8":               "application/vnd.apple.mpegurl",
		"7/remux/0123456789abcdef/audio0-boost/seg00001.m4s": "video/iso.segment",
		"7/remux/0123456789abcdef/video/init_0.mp4":          "video/mp4",
	} {
		got := request(t, handler, http.MethodGet, nodeproto.PathBytesPrepared+name, nil)
		if got.Code != http.StatusOK || got.Header().Get("Content-Type") != want {
			t.Errorf("%s: %d %q, want 200 %s", name, got.Code, got.Header().Get("Content-Type"), want)
		}
	}
	for _, name := range []string{
		"7/remux/0123456789abcdef/notes.txt", "7/remux/0123456789abcdef/video/seg00002.m4s",
		"7/other/0123456789abcdef/master.m3u8", "0/remux/0123456789abcdef/master.m3u8", "7/remux/master.m3u8",
	} {
		if got := request(t, handler, http.MethodGet, nodeproto.PathBytesPrepared+name, nil); got.Code !=
			http.StatusNotFound {
			t.Errorf("%s: %d, want 404", name, got.Code)
		}
	}
	// A path with .. is only ever redirected to its cleaned form, which is then judged as above.
	for _, name := range []string{"7/remux/../remux/master.m3u8", "7/remux/video/../../../etc/passwd"} {
		got := request(t, handler, http.MethodGet, nodeproto.PathBytesPrepared+name, nil)
		if got.Code == http.StatusOK {
			t.Errorf("%s was served", name)
		}
	}
	if got := request(t, byteserver.New(nil, nil).Handler(), http.MethodGet,
		nodeproto.PathBytesPrepared+"7/remux/0123456789abcdef/master.m3u8", nil); got.Code != http.StatusNotFound {
		t.Errorf("a node that does not prepare: %d, want 404", got.Code)
	}
}

func TestServeRenditionFile_SaysWhichRenditionItServed(t *testing.T) {
	t.Parallel()
	var served []string
	handler := byteserver.New(nil, preparedRoot(t)).OnRenditionServed(func(renditionDir string) {
		served = append(served, renditionDir)
	}).Handler()
	const run = nodeproto.PathBytesPrepared + "7/remux/0123456789abcdef/"
	request(t, handler, http.MethodGet, run+"audio0-boost/seg00001.m4s", nil)
	request(t, handler, http.MethodGet, run+"notes.txt", nil)
	if want := []string{"7/remux/0123456789abcdef"}; !slices.Equal(served, want) {
		t.Errorf("served %v, want only the segment's rendition, its last use (D217 §5)", served)
	}
}

// A store's init and chunks are served, and counted a use of the store; its manifest, its raw files
// and any other name are not (D247 §10).
func TestServeStoreFile_ServesAStoresInitAndChunksAndNothingElse(t *testing.T) {
	t.Parallel()
	var served []string
	handler := byteserver.New(nil, preparedRoot(t)).OnRenditionServed(func(dir string) {
		served = append(served, dir)
	}).Handler()
	const store = nodeproto.PathBytesChunks + "7/chunks-q1200k480p/0123456789abcdef/"
	for name, want := range map[string]string{"init.mp4": "video/mp4", "00012.m4s": "video/iso.segment"} {
		got := request(t, handler, http.MethodGet, store+name, nil)
		if got.Code != http.StatusOK || got.Header().Get("Content-Type") != want {
			t.Errorf("%s: %d %q, want 200 %s", name, got.Code, got.Header().Get("Content-Type"), want)
		}
	}
	for _, name := range []string{
		store + "store.json", store + "raw-1/00013.mp4", store + "00013.m4s",
		nodeproto.PathBytesChunks + "7/remux/0123456789abcdef/master.m3u8",
		nodeproto.PathBytesPrepared + "7/chunks-q1200k480p/0123456789abcdef/00012.m4s",
	} {
		if got := request(t, handler, http.MethodGet, name, nil); got.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", name, got.Code)
		}
	}
	const storeDir = "7/chunks-q1200k480p/0123456789abcdef"
	if want := []string{storeDir, storeDir}; !slices.Equal(served, want) {
		t.Errorf("served %v, want the store twice, for its init and its chunk", served)
	}
}

// heldResults holds results by token, as agent.HeldResults does.
type heldResults map[string][]byte

func (held heldResults) Result(token string) ([]byte, bool) {
	encoded, found := held[token]
	return encoded, found
}

// A result the node holds, too large to post, is served whole by its attempt's token, and nothing
// is for a token it does not hold (D234 §5).
func TestServeResult_ServesAHeldResultByItsTokenAndNothingElse(t *testing.T) {
	t.Parallel()
	held := heldResults{"a5": []byte(`{"jobId":5}`)}
	handler := byteserver.New(nil, nil).ServingResults(held).Handler()
	served := request(t, handler, http.MethodGet, nodeproto.PathBytesResult+"a5", nil)
	if served.Code != http.StatusOK || served.Body.String() != `{"jobId":5}` ||
		served.Header().Get("Content-Type") != "application/json" {
		t.Errorf("the held result: %d %q, want it whole as JSON", served.Code, served.Body.String())
	}
	if unknown := request(t, handler, http.MethodGet, nodeproto.PathBytesResult+"a6", nil); unknown.Code !=
		http.StatusNotFound {
		t.Errorf("a token held nothing for: %d, want 404", unknown.Code)
	}
	servingNone := byteserver.New(nil, nil).Handler()
	if none := request(t, servingNone, http.MethodGet, nodeproto.PathBytesResult+"a5", nil); none.Code !=
		http.StatusNotFound {
		t.Errorf("a server serving no results: %d, want 404", none.Code)
	}
}
