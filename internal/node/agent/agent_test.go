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
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/node/scan"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeCore answers the node channel the way the core does, and hands the test what it hears.
type fakeCore struct {
	mutex    sync.Mutex
	hellos   []nodeproto.Hello
	commands chan nodeproto.Command
	records  chan nodeproto.Record
	// The node's results, each on its kind's channel: a Result carrying its job id and one payload.
	artwork        chan nodeproto.Result
	prepareReports chan nodeproto.Result
	subtitles      chan nodeproto.Result
	loudness       chan nodeproto.Result
	reportsToDrop  int                // the first reports it ends at once, as a core that went away would
	postsToRefuse  map[resultKind]int // by kind, the first results it refuses, as a busy core would
	stopped        chan struct{}      // closed at teardown: a handler stops handing on what it heard
	// endSession, sent on, ends the session it reaches: the open long-poll answers 503, as a core
	// that went away does.
	endSession chan struct{}
	// helloHeard is sent on at each hello, while it has room: a test waits on it for a session to begin.
	helloHeard chan struct{}
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
		artwork: make(chan nodeproto.Result, 4), prepareReports: make(chan nodeproto.Result, 16),
		subtitles: make(chan nodeproto.Result, 4), loudness: make(chan nodeproto.Result, 4),
		stopped: make(chan struct{}), endSession: make(chan struct{}), helloHeard: make(chan struct{}, 8),
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
		select {
		case core.helloHeard <- struct{}{}:
		default:
		}
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
		case <-core.endSession:
			writer.WriteHeader(http.StatusServiceUnavailable)
		case <-request.Context().Done():
		case <-core.stopped:
		}
	})
	mux.HandleFunc("POST "+nodeproto.PathReport, core.receiveReport(t))
	mux.HandleFunc("POST "+nodeproto.PathResult, core.receiveResult(t))
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

// resultKind is which payload a result carries.
type resultKind string

// The kinds of result.
const (
	artworkResult  resultKind = "artwork"
	prepareResult  resultKind = "prepare"
	subtitleResult resultKind = "subtitle"
	loudnessResult resultKind = "loudness"
)

// kindOf is the one payload a result carries; a result with none or several is not one the node
// should post.
func kindOf(result nodeproto.Result) (resultKind, bool) {
	var kinds []resultKind
	for kind, isSet := range map[resultKind]bool{
		artworkResult: result.Artwork != nil, prepareResult: result.Prepare != nil,
		subtitleResult: result.Subtitle != nil, loudnessResult: result.Loudness != nil,
	} {
		if isSet {
			kinds = append(kinds, kind)
		}
	}
	if len(kinds) != 1 {
		return "", false
	}
	return kinds[0], true
}

// receiveResult hands the test each result the node posts, on its kind's channel, past the first
// postsToRefuse of its kind.
func (core *fakeCore) receiveResult(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		var result nodeproto.Result
		err := json.NewDecoder(request.Body).Decode(&result)
		if isMalformed(err) {
			t.Error(err)
		}
		if err != nil {
			return
		}
		kind, isOne := kindOf(result)
		if !isOne {
			t.Errorf("the node posted %+v, want a result with exactly one payload", result)
			return
		}
		if core.refusesNextPost(kind) {
			http.Error(writer, "the core is busy", http.StatusServiceUnavailable)
			return
		}
		into := map[resultKind]chan nodeproto.Result{
			artworkResult: core.artwork, prepareResult: core.prepareReports,
			subtitleResult: core.subtitles, loudnessResult: core.loudness,
		}[kind]
		if !handOn(core, into, result) {
			return
		}
		_, _ = writer.Write([]byte("{}")) //nolint:errcheck // a node that went away has no use for it
	}
}

// refuseFirstPosts has the core refuse the next count results of kind.
func (core *fakeCore) refuseFirstPosts(kind resultKind, count int) {
	core.mutex.Lock()
	defer core.mutex.Unlock()
	if core.postsToRefuse == nil {
		core.postsToRefuse = map[resultKind]int{}
	}
	core.postsToRefuse[kind] = count
}

// refusesNextPost says whether the core refuses this result of kind, and counts it.
func (core *fakeCore) refusesNextPost(kind resultKind) bool {
	core.mutex.Lock()
	defer core.mutex.Unlock()
	if core.postsToRefuse[kind] <= 0 {
		return false
	}
	core.postsToRefuse[kind]--
	return true
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

// fakeArtworker answers every artwork command with one size, the source's bytes reversed, and a
// cancelled one with a refusal.
type fakeArtworker struct{}

func (fakeArtworker) Derive(ctx context.Context, command nodeproto.Artwork) nodeproto.ArtworkResult {
	if ctx.Err() != nil {
		return nodeproto.ArtworkResult{Refusal: "cancelled"}
	}
	reversed := slices.Clone(command.SourceImage)
	slices.Reverse(reversed)
	return nodeproto.ArtworkResult{Images: []nodeproto.ArtImage{{SizeName: nodeproto.PortraitS, WebP: reversed}}}
}

// testCapacity is the test node's: one encode and two remuxes at once.
var testCapacity = nodeproto.Capacity{EncodeSlotCount: 1, RemuxSlotCount: 2, SlowestReadBytesPerSecond: 20 << 20}

func startAgent(t *testing.T, socket string) (*confine.MediaRoot, chan time.Time) {
	t.Helper()
	return startAgentWithShelf(t, socket, nil)
}

// startAgentWithShelf is startAgent with a prepared shelf, which may be nil.
func startAgentWithShelf(t *testing.T, socket string, shelf PreparedShelf) (*confine.MediaRoot, chan time.Time) {
	t.Helper()
	return startAgentWithWorkers(t, socket, Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Preparer: fakePreparer{}, Subtitler: fakeSubtitler{},
		LoudnessMeasurer: fakeMeasurer{}, PreparedShelf: shelf,
	})
}

// startAgentWithWorkers starts an agent with workers, which tries an answer the core did not take
// again within milliseconds.
func startAgentWithWorkers(t *testing.T, socket string, workers Workers) (*confine.MediaRoot, chan time.Time) {
	t.Helper()
	root := nodetest.OpenMediaRoot(t, t.TempDir())
	now := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	node := New(socket, nodeproto.Hello{Name: "local", Capacity: testCapacity}, []*confine.MediaRoot{root}, workers,
		now, slog.New(slog.DiscardHandler))
	node.answerRetry = answerRetry{
		firstWait: time.Millisecond, longestWait: 4 * time.Millisecond, patience: time.Second,
	}
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1)
	go func() { finished <- node.Run(ctx, tick) }()
	t.Cleanup(func() {
		cancel()
		if err := testutil.Receive(t, finished, "the node's run to end"); err != nil {
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
		SourceImage: []byte("abc"), Shape: nodeproto.ShapePortrait,
		SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
	}
	core.commands <- nodeproto.Command{
		CommandSeq: 1, Type: nodeproto.CommandEncodeArtwork, JobID: 12, EncodeArtwork: artwork,
	}
	select {
	case result := <-core.artwork:
		if result.JobID != 12 || len(result.Artwork.Images) != 1 || string(result.Artwork.Images[0].WebP) != "cba" {
			t.Errorf("the core heard %+v", result)
		}
	case <-t.Context().Done():
		t.Fatal("no artwork answer reached the core")
	}
}

// stopWatchingPreparer runs every prepare until it is stopped, and closes stopped when it is.
type stopWatchingPreparer struct{ stopped chan struct{} }

func (preparer stopWatchingPreparer) Prepare(
	ctx context.Context, _ string, _ nodeproto.Prepare, progress func(preparedThroughMs int64),
) (prepare.Made, error) {
	progress(500)
	<-ctx.Done()
	close(preparer.stopped)
	return prepare.Made{}, ctx.Err()
}

// One answer the core did not take is tried again, and costs nothing else: the session goes on
// and a prepare running beside it is not stopped.
func TestAgent_AnArtworkAnswerTheCoreRefusedOnceArrivesOnRetryAndStopsNoOtherWork(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	preparer := stopWatchingPreparer{stopped: make(chan struct{})}
	root, _ := startAgentWithWorkers(t, socket, Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Preparer: preparer,
	})
	core.sendPrepare(1, preparableFilm(t, root), "3/remux/0123456789abcdef")
	if running := nextPrepareReport(t, core); running.State != nodeproto.PrepareRunning {
		t.Fatalf("the prepare's first report = %+v, want running", running)
	}
	core.refuseFirstPosts(artworkResult, 1)
	artwork := &nodeproto.Artwork{
		SourceImage: []byte("abc"), Shape: nodeproto.ShapePortrait,
		SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
	}
	core.commands <- nodeproto.Command{
		CommandSeq: 2, Type: nodeproto.CommandEncodeArtwork, JobID: 12, EncodeArtwork: artwork,
	}
	select {
	case result := <-core.artwork:
		if result.JobID != 12 || len(result.Artwork.Images) != 1 {
			t.Errorf("the core heard %+v, want job 12's image", result)
		}
	case <-time.After(answerTimeout):
		t.Fatal("the artwork answer the core refused once never reached it")
	}
	select {
	case <-preparer.stopped:
		t.Error("the running prepare was stopped by one answer the core did not take")
	default:
	}
	if count := core.helloCount(); count != 1 {
		t.Errorf("%d hellos, want 1: one answer the core did not take ends no session", count)
	}
}

func TestAgent_AnArtworkJobTheCoreCancelledIsStopped(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgent(t, socket)
	core.commands <- nodeproto.Command{CommandSeq: 1, Type: nodeproto.CommandCancel, JobID: 12}
	artwork := &nodeproto.Artwork{
		SourceImage: []byte("abc"), Shape: nodeproto.ShapePortrait,
		SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
	}
	core.commands <- nodeproto.Command{
		CommandSeq: 2, Type: nodeproto.CommandEncodeArtwork, JobID: 12, EncodeArtwork: artwork,
	}
	select {
	case result := <-core.artwork:
		if result.JobID != 12 || result.Artwork.Refusal != "cancelled" || len(result.Artwork.Images) != 0 {
			t.Errorf("the core heard %+v, want job 12 stopped before it re-encoded anything", result)
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
			CommandSeq: job, Type: nodeproto.CommandEncodeArtwork, JobID: job,
			EncodeArtwork: &nodeproto.Artwork{
				AbsolutePath: &nodeproto.Path{Text: named}, Shape: nodeproto.ShapePortrait,
				SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
			},
		}
		select {
		case result := <-core.artwork:
			if result.JobID != job {
				t.Errorf("the core heard job %d's image, want job %d's", result.JobID, job)
			}
			return *result.Artwork
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

func TestReadImage_ReadsAnImageUnderARootOfSlash(t *testing.T) {
	t.Parallel()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "poster.jpg"), []byte("xyz"), 0o600); err != nil {
		t.Fatal(err)
	}
	slash := nodetest.OpenMediaRoot(t, "/")
	node := New("/nowhere", nodeproto.Hello{Name: "local"}, []*confine.MediaRoot{slash}, Workers{},
		clocktest.NewFake(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	image, err := node.readImage(nodeproto.Path{Text: filepath.Join(directory, "poster.jpg")})
	if err != nil || string(image) != "xyz" {
		t.Errorf("an image under the root / read as %q, %v; want its bytes", image, err)
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
	root := nodetest.OpenMediaRoot(t, t.TempDir())
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

// releasedPreparer runs every prepare until the test releases it, saying when it started, and
// says whether one was stopped instead.
type releasedPreparer struct {
	started, release chan struct{}
	stopped          *atomic.Bool
}

func (preparer releasedPreparer) Prepare(
	ctx context.Context, _ string, _ nodeproto.Prepare, progress func(preparedThroughMs int64),
) (prepare.Made, error) {
	progress(500)
	close(preparer.started)
	select {
	case <-preparer.release:
		return prepare.Made{SizeBytes: 9, VideoEncoder: "copy"}, nil
	case <-ctx.Done():
		preparer.stopped.Store(true)
		return prepare.Made{}, ctx.Err()
	}
}

// A session that ends, the core gone or restarting, stops none of the node's work: the prepare
// runs on to its end, and its result reaches the core of the next session (D233 §2).
func TestAgent_ASessionThatEndsStopsNoWorkAndItsResultReachesTheNextSession(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	preparer := releasedPreparer{
		started: make(chan struct{}), release: make(chan struct{}), stopped: &atomic.Bool{},
	}
	root, tick := startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, Preparer: preparer})
	if err := os.WriteFile(filepath.Join(root.Path(), "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
	}
	payload := &nodeproto.Prepare{
		Video: source, Profile: nodeproto.ProfileRemux, RenditionDir: "3/remux/0123456789abcdef",
	}
	core.commands <- nodeproto.Command{CommandSeq: 1, Type: nodeproto.CommandPrepare, JobID: 9, Prepare: payload}
	testutil.Receive(t, preparer.started, "the prepare to start")
	select {
	case core.endSession <- struct{}{}: // the open long-poll answers 503, which ends the session
	case <-time.After(testutil.ReceiveLimit):
		t.Fatal("the session's long-poll never reached the core to be ended")
	}
	// The next tick starts the next session; the prepare ends only once it has.
	testutil.Receive(t, core.helloHeard, "the first session's hello")
	ticking, stopTicking := context.WithCancel(t.Context())
	defer stopTicking()
	go func() {
		for {
			select {
			case tick <- time.Time{}:
			case <-ticking.Done():
				return
			}
		}
	}()
	testutil.Receive(t, core.helloHeard, "the next session's hello")
	close(preparer.release)
	if done := finalPrepareReport(t, core); done.JobID != 9 || done.State != nodeproto.PrepareDone {
		t.Errorf("the prepare's end = %+v, want job 9 done", done)
	}
	if preparer.stopped.Load() {
		t.Error("the session's end stopped the prepare")
	}
}
