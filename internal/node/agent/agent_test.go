// SPDX-License-Identifier: AGPL-3.0-or-later

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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/node/prepared"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// fakeCore answers the node channel the way the core does, and hands the test what it hears.
type fakeCore struct {
	mutex    sync.Mutex
	hellos   []nodeproto.Hello
	commands chan nodeproto.Command
	statuses chan nodeproto.Status
	// The node's results, each on its kind's channel: a Result carrying its job id and one payload.
	artwork chan nodeproto.Result
	// progress is the node's words on where its attempts are, dropped when the test reads none.
	progress     chan nodeproto.Progress
	subtitles    chan nodeproto.Result
	readThroughs chan nodeproto.Result
	wholeHashes  chan nodeproto.Result
	chunkRuns    chan nodeproto.Result
	crashes      chan nodeproto.Result
	refusals     chan nodeproto.Result
	held         chan nodeproto.Result
	scans        chan nodeproto.Result
	// changes are the watcher's notices the node posts.
	changes          chan nodeproto.Changed
	statusesToRefuse int                // the first statuses it refuses, as a core that went away would
	postsToRefuse    map[resultKind]int // by kind, the first results it refuses, as a busy core would
	refusedPosts     chan struct{}      // sent on, while it has room, at each result it refuses
	stopped          chan struct{}      // closed at teardown: a handler stops handing on what it heard
	// endSession, sent on, ends the session it reaches: the open long-poll answers 503, as a core
	// that went away does.
	endSession chan struct{}
	// helloHeard is sent on at each hello, while it has room: a test waits on it for a session to begin.
	helloHeard chan struct{}
	// statusEverySeconds is the status cadence the reply to a hello says. Guarded by mutex.
	statusEverySeconds int
	// watchFolders are the library folders the reply to a hello and each poll's answer name. Guarded by
	// mutex.
	watchFolders []string
	// tokenlessJobIDs are the jobs whose commands it sends naming no attempt, as no core does.
	tokenlessJobIDs map[int64]bool
}

// sendsTokenless has the core send job's commands naming no attempt.
func (core *fakeCore) sendsTokenless(job int64) {
	core.mutex.Lock()
	defer core.mutex.Unlock()
	if core.tokenlessJobIDs == nil {
		core.tokenlessJobIDs = map[int64]bool{}
	}
	core.tokenlessJobIDs[job] = true
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
		commands: make(chan nodeproto.Command, 4), statuses: make(chan nodeproto.Status, 64),
		artwork:  make(chan nodeproto.Result, 4),
		progress: make(chan nodeproto.Progress, 64), refusedPosts: make(chan struct{}, 1),
		subtitles: make(chan nodeproto.Result, 4), readThroughs: make(chan nodeproto.Result, 4),
		wholeHashes: make(chan nodeproto.Result, 4),
		chunkRuns:   make(chan nodeproto.Result, 16), crashes: make(chan nodeproto.Result, 4),
		refusals: make(chan nodeproto.Result, 4), held: make(chan nodeproto.Result, 4),
		scans: make(chan nodeproto.Result, 4), changes: make(chan nodeproto.Changed, 4),
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
		statusEvery, watchFolders := core.statusEverySeconds, core.watchFolders
		core.mutex.Unlock()
		select {
		case core.helloHeard <- struct{}{}:
		default:
		}
		if err := json.NewEncoder(writer).Encode(nodeproto.HelloReply{
			NodeID: 7, Tuning: nodeproto.Tuning{StatusEverySeconds: statusEvery}, WatchFolders: watchFolders,
		}); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("GET "+nodeproto.PathCommands, func(writer http.ResponseWriter, request *http.Request) {
		select {
		case command := <-core.commands:
			// Work and its cancel name an attempt, as the core's do: job N's one attempt, unless the
			// test named another.
			core.mutex.Lock()
			isTokenless, watchFolders := core.tokenlessJobIDs[command.JobID], core.watchFolders
			core.mutex.Unlock()
			if command.AttemptToken == "" && !isTokenless {
				command.AttemptToken = tokenOf(command.JobID)
			}
			answer := nodeproto.Commands{Commands: []nodeproto.Command{command}, WatchFolders: watchFolders}
			if err := json.NewEncoder(writer).Encode(answer); err != nil {
				t.Error(err)
			}
		case <-core.endSession:
			writer.WriteHeader(http.StatusServiceUnavailable)
		case <-request.Context().Done():
		case <-core.stopped:
		}
	})
	mux.HandleFunc("POST "+nodeproto.PathStatus, core.receiveStatus(t))
	mux.HandleFunc("POST "+nodeproto.PathResult, core.receiveResult(t))
	mux.HandleFunc("POST "+nodeproto.PathProgress, core.receiveProgress(t))
	mux.HandleFunc("POST "+nodeproto.PathChanged, func(writer http.ResponseWriter, request *http.Request) {
		var changed nodeproto.Changed
		if err := json.NewDecoder(request.Body).Decode(&changed); isMalformed(err) {
			t.Error(err)
		}
		if handOn(core, core.changes, changed) {
			_, _ = writer.Write([]byte("{}")) //nolint:errcheck // a node that went away has no use for it
		}
	})
	socket := core.serve(t, mux)
	return core, socket
}

// receiveStatus hands the test each status the node posts, refusing the first statusesToRefuse.
func (core *fakeCore) receiveStatus(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get(nodeHeader) != "7" {
			t.Errorf("a status named node %q, want the 7 the hello answered", request.Header.Get(nodeHeader))
		}
		core.mutex.Lock()
		refuse := core.statusesToRefuse > 0
		core.statusesToRefuse--
		core.mutex.Unlock()
		if refuse {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var status nodeproto.Status
		err := json.NewDecoder(request.Body).Decode(&status)
		if isMalformed(err) {
			t.Error(err)
		}
		if err != nil || !handOn(core, core.statuses, status) {
			return
		}
		_, _ = writer.Write([]byte("{}")) //nolint:errcheck // a node that went away has no use for it
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
	artworkResult     resultKind = "artwork"
	subtitleResult    resultKind = "subtitle"
	readThroughResult resultKind = "read-through"
	wholeHashResult   resultKind = "whole-hash"
	chunksResult      resultKind = "chunks"
	// A listing and an examining, a scan's work, share one channel.
	scanResult resultKind = "scan"
	// A refusal of a command the node could not read carries no payload.
	refusalResult resultKind = "refusal"
	// A result the node holds, too large to post, is named by the post.
	heldResult resultKind = "held"
	// A job whose work panicked carries only why.
	crashedResult resultKind = "crashed"
)

// kindOf is the one payload a result carries, or a refusal and none; a result with neither, or
// several, is not one the node should post.
func kindOf(result nodeproto.Result) (resultKind, bool) {
	var kinds []resultKind
	for kind, isSet := range map[resultKind]bool{
		artworkResult:  result.Artwork != nil,
		subtitleResult: result.Subtitle != nil, readThroughResult: result.ReadThrough != nil,
		wholeHashResult: result.WholeHash != nil,
		chunksResult:    result.MakeChunks != nil,
		refusalResult:   result.Refusal != "", heldResult: result.Held != nil, crashedResult: result.Crashed != "",
		scanResult: result.Listing != nil || result.Examined != nil,
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
			t.Errorf("the node posted %+v, want a result with exactly one payload, or a refusal alone", result)
			return
		}
		if core.refusesNextPost(kind) {
			http.Error(writer, "the core is busy", http.StatusServiceUnavailable)
			select {
			case core.refusedPosts <- struct{}{}:
			default:
			}
			return
		}
		into := map[resultKind]chan nodeproto.Result{
			artworkResult:  core.artwork,
			subtitleResult: core.subtitles, readThroughResult: core.readThroughs, refusalResult: core.refusals,
			wholeHashResult: core.wholeHashes,
			chunksResult:    core.chunkRuns, crashedResult: core.crashes,
			heldResult: core.held, scanResult: core.scans,
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

// fakeScanner lists and examines as the node's scanner does (listing_test.go).
type fakeScanner struct{}

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
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, Subtitler: fakeSubtitler{},
		ThroughReader: fakeThroughReader{}, ChunkMaker: fakeChunkMaker{}, PreparedShelf: shelf,
	})
}

// startAgentWithWorkers starts an agent with workers, which tries an answer the core did not take
// again within milliseconds, changed by adjustments before it runs.
func startAgentWithWorkers(
	t *testing.T, socket string, workers Workers, adjustments ...func(*Agent),
) (*confine.MediaRoot, chan time.Time) {
	t.Helper()
	root := nodetest.OpenMediaRoot(t, t.TempDir())
	now := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	node := New(socket, nodeproto.Hello{Name: "local", Capacity: testCapacity}, []*confine.MediaRoot{root}, workers,
		now, slog.New(slog.DiscardHandler))
	node.answerRetry = answerRetry{firstWait: time.Millisecond, longestWait: 4 * time.Millisecond}
	for _, adjust := range adjustments {
		adjust(node)
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

func TestAgent_SaysHelloAndPostsItsStatusEachTick(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, tick := startAgent(t, socket)
	for range 2 {
		tick <- time.Time{}
		if status := testutil.Receive(t, core.statuses, "a status"); status.SentAt == 0 {
			t.Errorf("heard %+v, want a status saying when it was sent", status)
		}
	}
	core.mutex.Lock()
	hello := core.hellos[0]
	core.mutex.Unlock()
	if hello.Name != "local" || len(hello.RootPaths) != 1 || hello.RootPaths[0] != root.Path() {
		t.Errorf("the hello was %+v, want the node's name and its root", hello)
	}
}

// keepTicking ticks the node's status loop until the test ends: a session that ends, and the next
// that begins, take ticks the test cannot count.
func keepTicking(t *testing.T, tick chan<- time.Time) {
	t.Helper()
	ticking, stop := context.WithCancel(t.Context())
	t.Cleanup(stop)
	go func() {
		for {
			select {
			case tick <- time.Time{}:
			case <-ticking.Done():
				return
			}
		}
	}()
}

func TestAgent_SaysHelloAgainAfterLosingTheCore(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	core.statusesToRefuse = 1
	_, tick := startAgent(t, socket)
	// Tick until the core hears something: the first status is refused, which ends the session, a
	// tick starts the next, and a later one posts a status in it.
	keepTicking(t, tick)
	testutil.Receive(t, core.statuses, "a status in the second session")
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
		CommandSeq: 1,
		JobID:      12,
		Work:       nodeproto.Work{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: oneImage(artwork)},
	}
	select {
	case result := <-core.artwork:
		made := onlyImage(t, result)
		if result.JobID != 12 || len(made.Images) != 1 || string(made.Images[0].WebP) != "cba" {
			t.Errorf("the core heard %+v", result)
		}
	case <-t.Context().Done():
		t.Fatal("no artwork answer reached the core")
	}
}

// stopWatchingChunkMaker makes every run's first chunk and runs on until it is stopped, and closes
// stopped when it is.
type stopWatchingChunkMaker struct{ stopped chan struct{} }

func (maker stopWatchingChunkMaker) Make(
	ctx context.Context, _ execx.Film, command nodeproto.MakeChunks, onChunk func(n int),
) (int, error) {
	onChunk(command.FirstChunk)
	<-ctx.Done()
	close(maker.stopped)
	return command.FirstChunk, ctx.Err()
}

// One answer the core did not take is tried again, and costs nothing else: the session goes on
// and a run of chunks beside it is not stopped.
func TestAgent_AnArtworkAnswerTheCoreRefusedOnceArrivesOnRetryAndStopsNoOtherWork(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	maker := stopWatchingChunkMaker{stopped: make(chan struct{})}
	root, _ := startAgentWithWorkers(t, socket, Workers{
		Scanner: fakeScanner{}, Artworker: fakeArtworker{}, ChunkMaker: maker,
	})
	core.sendRun(1, chunkableFilm(t, root), "3/chunks-picture/0123456789abcdef")
	if running := nextProgress(t, core); running.State != nodeproto.ProgressRunning {
		t.Fatalf("the run's first report = %+v, want running", running)
	}
	core.refuseFirstPosts(artworkResult, 1)
	artwork := &nodeproto.Artwork{
		SourceImage: []byte("abc"), Shape: nodeproto.ShapePortrait,
		SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
	}
	core.commands <- nodeproto.Command{
		CommandSeq: 2,
		JobID:      12,
		Work:       nodeproto.Work{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: oneImage(artwork)},
	}
	select {
	case result := <-core.artwork:
		if result.JobID != 12 || len(onlyImage(t, result).Images) != 1 {
			t.Errorf("the core heard %+v, want job 12's image", result)
		}
	case <-time.After(answerTimeout):
		t.Fatal("the artwork answer the core refused once never reached it")
	}
	select {
	case <-maker.stopped:
		t.Error("the running run was stopped by one answer the core did not take")
	default:
	}
	if count := core.helloCount(); count != 1 {
		t.Errorf("%d hellos, want 1: one answer the core did not take ends no session", count)
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
			CommandSeq: job,
			JobID:      job,
			Work: nodeproto.Work{Type: nodeproto.CommandEncodeArtwork, EncodeArtwork: oneImage(&nodeproto.Artwork{
				AbsolutePath: &nodeproto.Path{Text: named}, Shape: nodeproto.ShapePortrait,
				SizeNames: []nodeproto.ArtSizeName{nodeproto.PortraitS},
			})},
		}
		select {
		case result := <-core.artwork:
			if result.JobID != job {
				t.Errorf("the core heard job %d's image, want job %d's", result.JobID, job)
			}
			return onlyImage(t, result)
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

func TestStatus_CarriesWhatThePreparedDirectoryHolds(t *testing.T) {
	t.Parallel()
	usage := prepared.Usage{UsedBytes: 10, BudgetBytes: 100, DiskUsedPercent: 50, DiskUsedLimitPercent: 95}
	node := New("/nowhere", nodeproto.Hello{Name: "local"}, nil, Workers{PreparedShelf: fakePreparedShelf{usage}},
		clocktest.NewFake(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)), slog.New(slog.DiscardHandler))
	got := node.status(t.Context()).PreparedUsage
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
	disks := node.status(t.Context()).Disks
	if len(disks) != 2 || disks[0].Path != root.Path() || disks[0].TotalBytes <= 0 || disks[0].Failure != "" {
		t.Fatalf("the status's disks = %+v; want the root's measured, then the prepared directory's (D225)", disks)
	}
	if disks[1].Path != gone || disks[1].Failure == "" {
		t.Errorf("a prepared directory that cannot be measured = %+v; want why", disks[1])
	}
}

// releasedChunkMaker makes every run's first chunk and runs on until the test releases it, saying
// when it started, and says whether one was stopped instead.
type releasedChunkMaker struct {
	started, release chan struct{}
	stopped          *atomic.Bool
}

func (maker releasedChunkMaker) Make(
	ctx context.Context, _ execx.Film, command nodeproto.MakeChunks, onChunk func(n int),
) (int, error) {
	onChunk(command.FirstChunk)
	close(maker.started)
	select {
	case <-maker.release:
		return command.FirstChunk, nil
	case <-ctx.Done():
		maker.stopped.Store(true)
		return command.FirstChunk, ctx.Err()
	}
}

// A session that ends, the core gone or restarting, stops none of the node's work: the run of chunks
// goes on to its end, and its result reaches the core of the next session (D233 §2).
func TestAgent_ASessionThatEndsStopsNoWorkAndItsResultReachesTheNextSession(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	maker := releasedChunkMaker{
		started: make(chan struct{}), release: make(chan struct{}), stopped: &atomic.Bool{},
	}
	root, tick := startAgentWithWorkers(t, socket, Workers{Scanner: fakeScanner{}, ChunkMaker: maker})
	core.sendRun(9, chunkableFilm(t, root), "3/chunks-picture/0123456789abcdef")
	testutil.Receive(t, maker.started, "the run to start")
	select {
	case core.endSession <- struct{}{}: // the open long-poll answers 503, which ends the session
	case <-time.After(testutil.ReceiveLimit):
		t.Fatal("the session's long-poll never reached the core to be ended")
	}
	// The next tick starts the next session; the run ends only once it has.
	testutil.Receive(t, core.helloHeard, "the first session's hello")
	keepTicking(t, tick)
	testutil.Receive(t, core.helloHeard, "the next session's hello")
	close(maker.release)
	if done := finalRunReport(t, core); done.JobID != 9 || done.State != nodeproto.RunDone {
		t.Errorf("the run's end = %+v, want job 9 done", done)
	}
	if maker.stopped.Load() {
		t.Error("the session's end stopped the run")
	}
}

// A command the node cannot read, unknown or without its payload, from a core of another build, is
// refused by its job's id rather than ignored or crashing the node, and the node says hello in its
// own protocol version (D233 §9, S89).
func TestAgent_RefusesACommandItCannotReadAndSaysItsProtocolInItsHello(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	startAgent(t, socket)
	core.commands <- nodeproto.Command{CommandSeq: 1, JobID: 31, Work: nodeproto.Work{Type: "transcode_to_av2"}}
	core.commands <- nodeproto.Command{
		CommandSeq: 2,
		JobID:      32,
		Work:       nodeproto.Work{Type: nodeproto.CommandList},
	} // no payload
	// Each refusal is posted beside the node's other work, so they may arrive in either order.
	var refusedJobIDs []int64
	for range 2 {
		refused := testutil.Receive(t, core.refusals, "the refusal of a command the node cannot read")
		if !strings.Contains(refused.Refusal, "cannot read the command") {
			t.Errorf("refusal %+v, want it to say the node cannot read its command", refused)
		}
		refusedJobIDs = append(refusedJobIDs, refused.JobID)
	}
	slices.Sort(refusedJobIDs)
	if !slices.Equal(refusedJobIDs, []int64{31, 32}) {
		t.Errorf("refused jobs %v, want 31 and 32, each by its own id", refusedJobIDs)
	}
	core.mutex.Lock()
	defer core.mutex.Unlock()
	if core.hellos[0].ProtocolVersion != nodeproto.ProtocolVersion {
		t.Errorf("the hello said protocol %d, want this build's %d", core.hellos[0].ProtocolVersion,
			nodeproto.ProtocolVersion)
	}
}

// A status says the last command taken before the work held, so a command taken while the held
// work is listed is not said taken: said taken and not held, the core judges it lost (D234 §6).
func TestTakenThenHeld_NeverSaysACommandTakenThatTheHeldWorkMisses(t *testing.T) {
	t.Parallel()
	var taken atomic.Int64
	taken.Store(24)
	seq, held := takenThenHeld(&taken, func() []nodeproto.HeldAttempt {
		taken.Store(25) // command 25 is taken and held just after the work is listed
		return []nodeproto.HeldAttempt{{JobID: 24, AttemptToken: "t24"}}
	})
	if seq != 24 || len(held) != 1 {
		t.Errorf("the status says command %d taken, holding %v; want 24, whose work it lists", seq, held)
	}
}
