// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/host"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// recordingDestinations are the job queue's parked destinations, keeping each one unparked.
type recordingDestinations struct {
	mutex    sync.Mutex
	unparked []jobs.Destination
}

func (destinations *recordingDestinations) Unpark(_ context.Context, destination jobs.Destination) {
	destinations.mutex.Lock()
	defer destinations.mutex.Unlock()
	destinations.unparked = append(destinations.unparked, destination)
}

func (destinations *recordingDestinations) all() []jobs.Destination {
	destinations.mutex.Lock()
	defer destinations.mutex.Unlock()
	return slices.Clone(destinations.unparked)
}

type fixture struct {
	database     *store.Database
	settings     *settings.Service
	service      *Service
	destinations *recordingDestinations
	server       *httptest.Server
	clock        *clocktest.Fake
}

func newFixture(t *testing.T, pollHold time.Duration) *fixture {
	t.Helper()
	database := storetest.OpenMigrated(t)
	settingsService := settingstest.Seeded(t, database)
	fakeClock := clocktest.NewFake(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))
	destinations := &recordingDestinations{}
	service := NewService(ServiceParts{
		Database: database, Clock: fakeClock, Settings: settingsService, Destinations: destinations,
		Logger: slog.New(slog.DiscardHandler), PollHold: pollHold,
	})
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	return &fixture{
		database: database, settings: settingsService, service: service, destinations: destinations,
		server: server, clock: fakeClock,
	}
}

// httpAnswer is a response, read and closed.
type httpAnswer struct {
	status int
	body   []byte
}

func (harness *fixture) post(t *testing.T, path string, nodeID int64, body io.Reader) httpAnswer {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, harness.server.URL+path, body)
	if err != nil {
		t.Error(err)
		return httpAnswer{}
	}
	if nodeID != 0 {
		request.Header.Set(NodeIDHeader, strconv.FormatInt(nodeID, 10))
	}
	response, err := harness.server.Client().Do(request)
	if err != nil {
		t.Error(err)
		return httpAnswer{}
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	read, err := io.ReadAll(response.Body)
	if err != nil {
		t.Error(err)
	}
	return httpAnswer{status: response.StatusCode, body: read}
}

// hello says a node's hello, in this build's protocol unless it names one, as a node's agent does.
func (harness *fixture) hello(t *testing.T, hello nodeproto.Hello) nodeproto.HelloReply {
	t.Helper()
	if hello.ProtocolVersion == 0 {
		hello.ProtocolVersion = nodeproto.ProtocolVersion
	}
	if hello.ByteSocketPath == "" {
		hello.ByteSocketPath = "/run/orpheum-test/node.sock"
	}
	encoded, err := json.Marshal(hello)
	if err != nil {
		t.Fatal(err)
	}
	response := harness.post(t, nodeproto.PathHello, 0, bytes.NewReader(encoded))
	if response.status != http.StatusOK {
		t.Fatalf("hello answered %d %s", response.status, response.body)
	}
	var reply nodeproto.HelloReply
	if err := json.Unmarshal(response.body, &reply); err != nil {
		t.Fatal(err)
	}
	return reply
}

func (harness *fixture) poll(t *testing.T, nodeID, after int64) (nodeproto.Commands, int) {
	t.Helper()
	url := fmt.Sprintf("%s%s?%s=%d", harness.server.URL, nodeproto.PathCommands, nodeproto.QueryAfterCommandSeq, after)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(NodeIDHeader, strconv.FormatInt(nodeID, 10))
	response, err := harness.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read-only
	var commands nodeproto.Commands
	if response.StatusCode == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&commands); err != nil {
			t.Fatal(err)
		}
	}
	return commands, response.StatusCode
}

// pollThroughHold is a long-poll with nothing to answer: the clock moves on past the poll's hold,
// which runs on the core's clock, until the poll answers.
func (harness *fixture) pollThroughHold(t *testing.T, nodeID, after int64) (nodeproto.Commands, int) {
	t.Helper()
	type answer struct {
		commands nodeproto.Commands
		status   int
	}
	answered := make(chan answer, 1)
	go func() {
		commands, status := harness.poll(t, nodeID, after)
		answered <- answer{commands, status}
	}()
	for len(answered) == 0 {
		harness.clock.Advance(harness.service.pollHold)
		runtime.Gosched()
	}
	got := testutil.Receive(t, answered, "the long-poll's empty answer")
	return got.commands, got.status
}

// A hello scans each of the node's roots a library covers, and no other: a folder the node can
// read is scanned only once it is in a library, which says what kind of thing it holds (the owner,
// 2026-10-02).
func TestHello_EnrolsANodeByNameOnceAndQueuesAScanOfEachRootALibraryCovers(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	local := librarytest.Node(t, harness.database, "local")
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/media/films"})
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Shows", Kind: "shows", MetadataSource: "nfo", Root: "/media/shows"})
	scans := &recordingScans{}
	harness.service.SetScans(scans)
	first := harness.hello(t, nodeproto.Hello{
		Name: "local", RootPaths: []string{"/media/films", "/media/everything", "/media/shows"},
	})
	again := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films"}})
	other := harness.hello(t, nodeproto.Hello{Name: "attic", RootPaths: nil})
	if first.NodeID == 0 || again.NodeID != first.NodeID || other.NodeID == first.NodeID {
		t.Errorf("node ids %d, %d, %d: want one id per name (D207)", first.NodeID, again.NodeID, other.NodeID)
	}

	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	if strings.Join(scans.roots, ",") != "/media/films,/media/shows,/media/films" ||
		slices.ContainsFunc(scans.priorities, func(priority int64) bool {
			return priority != jobs.PriorityBackground
		}) {
		t.Errorf("scans asked for of %q at %v; want one per root a library covers in each hello, no person waiting",
			scans.roots, scans.priorities)
	}
}

// A hello that no longer names a library root on its node makes it unavailable, whether or not a
// hello asks for scans; one that names it again asks for its scan, which brings it back (D241 §2).
func TestHello_MakesALibraryRootItNoLongerNamesUnavailable(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	local := librarytest.Node(t, harness.database, "local")
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/media/films"})
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Shows", Kind: "shows", MetadataSource: "nfo", Root: "/media/shows"})
	scans := &recordingScans{}
	harness.service.SetScans(scans)
	harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films", "/media/shows"}})
	harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/mnt/films", "/media/shows"}})
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	if !slices.Equal(scans.notNamed, []string{"/media/films"}) {
		t.Errorf("roots made unavailable: %q; want /media/films, once, when the second hello named it no more",
			scans.notNamed)
	}
}

// recordingScans keeps the scans asked of it, and the roots made unavailable for a hello not naming
// them.
type recordingScans struct {
	mutex      sync.Mutex
	roots      []string
	priorities []int64
	notNamed   []string
}

func (scans *recordingScans) MarkRootNotNamed(_ context.Context, _ int64, root string) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	scans.notNamed = append(scans.notNamed, root)
	return nil
}

func (scans *recordingScans) Request(_ context.Context, _ int64, root string, priority int64) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	scans.roots = append(scans.roots, root)
	scans.priorities = append(scans.priorities, priority)
	return nil
}

func TestHello_ScansNothingWhenTheSettingSaysNot(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	local := librarytest.Node(t, harness.database, "local")
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/media/films"})
	scans := &recordingScans{}
	harness.service.SetScans(scans)
	harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films"}})
	if len(scans.roots) != 0 {
		t.Errorf("scans asked for of %v, want none", scans.roots)
	}
}

// cancelOf is a command the core sends as it is: a cancel of job jobID's attempt.
func cancelOf(jobID int64) nodeproto.Command {
	return nodeproto.Command{JobID: jobID, AttemptToken: "attempt", Work: nodeproto.Work{Type: nodeproto.CommandCancel}}
}

func TestCommands_ALongPollWaitsForACommandAndAnAcknowledgedOneIsDropped(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Hour)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})

	got := make(chan nodeproto.Commands)
	go func() {
		commands, _ := harness.poll(t, reply.NodeID, 0)
		got <- commands
	}()
	// The poll is held until a command is queued; Enqueue before the poll starts is also
	// answered at once, so the order of these two does not matter.
	harness.service.Enqueue(reply.NodeID, cancelOf(1))
	first := testutil.Receive(t, got, "the long-poll's answer")
	if len(first.Commands) != 1 || first.Commands[0].JobID != 1 {
		t.Fatalf("the long-poll answered %+v, want the cancel of job 1", first)
	}
	harness.service.Enqueue(reply.NodeID, cancelOf(2))
	commands, _ := harness.poll(t, reply.NodeID, 1)
	if len(commands.Commands) != 1 || commands.Commands[0].CommandSeq != 2 {
		t.Errorf("after acknowledging 1: %+v, want only command 2", commands.Commands)
	}
}

func TestCommands_ALongPollWithNothingToSayAnswersEmpty(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	commands, status := harness.pollThroughHold(t, reply.NodeID, 0)
	if status != http.StatusOK || commands.Commands == nil || len(commands.Commands) != 0 {
		t.Errorf("status %d, %+v; want 200 and an empty list", status, commands)
	}
}

func TestConnectedNode_ANodeMustSayHelloFirst(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	if _, status := harness.poll(t, 99, 0); status != http.StatusConflict {
		t.Errorf("a poll from an unknown node: %d, want 409", status)
	}
	response := harness.post(t, nodeproto.PathStatus, 0, strings.NewReader("{}"))
	if response.status != http.StatusBadRequest {
		t.Errorf("a status without the node header: %d, want 400", response.status)
	}
}

// postStatus posts a node's status, failing the test unless it is taken.
func (harness *fixture) postStatus(t *testing.T, nodeID int64, status nodeproto.Status) {
	t.Helper()
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if response := harness.post(t, nodeproto.PathStatus, nodeID, bytes.NewReader(encoded)); response.status !=
		http.StatusOK {
		t.Fatalf("the status answered %d %s", response.status, response.body)
	}
}

// Each status is taken as it arrives: the node is heard, and its last status kept (D234 §6).
func TestStatus_TheLastIsKeptAndTheNodeHeard(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	for _, sentAt := range []int64{7, 8} {
		harness.postStatus(t, reply.NodeID, nodeproto.Status{SentAt: sentAt})
	}
	if facts, _ := harness.service.Facts("local"); facts.Status == nil || facts.Status.SentAt != 8 {
		t.Errorf("the status kept in memory is %+v, want the last one posted", facts.Status)
	}
	if heard, ever := harness.service.LastHeard("local"); !ever || !heard.Equal(harness.clock.Now()) {
		t.Errorf("LastHeard = %v, %v", heard, ever)
	}
}

func TestStatus_ABodyThatIsNotAStatusIsRefused(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	response := harness.post(t, nodeproto.PathStatus, reply.NodeID, strings.NewReader(`{"sentAt":"soon"}`))
	if response.status != http.StatusBadRequest || !strings.Contains(string(response.body), "a status is JSON") {
		t.Errorf("a status that is not one: %d %q, want a 400 saying why", response.status, response.body)
	}
}

// takenChanges keeps the watchers' notices the channel hands on.
type takenChanges struct {
	mutex   sync.Mutex
	notices []nodeproto.Changed
}

func (taken *takenChanges) TakeChange(_ context.Context, _ int64, root, folder string) error {
	taken.mutex.Lock()
	defer taken.mutex.Unlock()
	taken.notices = append(taken.notices, nodeproto.Changed{RootPath: root, Path: folder})
	return nil
}

// A node watcher's notice is handed on, and each root's watch its status says is kept (D272 §9).
func TestChanged_AWatchersNoticeIsHandedOnAndItsRootsWatchesKept(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	taken := &takenChanges{}
	harness.service.SetChanges(taken)
	reply := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media"}})
	watch := nodeproto.RootWatch{RootPath: "/media", IsWatching: true}
	harness.postStatus(t, reply.NodeID, nodeproto.Status{SentAt: 1, Watches: []nodeproto.RootWatch{watch}})
	if got := harness.service.RootWatches(reply.NodeID); len(got) != 1 || got["/media"] != watch {
		t.Errorf("the node's roots' watches are %+v, want %+v", got, watch)
	}
	response := harness.post(t, nodeproto.PathChanged, reply.NodeID,
		strings.NewReader(`{"rootPath":"/media","path":"Shows/Dag"}`))
	taken.mutex.Lock()
	defer taken.mutex.Unlock()
	want := nodeproto.Changed{RootPath: "/media", Path: "Shows/Dag"}
	if response.status != http.StatusOK || len(taken.notices) != 1 || taken.notices[0] != want {
		t.Errorf("a notice answered %d and handed on %+v; want 200 and %+v", response.status, taken.notices, want)
	}
}

func TestConnectedNodeRoots_AreWhatTheNodesLastHelloNamed(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	if _, _, connected := harness.service.ConnectedNodeRoots(t.Context(), "local"); connected {
		t.Error("a node that never said hello has roots")
	}
	harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films", "/media/shows"}})
	reply := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films"}})
	nodeID, roots, connected := harness.service.ConnectedNodeRoots(t.Context(), "local")
	if !connected || nodeID != reply.NodeID || !slices.Equal(roots, []string{"/media/films"}) {
		t.Errorf("Roots = %d, %v, %v; want the last hello's", nodeID, roots, connected)
	}
}

// The answer to a result is the node's acknowledgement (D234 §5): a result the sink could not record
// is refused, and the node sends it again; one it cannot take as it is, as the node's mistake.
func TestResult_IsAcknowledgedOnlyOnceTheSinkRecordedIt(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	post := func() int {
		result := nodeproto.Result{JobID: 4, AttemptToken: "a4", ReadThrough: &nodeproto.ReadThroughResult{}}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return harness.post(t, nodeproto.PathResult, reply.NodeID, bytes.NewReader(encoded)).status
	}
	sink.refusal = errors.New("cache.db is busy")
	if status := post(); status != http.StatusInternalServerError {
		t.Errorf("a result the sink could not record: %d, want 500", status)
	}
	sink.refusal = ErrMalformedResult
	if status := post(); status != http.StatusBadRequest {
		t.Errorf("a result the sink cannot take as it is: %d, want 400", status)
	}
	sink.refusal = nil
	if status := post(); status != http.StatusOK || len(sink.results) != 1 {
		t.Errorf("a result the sink recorded: %d, sink %+v; want 200 and the result", status, sink.results)
	}
}

// recordingResultSink records the results and progress it is handed, and the nodes it is told
// started again.
type recordingResultSink struct {
	results  []nodeproto.Result
	from     []int64
	progress []nodeproto.Progress
	// interrupted are the calls to interrupt the work of a node's other starts, in order.
	interrupted []interruption
	// refusal, when set, is Deliver's error: a result the sink could not record.
	refusal error
	// reconciled are the calls to reconcile what a node holds, in order.
	reconciled []reconciliation
}

func (sink *recordingResultSink) Reconcile(
	_ context.Context, _ int64, held []nodeproto.HeldAttempt, onTheirWay map[string]bool,
) {
	sink.reconciled = append(sink.reconciled, reconciliation{held: held, onTheirWay: onTheirWay})
}

// reconciliation is one call to reconcile what a node holds.
type reconciliation struct {
	held       []nodeproto.HeldAttempt
	onTheirWay map[string]bool
}

func (sink *recordingResultSink) DeliverProgress(_ context.Context, _ int64, progress nodeproto.Progress) {
	sink.progress = append(sink.progress, progress)
}

// A node's progress, on its own path, reaches the result sink from a node that said hello, and
// counts as hearing from it.
func TestProgress_ReachesTheResultSinkFromAKnownNodeOnly(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	progressJSON := func() io.Reader {
		encoded, err := json.Marshal(nodeproto.Progress{JobID: 4, AttemptToken: "a4", State: nodeproto.ProgressRunning})
		if err != nil {
			t.Fatal(err)
		}
		return bytes.NewReader(encoded)
	}
	if stranger := harness.post(t, nodeproto.PathProgress, reply.NodeID+1, progressJSON()); stranger.status !=
		http.StatusConflict {
		t.Errorf("progress from a node that never said hello: %d, want 409", stranger.status)
	}
	taken := harness.post(t, nodeproto.PathProgress, reply.NodeID, progressJSON())
	if taken.status != http.StatusOK || len(sink.progress) != 1 || sink.progress[0].AttemptToken != "a4" ||
		sink.progress[0].State != nodeproto.ProgressRunning {
		t.Errorf("answered %d %s; the sink has %+v, want attempt a4 running", taken.status, taken.body, sink.progress)
	}
}

// interruption is a call to interrupt the work given to a node under another start than startID.
type interruption struct {
	nodeID  int64
	startID string
}

func (sink *recordingResultSink) Interrupt(_ context.Context, nodeID int64, startID string) {
	sink.interrupted = append(sink.interrupted, interruption{nodeID: nodeID, startID: startID})
}

// Every hello, even the first since the core itself started, has the work given to any other start
// of its node interrupted, by the start it names: a node that started again lost the work it held in
// memory (D238 §3), whether or not this core heard its last start. The work its own start was given
// is left with it, as the start ids it was given under say.
func TestHello_ANodeThatStartedAgainInterruptsTheJobsWaitingOnIt(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	first := harness.hello(t, nodeproto.Hello{Name: "local", StartID: "a1"})
	harness.hello(t, nodeproto.Hello{Name: "local", StartID: "b2"})
	want := []interruption{{nodeID: first.NodeID, startID: "a1"}, {nodeID: first.NodeID, startID: "b2"}}
	if !slices.Equal(sink.interrupted, want) {
		t.Errorf("two hellos interrupted %+v, want the work of every start but each hello's own: %+v",
			sink.interrupted, want)
	}
}

func (sink *recordingResultSink) Deliver(_ context.Context, nodeID int64, result nodeproto.Result) error {
	if sink.refusal != nil {
		return sink.refusal
	}
	sink.results = append(sink.results, result)
	sink.from = append(sink.from, nodeID)
	return nil
}

func TestResult_AResultReachesTheResultSinkFromAKnownNodeOnly(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	resultJSON := func(result nodeproto.Result) io.Reader {
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return bytes.NewReader(encoded)
	}
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	result := nodeproto.Result{JobID: 4, Artwork: &nodeproto.ArtworkBatchResult{Images: []nodeproto.ArtworkResult{{
		Images: []nodeproto.ArtImage{{SizeName: nodeproto.PortraitS, WebP: []byte("RIFF")}},
	}}}}
	if refused := harness.post(t, nodeproto.PathResult, reply.NodeID, resultJSON(result)); refused.status !=
		http.StatusServiceUnavailable {
		t.Errorf("with no result sink: %d, want 503", refused.status)
	}
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	if stranger := harness.post(t, nodeproto.PathResult, reply.NodeID+1, resultJSON(result)); stranger.status !=
		http.StatusConflict {
		t.Errorf("from a node that never said hello: %d, want 409", stranger.status)
	}
	taken := harness.post(t, nodeproto.PathResult, reply.NodeID, resultJSON(result))
	if taken.status != http.StatusOK || len(sink.results) != 1 || sink.from[0] != reply.NodeID ||
		sink.results[0].JobID != 4 || sink.results[0].Artwork == nil ||
		string(sink.results[0].Artwork.Images[0].Images[0].WebP) != "RIFF" {
		t.Errorf("answered %d %s; sink has %+v", taken.status, taken.body, sink.results)
	}
	if connected, found := harness.service.ConnectedNodeID(t.Context()); !found || connected != reply.NodeID {
		t.Errorf("Connected = %d, %v; want the node", connected, found)
	}
	ctx := t.Context()
	if !harness.service.IsConnected(ctx, reply.NodeID) || harness.service.IsConnected(ctx, reply.NodeID+1) {
		t.Error("IsConnected does not tell the node that said hello from one that did not")
	}
}

func TestHello_KeepsWhatTheNodeSaidAboutItsConfinement(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	if _, found := harness.service.Facts("local"); found {
		t.Error("a node that never said hello has a confinement")
	}
	said := []nodeproto.Confinement{
		{Mechanism: "landlock", Reason: "not enabled at boot", Remedy: "add landlock to lsm="},
		{Mechanism: "seccomp", IsInForce: true},
	}
	harness.hello(t, nodeproto.Hello{Name: "local", Confinement: said})
	facts, found := harness.service.Facts("local")
	kept := facts.Hello.Confinement
	if !found || len(kept) != 2 || kept[0] != said[0] || kept[1] != said[1] {
		t.Errorf("the kept confinement = %+v, %v; want what the hello said (D112)", kept, found)
	}
}

func TestStatus_KeepsWhatTheNodeSaysItsPreparedDirectoryHolds(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	if facts, _ := harness.service.Facts("local"); facts.Status != nil {
		t.Error("a node that has sent no status has a prepared usage")
	}
	usage := nodeproto.PreparedUsage{UsedBytes: 10, BudgetBytes: 100, DiskUsedPercent: 50, DiskUsedLimitPercent: 95}
	harness.postStatus(t, reply.NodeID, nodeproto.Status{PreparedUsage: &usage})
	facts, _ := harness.service.Facts("local")
	if facts.Status == nil || facts.Status.PreparedUsage == nil || *facts.Status.PreparedUsage != usage {
		t.Errorf("the kept status = %+v; want the prepared usage it said (D217 §5)", facts.Status)
	}
}

func TestFacts_KeepTheHellosHardwareAndTheLastStatussDisks(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	if _, found := harness.service.Facts("local"); found {
		t.Error("a node that has not said hello has facts")
	}
	hardware := nodeproto.Hardware{
		GPUs:          []nodeproto.GPU{{DevicePath: "/dev/dri/renderD128", Driver: "i915"}},
		Trials:        []nodeproto.EncoderTrial{{Encoder: "h264_qsv"}, {Encoder: "h264_nvenc", Failure: "no device"}},
		ChosenEncoder: "h264_qsv",
	}
	reply := harness.hello(t, nodeproto.Hello{
		Name: "local", Host: host.System{Platform: "linux/amd64", Codename: "trixie"}, Hardware: hardware,
	})
	disk := nodeproto.Disk{Path: "/media", TotalBytes: 1000, AvailableBytes: 400}
	harness.postStatus(t, reply.NodeID, nodeproto.Status{Disks: []nodeproto.Disk{disk}})
	facts, found := harness.service.Facts("local")
	kept := facts.Hello.Hardware
	if !found || facts.NodeID != reply.NodeID || facts.Hello.Host.Codename != "trixie" ||
		kept.ChosenEncoder != "h264_qsv" || len(kept.Trials) != 2 || kept.GPUs[0].Driver != "i915" {
		t.Errorf("the facts' hello is %+v, %v; want the host and hardware the hello said (D225)",
			facts.Hello, found)
	}
	if facts.Status == nil || len(facts.Status.Disks) != 1 || facts.Status.Disks[0] != disk {
		t.Errorf("the facts' status is %+v; want the disks the status said (D225)", facts.Status)
	}
}

// heldResultReader is a node's byte socket holding results, by token.
type heldResultReader struct {
	held map[string]nodeproto.Result
}

func (reader heldResultReader) ReadHeldResult(
	_ context.Context, _ int64, posted nodeproto.Result,
) (nodeproto.Result, error) {
	result, found := reader.held[posted.AttemptToken]
	if !found {
		return nodeproto.Result{}, errors.New("the node answered 404 Not Found")
	}
	return result, nil
}

// A post naming a result the node holds, too large to post, is taken as the whole result it
// fetches: the sink records that; a held result it cannot fetch leaves the post unacknowledged.
func TestResult_AHeldResultIsFetchedWholeBeforeItIsTaken(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	sink := &recordingResultSink{}
	harness.service.SetResultSink(sink)
	whole := nodeproto.Result{JobID: 4, AttemptToken: "a4", Artwork: &nodeproto.ArtworkBatchResult{
		Images: []nodeproto.ArtworkResult{{SourceWidth: 4000}},
	}}
	harness.service.SetHeldResultReader(heldResultReader{held: map[string]nodeproto.Result{"a4": whole}})
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	post := func(token string) int {
		encoded, err := json.Marshal(nodeproto.Result{
			JobID: 4, AttemptToken: token, Held: &nodeproto.HeldResult{SizeBytes: 5 << 20, Sha256: "ab"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return harness.post(t, nodeproto.PathResult, reply.NodeID, bytes.NewReader(encoded)).status
	}
	if status := post("a5"); status != http.StatusInternalServerError || len(sink.results) != 0 {
		t.Errorf("a held result the node could not serve: %d, sink %+v; want 500 and nothing taken", status,
			sink.results)
	}
	if status := post("a4"); status != http.StatusOK || len(sink.results) != 1 || sink.results[0].Artwork == nil ||
		sink.results[0].Artwork.Images[0].SourceWidth != 4000 {
		t.Errorf("a held result: %d, sink %+v; want the whole result taken", status, sink.results)
	}
}

// A held result's bytes are taken only at the size and hash its post named, for the same job and
// attempt.
func TestTakeHeld_TakesOnlyTheBytesThePostNamedForItsAttempt(t *testing.T) {
	t.Parallel()
	encode := func(result nodeproto.Result) []byte {
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	named := func(encoded []byte) nodeproto.Result {
		sum := sha256.Sum256(encoded)
		return nodeproto.Result{JobID: 4, AttemptToken: "a4", Held: &nodeproto.HeldResult{
			SizeBytes: int64(len(encoded)), Sha256: hex.EncodeToString(sum[:]),
		}}
	}
	own := encode(nodeproto.Result{JobID: 4, AttemptToken: "a4", ReadThrough: &nodeproto.ReadThroughResult{}})
	if taken, err := takeHeld(named(own), own); err != nil || taken.ReadThrough == nil {
		t.Errorf("its own bytes: %+v, %v; want the result", taken, err)
	}
	if _, err := takeHeld(named(own), append(own, ' ')); !errors.Is(err, ErrMalformedResult) {
		t.Errorf("bytes of another size: %v, want ErrMalformedResult", err)
	}
	altered := bytes.Replace(own, []byte(`"a4"`), []byte(`"b4"`), 1)
	if _, err := takeHeld(named(own), altered); !errors.Is(err, ErrMalformedResult) {
		t.Errorf("bytes of another hash: %v, want ErrMalformedResult", err)
	}
	if _, err := takeHeld(named(altered), altered); !errors.Is(err, ErrMalformedResult) {
		t.Errorf("another attempt's result: %v, want ErrMalformedResult", err)
	}
}

// The core fetches a held result from the node's byte socket by its attempt's token, and takes it
// as the bytes its post named.
func TestReadHeldResult_FetchesItFromTheNodesByteSocket(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	socketDir := filepath.Join(t.TempDir(), "run")
	// The core reads from the socket the node's hello names (D242 §7).
	reply := harness.hello(t, nodeproto.Hello{Name: "local", ByteSocketPath: filepath.Join(socketDir, "node.sock")})
	encoded, err := json.Marshal(nodeproto.Result{
		JobID: 4, AttemptToken: "a4", ReadThrough: &nodeproto.ReadThroughResult{},
	})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := unixsocket.Listen(socketDir, "node.sock")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+nodeproto.PathBytesResult+"a4", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(encoded) //nolint:errcheck // the test reads what arrives
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }() //nolint:errcheck // closed at cleanup
	t.Cleanup(func() { _ = server.Close() })   //nolint:errcheck // as above
	sum := sha256.Sum256(encoded)
	posted := nodeproto.Result{JobID: 4, AttemptToken: "a4", Held: &nodeproto.HeldResult{
		SizeBytes: int64(len(encoded)), Sha256: hex.EncodeToString(sum[:]),
	}}
	client := NewByteSocketClient(harness.service)
	result, err := client.ReadHeldResult(t.Context(), reply.NodeID, posted)
	if err != nil || result.ReadThrough == nil || result.JobID != 4 {
		t.Errorf("the held result = %+v, %v; want job 4's measurement", result, err)
	}
	posted.AttemptToken = "a5"
	if _, err := client.ReadHeldResult(t.Context(), reply.NodeID, posted); err == nil {
		t.Error("a result the node does not hold was taken")
	}
}

// A hello says where its node's byte socket is, as an absolute path, or it is refused: the core
// reads every byte of the node's from there (D242 §7).
func TestHello_RefusesOneThatDoesNotSayWhereItsByteSocketIs(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	for _, socketPath := range []string{"", "run/node.sock"} {
		encoded, err := json.Marshal(nodeproto.Hello{
			Name: "local", ProtocolVersion: nodeproto.ProtocolVersion, ByteSocketPath: socketPath,
		})
		if err != nil {
			t.Fatal(err)
		}
		if response := harness.post(t, nodeproto.PathHello, 0, bytes.NewReader(encoded)); response.status !=
			http.StatusBadRequest {
			t.Errorf("a hello whose byte socket is %q answered %d; want 400", socketPath, response.status)
		}
	}
}
