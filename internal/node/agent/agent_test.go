// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
)

// fakeCore answers the node channel the way the core does, and hands the test what it hears.
type fakeCore struct {
	mutex          sync.Mutex
	hellos         []nodeproto.Hello
	commands       chan nodeproto.Command
	records        chan nodeproto.Record
	artwork        chan nodeproto.ArtworkResult
	prepareReports chan nodeproto.PrepareReport
	subtitles      chan nodeproto.SubtitleText
	loudness       chan nodeproto.LoudnessResult
	reportsToDrop  int           // the first reports it ends at once, as a core that went away would
	stopped        chan struct{} // closed at teardown: a handler stops handing on what it heard
}

// isMalformed says the node posted something that is not the JSON it should be; a body cut short
// is the node going away, which the tests cause.
func isMalformed(err error) bool {
	var syntaxError *json.SyntaxError
	var typeError *json.UnmarshalTypeError
	return errors.As(err, &syntaxError) || errors.As(err, &typeError)
}

// handOn gives the test what a handler heard, unless the test has ended and nobody will read it.
func handOn[T any](core *fakeCore, into chan<- T, value T) bool {
	select {
	case into <- value:
		return true
	case <-core.stopped:
		return false
	}
}

func startFakeCore(t *testing.T) (*fakeCore, string) {
	t.Helper()
	core := &fakeCore{
		commands: make(chan nodeproto.Command, 4), records: make(chan nodeproto.Record, 64),
		artwork: make(chan nodeproto.ArtworkResult, 4), prepareReports: make(chan nodeproto.PrepareReport, 16),
		subtitles: make(chan nodeproto.SubtitleText, 4), loudness: make(chan nodeproto.LoudnessResult, 4),
		stopped: make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+nodeproto.PathHello, func(writer http.ResponseWriter, request *http.Request) {
		var hello nodeproto.Hello
		if err := json.NewDecoder(request.Body).Decode(&hello); isMalformed(err) {
			t.Error(err)
		}
		core.mutex.Lock()
		core.hellos = append(core.hellos, hello)
		core.mutex.Unlock()
		if err := json.NewEncoder(writer).Encode(nodeproto.HelloReply{
			NodeID:                7,
			LastIngestedReportSeq: 40,
		}); err != nil {
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
		case <-core.stopped:
		}
	})
	mux.HandleFunc("POST "+nodeproto.PathReport, core.receiveReport(t))
	mux.HandleFunc("POST "+nodeproto.PathArtwork, forwardPosted(t, core, core.artwork))
	mux.HandleFunc("POST "+nodeproto.PathPrepare, forwardPosted(t, core, core.prepareReports))
	mux.HandleFunc("POST "+nodeproto.PathSubtitle, forwardPosted(t, core, core.subtitles))
	mux.HandleFunc("POST "+nodeproto.PathLoudness, forwardPosted(t, core, core.loudness))
	socket := core.serve(t, mux)
	return core, socket
}

// receiveReport hands the test each record of a report, dropping the first reportsToDrop.
func (core *fakeCore) receiveReport(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get(nodeHeader) != "7" {
			t.Errorf("a report named node %q, want the 7 the hello answered", request.Header.Get(nodeHeader))
		}
		core.mutex.Lock()
		drop := core.reportsToDrop > 0
		core.reportsToDrop--
		core.mutex.Unlock()
		if drop {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		decoder := json.NewDecoder(request.Body)
		for {
			var record nodeproto.Record
			err := decoder.Decode(&record)
			if isMalformed(err) {
				t.Error(err)
			}
			if err != nil || !handOn(core, core.records, record) {
				return
			}
		}
	}
}

// serve answers on a socket until the test ends. Teardown closes every connection and then
// waits for the handlers to return, so none reports to a test that has ended (which panics).
func (core *fakeCore) serve(t *testing.T, handler http.Handler) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "c.sock")
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var handling sync.WaitGroup
	counted := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		core.mutex.Lock()
		select {
		case <-core.stopped:
			core.mutex.Unlock()
			return
		default:
			handling.Add(1)
		}
		core.mutex.Unlock()
		defer handling.Done()
		handler.ServeHTTP(writer, request)
	})
	server := &http.Server{Handler: counted, ReadHeaderTimeout: time.Minute}
	go func() { _ = server.Serve(listener) }() //nolint:errcheck // ends when the test closes it
	t.Cleanup(func() {
		core.mutex.Lock()
		close(core.stopped)
		core.mutex.Unlock()
		_ = server.Close() //nolint:errcheck // test teardown
		handling.Wait()
	})
	return socket
}

// forwardPosted hands the test each answer of type T the node posts.
func forwardPosted[T any](t *testing.T, core *fakeCore, into chan<- T) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		var answer T
		err := json.NewDecoder(request.Body).Decode(&answer)
		if isMalformed(err) {
			t.Error(err)
		}
		if err != nil || !handOn(core, into, answer) {
			return
		}
		_, _ = writer.Write([]byte("{}")) //nolint:errcheck // a node that went away has no use for it
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
		{Type: nodeproto.RecordScanBegin, ScanBegin: &nodeproto.ScanBegin{RootPath: root.Path()}},
		{Type: nodeproto.RecordScanComplete, ScanComplete: &nodeproto.ScanComplete{RootPath: root.Path()}},
	} {
		if err := emit(record); err != nil {
			return err
		}
	}
	return nil
}

// fakeArtworker answers every artwork command with one size, the source's bytes reversed.
type fakeArtworker struct{}

func (fakeArtworker) Derive(_ context.Context, command nodeproto.Artwork) nodeproto.ArtworkResult {
	reversed := slices.Clone(command.SourceImage)
	slices.Reverse(reversed)
	return nodeproto.ArtworkResult{
		JobID: command.JobID, Images: []nodeproto.ArtImage{{SizeName: nodeproto.PortraitS, WebP: reversed}},
	}
}

func startAgent(t *testing.T, socket string) (*confine.MediaRoot, chan time.Time) {
	t.Helper()
	root, err := confine.OpenMediaRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() }) //nolint:errcheck // test teardown
	now := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	workers := Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Preparer: fakePreparer{}, Subtitler: fakeSubtitler{},
		LoudnessMeasurer: fakeMeasurer{},
	}
	node := New(socket, nodeproto.Hello{Name: "local"}, []*confine.MediaRoot{root}, workers,
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

func receiveRecord(t *testing.T, core *fakeCore) nodeproto.Record {
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
	root, tick := startAgent(t, socket)

	core.commands <- nodeproto.Command{
		CommandSeq: 1, Type: nodeproto.CommandScan,
		Scan: &nodeproto.Scan{RootPath: root.Path()},
	}
	begin, complete := receiveRecord(t, core), receiveRecord(t, core)
	if begin.Type != nodeproto.RecordScanBegin || begin.ReportSeq != 41 ||
		complete.Type != nodeproto.RecordScanComplete ||
		complete.ReportSeq != 42 {
		t.Errorf("heard %s #%d then %s #%d; want the scan numbered on from the core's 40",
			begin.Type, begin.ReportSeq, complete.Type, complete.ReportSeq)
	}
	// A status record reaches the core while the report is still open: each record is sent as
	// it is written, not when the request ends.
	tick <- time.Time{}
	if status := receiveRecord(t, core); status.Type != nodeproto.RecordStatus || status.ReportSeq != 43 ||
		status.Status.Activity != "idle" {
		t.Errorf("heard %+v, want status #43, idle", status)
	}
	core.mutex.Lock()
	hello := core.hellos[0]
	core.mutex.Unlock()
	if hello.Name != "local" || len(hello.RootPaths) != 1 || hello.RootPaths[0] != root.Path() {
		t.Errorf("the hello was %+v, want the node's name and its root", hello)
	}
}

func TestAgent_RefusesToScanARootItWasNotGiven(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	_, tick := startAgent(t, socket)
	core.commands <- nodeproto.Command{
		CommandSeq: 1, Type: nodeproto.CommandScan,
		Scan: &nodeproto.Scan{RootPath: "/etc"},
	}
	tick <- time.Time{}
	if record := receiveRecord(t, core); record.Type != nodeproto.RecordStatus {
		t.Errorf("heard %s before the status; a scan of /etc ran (D107)", record.Type)
	}
}

func TestAgent_SaysHelloAgainAfterLosingTheCore(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	core.reportsToDrop = 1
	_, tick := startAgent(t, socket)
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
	if record := receiveRecord(t, core); record.Type != nodeproto.RecordStatus {
		t.Errorf("heard %s, want a status in the second session", record.Type)
	}
	if count := core.helloCount(); count != 2 {
		t.Errorf("%d hellos, want 2: one per session", count)
	}
}

func TestAgent_ReEncodesArtworkAndPostsTheAnswer(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgent(t, socket)
	artwork := &nodeproto.Artwork{
		JobID: 12, SourceImage: []byte("abc"), Shape: nodeproto.ShapePortrait,
		SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
	}
	core.commands <- nodeproto.Command{CommandSeq: 1, Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: artwork}
	select {
	case result := <-core.artwork:
		if result.JobID != 12 || len(result.Images) != 1 || string(result.Images[0].WebP) != "cba" {
			t.Errorf("the core heard %+v", result)
		}
	case <-t.Context().Done():
		t.Fatal("no artwork answer reached the core")
	}
}

func TestAgent_ReadsAnImageBesideTheMediaOnlyUnderItsRoots(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	if err := os.WriteFile(filepath.Join(root.Path(), "poster.jpg"), []byte("xyz"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.jpg")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	answer := func(job int64, named string) nodeproto.ArtworkResult {
		t.Helper()
		core.commands <- nodeproto.Command{
			CommandSeq: job, Type: nodeproto.CommandEncodeArtwork,
			EncodeArtwork: &nodeproto.Artwork{
				JobID: job, AbsolutePath: &nodeproto.Path{Text: named}, Shape: nodeproto.ShapePortrait,
				SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
			},
		}
		select {
		case result := <-core.artwork:
			return result
		case <-t.Context().Done():
			t.Fatal("no artwork answer reached the core")
			return nodeproto.ArtworkResult{}
		}
	}
	sum := sha256.Sum256([]byte("xyz"))
	if beside := answer(1, filepath.Join(root.Path(), "poster.jpg")); beside.Refusal != "" ||
		string(beside.Images[0].WebP) != "zyx" || beside.SourceSha256 != hex.EncodeToString(sum[:]) {
		t.Errorf("an image under the root: %+v; want it read, re-encoded and hashed", beside)
	}
	for job, named := range map[int64]string{
		2: outside, 3: filepath.Join(root.Path(), "..", filepath.Base(filepath.Dir(outside)), "secret.jpg"),
	} {
		if refused := answer(job, named); refused.Refusal == "" || len(refused.Images) != 0 {
			t.Errorf("%s: %+v; a path outside the node's roots is refused", named, refused)
		}
	}
}

// fakePreparedShelf is a prepared shelf whose last eviction left usage.
type fakePreparedShelf struct{ usage prepared.Usage }

func (kept fakePreparedShelf) Evict() (prepared.Usage, error)    { return kept.usage, nil }
func (kept fakePreparedShelf) LastUsage() (prepared.Usage, bool) { return kept.usage, true }

func TestStatus_CarriesWhatThePreparedDirectoryHolds(t *testing.T) {
	t.Parallel()
	usage := prepared.Usage{UsedBytes: 10, BudgetBytes: 100, DiskUsedPercent: 50, DiskUsedLimitPercent: 95}
	node := New("/nowhere", nodeproto.Hello{Name: "local"}, nil, Workers{PreparedShelf: fakePreparedShelf{usage}},
		clocktest.NewFake(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	got := node.status().Status.PreparedUsage
	want := nodeproto.PreparedUsage{UsedBytes: 10, BudgetBytes: 100, DiskUsedPercent: 50, DiskUsedLimitPercent: 95}
	if got == nil || *got != want {
		t.Errorf("the status's prepared usage = %+v, want %+v (D217 §5)", got, want)
	}
}

func TestStatus_CarriesTheDisksOfTheRootsAndThePreparedDirectory(t *testing.T) {
	t.Parallel()
	root, err := confine.OpenMediaRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() }) //nolint:errcheck // test teardown
	gone := filepath.Join(t.TempDir(), "gone")
	node := New("/nowhere", nodeproto.Hello{Name: "local"}, []*confine.MediaRoot{root},
		Workers{PreparedDirectory: gone},
		clocktest.NewFake(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	disks := node.status().Status.Disks
	if len(disks) != 2 || disks[0].Path != root.Path() || disks[0].TotalBytes <= 0 || disks[0].Failure != "" {
		t.Fatalf("the status's disks = %+v; want the root's measured, then the prepared directory's (D225)", disks)
	}
	if disks[1].Path != gone || disks[1].Failure == "" {
		t.Errorf("a prepared directory that cannot be measured = %+v; want why", disks[1])
	}
}
