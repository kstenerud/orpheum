// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/byteserver"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// folders asks handler for the folders in path, and decodes its answer.
func folders(t *testing.T, handler http.Handler, path string) (int, nodeproto.FolderListing) {
	t.Helper()
	recorder := request(t, handler, http.MethodGet, nodeproto.FoldersRequest{Path: path}.Target(), nil)
	var answer nodeproto.FolderListing
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
			t.Fatal(err)
		}
	}
	return recorder.Code, answer
}

// The folders in a root or a folder inside one are its real folders, no link and no hidden one; a
// path no root holds, or climbing out of one, is refused; a folder not there is missing, and a link
// or a file in its path fails (D359 §2, D107).
func TestServeFolders_ListsTheRealFoldersInAFolderInsideARootAndNothingOutside(t *testing.T) {
	t.Parallel()
	directory, outside := t.TempDir(), t.TempDir()
	for _, folder := range []string{"films/Arkon", "films/.cache", "shows"} {
		if err := os.MkdirAll(filepath.Join(directory, folder), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(directory, "films", "elsewhere")); err != nil {
		t.Fatal(err)
	}
	root := nodetest.OpenMediaRoot(t, directory)
	handler := byteserver.New([]*confine.MediaRoot{root}).Handler()
	if code, answer := folders(t, handler, root.Path()); code != http.StatusOK ||
		answer.State != nodeproto.ProbeReadable || !slices.Equal(answer.Folders, []string{"films", "shows"}) {
		t.Errorf("the root's folders: %d, %+v; want films and shows", code, answer)
	}
	if code, answer := folders(t, handler, root.Path()+"/films"); code != http.StatusOK ||
		!slices.Equal(answer.Folders, []string{"Arkon"}) {
		t.Errorf("films' folders: %d, %+v; want Arkon alone: no link, no hidden folder", code, answer)
	}
	for _, refused := range []string{"/etc", outside, root.Path() + "/films/../..", root.Path() + "/films/", "films"} {
		if code, _ := folders(t, handler, refused); code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404: no root holds it", refused, code)
		}
	}
	if _, answer := folders(t, handler, root.Path()+"/films/gone"); answer.State != nodeproto.ProbeMissing {
		t.Errorf("a folder not there: %+v, want missing", answer)
	}
	if _, answer := folders(t, handler, root.Path()+"/films/elsewhere"); answer.State != nodeproto.ProbeFailed {
		t.Errorf("a link: %+v, want failed: a library folder is named by its real path", answer)
	}
}

// A file named under a library folder inside a root is served as one under the root is; a path
// climbing out of the folder is refused, and a stuck share is the configured root's, which holds one
// stuck reading however many folders it holds, while the 504 names the folder the core asked by
// (D359 §1, §4).
func TestServeFile_ServesAFileUnderALibraryFolderInsideARootAndNothingOutsideIt(t *testing.T) {
	t.Parallel()
	movie := newFilm(t, "video/movies/Arkon (1979)/Arkon (1979).mkv")
	gate := diskwatch.NewGate(clocktest.NewFake(time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)))
	handler := byteserver.New([]*confine.MediaRoot{movie.root}).WatchingOpens(gate, time.Minute).Handler()
	folder := movie.root.Path() + "/video/movies"
	named := func(path string) string {
		return nodeproto.BytesFile{RootPath: folder, Path: []byte(path), PayloadHash: movie.payloadHash}.Target()
	}
	served := request(t, handler, http.MethodGet, named("Arkon (1979)/Arkon (1979).mkv"), nil)
	if served.Code != http.StatusOK || served.Body.String() != string(movie.contents) {
		t.Errorf("a file under a library folder: %d with %d bytes, want 200 with the file", served.Code,
			served.Body.Len())
	}
	climbing := named("../movies/Arkon (1979)/Arkon (1979).mkv")
	if got := request(t, handler, http.MethodGet, climbing, nil); got.Code == http.StatusOK {
		t.Errorf("a path climbing out of the folder: %d, want it refused", got.Code)
	}
	release, isHeld := gate.Hold(movie.root.Path(), movie.root.Path()+"/video/shows/other.mkv")
	if !isHeld {
		t.Fatal("the root could not be held")
	}
	t.Cleanup(release)
	stalled := request(t, handler, http.MethodGet, named("Arkon (1979)/Arkon (1979).mkv"), nil)
	if stalled.Code != http.StatusGatewayTimeout || stalled.Header().Get(nodeproto.HeaderStalledRoot) != folder {
		t.Errorf("a file under a folder whose root's share is stuck: %d naming %q; want 504 naming %s",
			stalled.Code, stalled.Header().Get(nodeproto.HeaderStalledRoot), folder)
	}
}

// A share that has gone away is answered not answering once the open window passes, never waited on;
// the next ask is answered so at once, the listing not tried again while the first has not returned
// (D359 §2, D233 §10).
func TestServeFolders_AShareThatHasGoneIsNotAnsweringWithinTheWindow(t *testing.T) {
	t.Parallel()
	root := nodetest.OpenMediaRoot(t, t.TempDir())
	fake := clocktest.NewFake(time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC))
	gate := diskwatch.NewGate(fake)
	var listed atomic.Int32
	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	handler := byteserver.New([]*confine.MediaRoot{root}).WatchingOpens(gate, 30*time.Second).
		ListingFoldersWith(func(*confine.MediaRoot, string) ([]string, error) {
			listed.Add(1)
			select { // the share's host is gone
			case <-hold:
			case <-t.Context().Done():
			}
			return nil, os.ErrDeadlineExceeded
		}).Handler()
	answered := make(chan nodeproto.FolderListing, 1)
	go func() {
		_, answer := folders(t, handler, root.Path())
		answered <- answer
	}()
	for listed.Load() == 0 || !fake.AdvanceToNext() { // the listing is reading, and waits on its window
		if t.Context().Err() != nil {
			t.Fatal("the listing never waited on its window")
		}
		runtime.Gosched()
	}
	if answer := testutil.Receive(t, answered, "the listing's answer"); answer.State != nodeproto.ProbeNotAnswering {
		t.Errorf("a listing whose share is gone: %+v, want not answering", answer)
	}
	if _, answer := folders(t, handler, root.Path()); answer.State != nodeproto.ProbeNotAnswering ||
		listed.Load() != 1 {
		t.Errorf("the next ask: %+v, listed %d times; want not answering at once, listed once", answer,
			listed.Load())
	}
}
