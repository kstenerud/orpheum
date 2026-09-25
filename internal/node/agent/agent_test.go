// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// fakeCore answers the node channel the way the core does, and hands the test what it hears.
type fakeCore struct {
	mutex       sync.Mutex
	hellos      []nodeproto.Hello
	commands    chan nodeproto.Command
	records     chan nodeproto.Record
	artwork     chan nodeproto.ArtworkResult
	dropReports int // the first reports it ends at once, as a core that went away would
}

func startFakeCore(t *testing.T) (*fakeCore, string) {
	t.Helper()
	core := &fakeCore{
		commands: make(chan nodeproto.Command, 4), records: make(chan nodeproto.Record, 64),
		artwork: make(chan nodeproto.ArtworkResult, 4),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeproto.PathHello, func(writer http.ResponseWriter, request *http.Request) {
		var hello nodeproto.Hello
		if err := json.NewDecoder(request.Body).Decode(&hello); err != nil {
			t.Error(err)
		}
		core.mutex.Lock()
		core.hellos = append(core.hellos, hello)
		core.mutex.Unlock()
		if err := json.NewEncoder(writer).Encode(nodeproto.HelloReply{NodeID: 7, ReportSeq: 40}); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("GET "+nodeproto.PathCommands, func(writer http.ResponseWriter, request *http.Request) {
		select {
		case command := <-core.commands:
			answer := nodeproto.Commands{Commands: []nodeproto.Command{command}}
			if err := json.NewEncoder(writer).Encode(answer); err != nil {
				t.Error(err)
			}
		case <-request.Context().Done():
		}
	})
	mux.HandleFunc("POST "+nodeproto.PathReport, func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get(nodeHeader) != "7" {
			t.Errorf("a report named node %q, want the 7 the hello answered", request.Header.Get(nodeHeader))
		}
		core.mutex.Lock()
		drop := core.dropReports > 0
		core.dropReports--
		core.mutex.Unlock()
		if drop {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		lines := bufio.NewScanner(request.Body)
		for lines.Scan() {
			var record nodeproto.Record
			if err := json.Unmarshal(lines.Bytes(), &record); err != nil {
				t.Error(err)
				return
			}
			core.records <- record
		}
	})
	mux.HandleFunc("POST "+nodeproto.PathArtwork, core.artworkHandler(t))
	socket := filepath.Join(t.TempDir(), "c.sock")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Minute}
	go func() { _ = server.Serve(listener) }() //nolint:errcheck // ends when the test closes it
	t.Cleanup(func() { _ = server.Close() })   //nolint:errcheck // test teardown
	return core, socket
}

// artworkHandler hands the test each artwork answer the node posts.
func (core *fakeCore) artworkHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		var result nodeproto.ArtworkResult
		if err := json.NewDecoder(request.Body).Decode(&result); err != nil {
			t.Error(err)
		}
		core.artwork <- result
		if _, err := writer.Write([]byte("{}")); err != nil {
			t.Error(err)
		}
	}
}

func (core *fakeCore) helloCount() int {
	core.mutex.Lock()
	defer core.mutex.Unlock()
	return len(core.hellos)
}

// fakeScanner reports a fixed scan of whatever root it is given.
type fakeScanner struct{}

func (fakeScanner) Scan(_ context.Context, root *confine.MediaRoot, emit scan.Emit) error {
	for _, record := range []nodeproto.Record{
		{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{Root: root.Path()}},
		{Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{Root: root.Path()}},
	} {
		if err := emit(record); err != nil {
			return err
		}
	}
	return nil
}

// fakeArtwork answers every artwork command with one size, the source's bytes reversed.
type fakeArtwork struct{}

func (fakeArtwork) Derive(_ context.Context, command nodeproto.Artwork) nodeproto.ArtworkResult {
	reversed := slices.Clone(command.Source)
	slices.Reverse(reversed)
	return nodeproto.ArtworkResult{
		Job: command.Job, Images: []nodeproto.ArtImage{{Size: nodeproto.PortraitS, Bytes: reversed}},
	}
}

func running(t *testing.T, socket string) (*confine.MediaRoot, chan time.Time) {
	t.Helper()
	root, err := confine.OpenMediaRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() }) //nolint:errcheck // test teardown
	now := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	workers := Workers{Scanner: fakeScanner{}, Artwork: fakeArtwork{}}
	node := New(socket, nodeproto.Hello{Name: "local", Version: "test"}, []*confine.MediaRoot{root}, workers,
		now, slog.New(slog.DiscardHandler))
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1)
	go func() { finished <- node.Run(ctx, tick) }()
	t.Cleanup(func() {
		cancel()
		if err := <-finished; err != nil {
			t.Error(err)
		}
	})
	return root, tick
}

func receive(t *testing.T, core *fakeCore) nodeproto.Record {
	t.Helper()
	select {
	case record := <-core.records:
		return record
	case <-t.Context().Done():
		t.Fatal("the test ended before the core heard a record")
		return nodeproto.Record{}
	}
}

func TestAgent_SaysHelloRunsAScanAndStreamsItsReportInNumberedOrder(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, tick := running(t, socket)

	core.commands <- nodeproto.Command{Seq: 1, Type: nodeproto.CommandScan, Scan: &nodeproto.Scan{Root: root.Path()}}
	begin, complete := receive(t, core), receive(t, core)
	if begin.Type != nodeproto.RecordScanBegin || begin.Seq != 41 || complete.Type != nodeproto.RecordScanComplete ||
		complete.Seq != 42 {
		t.Errorf("heard %s #%d then %s #%d; want the scan numbered on from the core's 40",
			begin.Type, begin.Seq, complete.Type, complete.Seq)
	}
	// A status record reaches the core while the report is still open: each record is sent as
	// it is written, not when the request ends.
	tick <- time.Time{}
	if status := receive(t, core); status.Type != nodeproto.RecordStatus || status.Seq != 43 ||
		status.Status.Doing != "idle" {
		t.Errorf("heard %+v, want status #43, idle", status)
	}
	core.mutex.Lock()
	hello := core.hellos[0]
	core.mutex.Unlock()
	if hello.Name != "local" || len(hello.Roots) != 1 || hello.Roots[0] != root.Path() {
		t.Errorf("the hello was %+v, want the node's name and its root", hello)
	}
}

func TestAgent_RefusesToScanARootItWasNotGiven(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	_, tick := running(t, socket)
	core.commands <- nodeproto.Command{Seq: 1, Type: nodeproto.CommandScan, Scan: &nodeproto.Scan{Root: "/etc"}}
	tick <- time.Time{}
	if record := receive(t, core); record.Type != nodeproto.RecordStatus {
		t.Errorf("heard %s before the status; a scan of /etc ran (D107)", record.Type)
	}
}

func TestAgent_SaysHelloAgainAfterLosingTheCore(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	core.dropReports = 1
	_, tick := running(t, socket)
	// Tick until the core hears something: the first session fails at once, a tick starts the
	// next, and a later one becomes a status record in it.
	ticking, stop := context.WithCancel(t.Context())
	defer stop()
	go func() {
		for {
			select {
			case tick <- time.Time{}:
			case <-ticking.Done():
				return
			}
		}
	}()
	if record := receive(t, core); record.Type != nodeproto.RecordStatus {
		t.Errorf("heard %s, want a status in the second session", record.Type)
	}
	if count := core.helloCount(); count != 2 {
		t.Errorf("%d hellos, want 2: one per session", count)
	}
}

func TestAgent_ReEncodesArtworkAndPostsTheAnswer(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	running(t, socket)
	core.commands <- nodeproto.Command{Seq: 1, Type: nodeproto.CommandArtwork, Artwork: &nodeproto.Artwork{
		Job: 12, Source: []byte("abc"), Shape: nodeproto.ShapePortrait,
		Sizes: []nodeproto.ArtSizeName{nodeproto.PortraitS},
	}}
	select {
	case result := <-core.artwork:
		if result.Job != 12 || len(result.Images) != 1 || string(result.Images[0].Bytes) != "cba" {
			t.Errorf("the core heard %+v", result)
		}
	case <-t.Context().Done():
		t.Fatal("no artwork answer reached the core")
	}
}

func TestAgent_ReadsAnImageBesideTheMediaOnlyUnderItsRoots(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := running(t, socket)
	if err := os.WriteFile(filepath.Join(root.Path(), "poster.jpg"), []byte("xyz"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.jpg")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	answer := func(job int64, named string) nodeproto.ArtworkResult {
		t.Helper()
		core.commands <- nodeproto.Command{Seq: job, Type: nodeproto.CommandArtwork, Artwork: &nodeproto.Artwork{
			Job: job, Path: &nodeproto.Path{Text: named}, Shape: nodeproto.ShapePortrait,
			Sizes: []nodeproto.ArtSizeName{nodeproto.PortraitS},
		}}
		select {
		case result := <-core.artwork:
			return result
		case <-t.Context().Done():
			t.Fatal("no artwork answer reached the core")
			return nodeproto.ArtworkResult{}
		}
	}
	sum := sha256.Sum256([]byte("xyz"))
	if beside := answer(1, filepath.Join(root.Path(), "poster.jpg")); beside.Refused != "" ||
		string(beside.Images[0].Bytes) != "zyx" || beside.SourceHash != hex.EncodeToString(sum[:]) {
		t.Errorf("an image under the root: %+v; want it read, re-encoded and hashed", beside)
	}
	for job, named := range map[int64]string{
		2: outside, 3: filepath.Join(root.Path(), "..", filepath.Base(filepath.Dir(outside)), "secret.jpg"),
	} {
		if refused := answer(job, named); refused.Refused == "" || len(refused.Images) != 0 {
			t.Errorf("%s: %+v; a path outside the node's roots is refused", named, refused)
		}
	}
}
