// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver_test

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/byteserver"
	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/diskwatch"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// probe asks handler to probe root, and decodes its answer.
func probe(t *testing.T, handler http.Handler, root string) (int, nodeproto.RootProbe) {
	t.Helper()
	recorder := request(t, handler, http.MethodGet, nodeproto.ProbeRequest{RootPath: root}.Target(), nil)
	var answer nodeproto.RootProbe
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
			t.Fatal(err)
		}
	}
	return recorder.Code, answer
}

// A probe answers whether one of the node's roots can be read now, with the facts the core tells
// its disk back by, and opens nothing under it; a root that is not the node's is refused, and one
// whose directory is gone is missing (D327 §1).
func TestServeProbe_AnswersAReadableRootWithItsFacts(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	root := nodetest.OpenMediaRoot(t, directory)
	gone := nodetest.OpenMediaRoot(t, t.TempDir())
	handler := byteserver.New([]*confine.MediaRoot{root, gone}, nil).
		ProbingWith(func(probed *confine.MediaRoot) (confine.Facts, error) {
			if probed == gone {
				return confine.Facts{}, &fs.PathError{Op: "stat", Path: gone.Path(), Err: fs.ErrNotExist}
			}
			return probed.Facts()
		}).Handler()
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	device, _ := confine.DeviceOf(info)
	if code, answer := probe(t, handler, directory); code != http.StatusOK ||
		answer.State != nodeproto.ProbeReadable || answer.Device != device || answer.IsMountPoint {
		t.Errorf("a readable root: %d, %+v; want readable on device %d, not a mount point", code, answer, device)
	}
	if code, _ := probe(t, handler, "/etc"); code != http.StatusNotFound {
		t.Errorf("a root not the node's: %d, want 404", code)
	}
	if code, answer := probe(t, handler, gone.Path()); code != http.StatusOK || answer.State != nodeproto.ProbeMissing {
		t.Errorf("a root whose directory is gone: %d, %+v; want missing", code, answer)
	}
}

// A root holding a reading that has not returned is not answering at once, since when and on what,
// its probe not tried: it would only wait beside the reading (D233 §10, D327 §2).
func TestServeProbe_ARootHoldingAStuckReadingIsNotAnsweringAtOnce(t *testing.T) {
	t.Parallel()
	root := nodetest.OpenMediaRoot(t, t.TempDir())
	stuckAt := time.Date(2026, 10, 21, 19, 6, 32, 0, time.UTC)
	gate := diskwatch.NewGate(clocktest.NewFake(stuckAt))
	var measured atomic.Int32
	handler := byteserver.New([]*confine.MediaRoot{root}, nil).WatchingOpens(gate, time.Minute).
		ProbingWith(func(root *confine.MediaRoot) (confine.Facts, error) {
			measured.Add(1)
			return root.Facts()
		}).Handler()
	release, isHeld := gate.Hold(root.Path(), root.Path()+"/Ditchwater.mkv")
	if !isHeld {
		t.Fatal("the root could not be held")
	}
	t.Cleanup(release)
	code, answer := probe(t, handler, root.Path())
	if code != http.StatusOK || answer.State != nodeproto.ProbeNotAnswering ||
		answer.StuckSinceMs != stuckAt.UnixMilli() || answer.WaitingOnPath != root.Path()+"/Ditchwater.mkv" ||
		measured.Load() != 0 {
		t.Errorf("a root holding a stuck reading: %d, %+v, measured %d times; want not answering since %v, "+
			"on Ditchwater.mkv, not measured", code, answer, measured.Load(), stuckAt)
	}
}

// Asks of a root while its probe runs share it: a probe of a share that is gone waits in the
// kernel, and each one more is one more thread held (D327 §2).
func TestServeProbe_AsksWhileAProbeRunsShareIt(t *testing.T) {
	t.Parallel()
	root := nodetest.OpenMediaRoot(t, t.TempDir())
	release := make(chan struct{})
	var measured atomic.Int32
	server := byteserver.New([]*confine.MediaRoot{root}, nil).
		ProbingWith(func(root *confine.MediaRoot) (confine.Facts, error) {
			measured.Add(1)
			select { // the share's host is gone, then back
			case <-release:
			case <-t.Context().Done():
			}
			return root.Facts()
		})
	handler := server.Handler()
	var answered sync.WaitGroup
	answers := make(chan nodeproto.RootProbe, 3)
	for range 3 {
		answered.Go(func() {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
				nodeproto.ProbeRequest{RootPath: root.Path()}.Target(), nil))
			var answer nodeproto.RootProbe
			_ = json.Unmarshal(recorder.Body.Bytes(), &answer) //nolint:errcheck // a bad answer fails below
			answers <- answer
		})
	}
	for server.ProbeAsks(root.Path()) != 3 { // each ask joined, the probe still waiting on its disk
		if t.Context().Err() != nil {
			t.Fatal("three asks never shared the probe")
		}
		runtime.Gosched()
	}
	close(release)
	answered.Wait()
	close(answers)
	for answer := range answers {
		if answer.State != nodeproto.ProbeReadable {
			t.Errorf("an ask's answer: %+v; want the shared probe's, readable", answer)
		}
	}
	if count := measured.Load(); count != 1 {
		t.Errorf("the root was measured %d times for three asks; want once", count)
	}
}
