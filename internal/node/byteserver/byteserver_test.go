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
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// film is a root with one file in it, and the file's identity.
type film struct {
	root     *confine.MediaRoot
	path     string
	contents []byte
	identity string
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
	root, err := confine.OpenMediaRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() }) //nolint:errcheck // read-only
	file, err := root.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // read-only
	identity, err := hash.Payload(file, int64(len(contents)))
	if err != nil {
		t.Fatal(err)
	}
	return film{root: root, path: name, contents: contents, identity: hex.EncodeToString(identity.Hash[:])}
}

func (movie film) target(path, identity string) string {
	return nodeproto.BytesFile{Root: movie.root.Path(), Path: []byte(path), PayloadHash: identity}.Target()
}

// get GETs the film's own path, naming identity as its hash.
func (movie film) get(
	t *testing.T, handler http.Handler, identity string, header http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	return request(t, handler, http.MethodGet, movie.target(movie.path, identity), header)
}

func request(t *testing.T, handler http.Handler, method, target string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	built := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	maps.Copy(built.Header, header)
	handler.ServeHTTP(recorder, built)
	return recorder
}

func TestFile_ServesTheBytesOfTheFileItsHashNames(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "Films/Film (1999)/Film (1999).mkv")
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	whole := movie.get(t, handler, movie.identity, nil)
	if whole.Code != http.StatusOK || whole.Body.String() != string(movie.contents) {
		t.Fatalf("GET = %d with %d bytes, want 200 with the file's %d",
			whole.Code, whole.Body.Len(), len(movie.contents))
	}
	if got := whole.Header().Get("ETag"); got != `"`+movie.identity+`"` {
		t.Errorf("ETag %q, want the payload hash", got)
	}
	if got := whole.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges %q, want bytes", got)
	}
	head := request(t, handler, http.MethodHead, movie.target(movie.path, strings.ToUpper(movie.identity)), nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Errorf("HEAD with the hash in capitals = %d with %d bytes, want 200 and no body", head.Code, head.Body.Len())
	}
}

func TestFile_HonoursRangeAndIfRange(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "film.mkv")
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	ranged := movie.get(t, handler, movie.identity, http.Header{"Range": {"bytes=10-19"}})
	if ranged.Code != http.StatusPartialContent || ranged.Body.String() != string(movie.contents[10:20]) {
		t.Errorf("Range bytes=10-19 = %d %q, want 206 %q", ranged.Code, ranged.Body.String(), movie.contents[10:20])
	}
	stale := request(t, handler, http.MethodGet, movie.target(movie.path, movie.identity),
		http.Header{"Range": {"bytes=10-19"}, "If-Range": {`"another"`}})
	if stale.Code != http.StatusOK || stale.Body.Len() != len(movie.contents) {
		t.Errorf("If-Range with another ETag = %d with %d bytes, want the whole file", stale.Code, stale.Body.Len())
	}
}

func TestFile_RefusesAFileThatIsNoLongerTheOneAskedFor(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "film.mkv")
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	if got := movie.get(t, handler, movie.identity, nil); got.Code != http.StatusOK {
		t.Fatalf("the first GET = %d, want 200", got.Code)
	}
	other := strings.Repeat("0", len(movie.identity))
	if got := movie.get(t, handler, other, nil); got.Code != http.StatusConflict {
		t.Errorf("GET naming another hash = %d, want 409", got.Code)
	}
	// Replaced in place after its identity was remembered: the new size makes it hashed again.
	full := filepath.Join(movie.root.Path(), movie.path)
	if err := os.WriteFile(full, []byte("a different film"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Unix(2_000_000_000, 0) // any time but the one it was written at
	if err := os.Chtimes(full, later, later); err != nil {
		t.Fatal(err)
	}
	if got := movie.get(t, handler, movie.identity, nil); got.Code != http.StatusConflict {
		t.Errorf("GET after the file was replaced = %d, want 409", got.Code)
	}
}

func TestFile_NamesNothingOutsideARoot(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "films/film.mkv")
	outside := filepath.Join(filepath.Dir(movie.root.Path()), "secret.mkv")
	if err := os.WriteFile(outside, movie.contents, 0o600); err != nil {
		t.Fatal(err)
	}
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	cases := map[string]string{
		"a path out of the root": movie.target("../secret.mkv", movie.identity),
		"an absolute path":       movie.target(outside, movie.identity),
		"a directory":            movie.target("films", movie.identity),
		"a root it does not have": nodeproto.BytesFile{
			Root: filepath.Dir(movie.root.Path()), Path: []byte("secret.mkv"), PayloadHash: movie.identity,
		}.Target(),
	}
	for name, target := range cases {
		if got := request(t, handler, http.MethodGet, target, nil); got.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", name, got.Code)
		}
	}
	for name, target := range map[string]string{
		"no hash": nodeproto.PathBytesFile + "?" + url.Values{"root": {movie.root.Path()}, "path": {"eA"}}.Encode(),
		"a path not base64": nodeproto.PathBytesFile + "?" + url.Values{
			"root": {movie.root.Path()}, "path": {"%%"}, "hash": {movie.identity},
		}.Encode(),
	} {
		if got := request(t, handler, http.MethodGet, target, nil); got.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, got.Code)
		}
	}
}

func TestFile_OnlyReads(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "film.mkv")
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		got := request(t, handler, method, movie.target(movie.path, movie.identity), nil)
		if got.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s = %d, want 405", method, got.Code)
		}
	}
}

func TestFile_ServesANameThatIsNotUTF8(t *testing.T) {
	t.Parallel()
	name := "caf\xe9.mkv" // Latin-1, as an old disk wrote it (D49)
	movie := newFilm(t, name)
	handler := byteserver.New([]*confine.MediaRoot{movie.root}, nil).Handler()
	if got := request(t, handler, http.MethodGet, movie.target(name, movie.identity), nil); got.Code != http.StatusOK {
		t.Errorf("GET a Latin-1 name = %d, want 200", got.Code)
	}
}

// rendition is a prepared directory with one rendition's master playlist, a segment and an init.
func rendition(t *testing.T) *confine.MediaRoot {
	t.Helper()
	directory := t.TempDir()
	for name, contents := range map[string]string{
		"7/remux/0123456789abcdef/master.m3u8":               "#EXTM3U\n",
		"7/remux/0123456789abcdef/audio0-boost/seg00001.m4s": "segment",
		"7/remux/0123456789abcdef/video/init_0.mp4":          "init",
		"7/remux/0123456789abcdef/notes.txt":                 "not a rendition's",
	} {
		full := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := confine.OpenMediaRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close() }) //nolint:errcheck // read-only
	return prepared
}

func TestRenditionFile_ServesARenditionsFilesAndNothingElse(t *testing.T) {
	t.Parallel()
	handler := byteserver.New(nil, rendition(t)).Handler()
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

func TestRenditionFile_SaysWhichRenditionItServed(t *testing.T) {
	t.Parallel()
	var served []string
	handler := byteserver.New(nil, rendition(t)).OnRenditionServed(func(renditionDir string) {
		served = append(served, renditionDir)
	}).Handler()
	const run = nodeproto.PathBytesPrepared + "7/remux/0123456789abcdef/"
	request(t, handler, http.MethodGet, run+"audio0-boost/seg00001.m4s", nil)
	request(t, handler, http.MethodGet, run+"notes.txt", nil)
	if want := []string{"7/remux/0123456789abcdef"}; !slices.Equal(served, want) {
		t.Errorf("served %v, want only the segment's rendition, its last use (D217 §5)", served)
	}
}
