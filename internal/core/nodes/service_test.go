// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/media/films"})
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Shows", Kind: "series", MetadataSource: "nfo", Root: "/media/shows"})
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
	// At what priority each is asked for is the scans' (TestScans_AHellosScanOfAnUnavailableRootGoesFirst).
	if strings.Join(scans.roots, ",") != "/media/films,/media/shows,/media/films" {
		t.Errorf("scans asked for of %q; want one per root a library covers in each hello", scans.roots)
	}
}

// A status saying a library root's disk is not answering makes the root unavailable, once while
// it says so: with nothing reading it, nothing else noticed, and its films were listed as playable
// (the owner's test, 2026-10-21, the production NAS off). A disk failing otherwise, and a directory no library
// covers, are left alone.
func TestStatus_ARootWhoseDiskIsNotAnsweringIsMadeUnavailableOnce(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	local := librarytest.Node(t, harness.database, "local")
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/media/films"})
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Shows", Kind: "series", MetadataSource: "nfo", Root: "/media/shows"})
	scans := &recordingScans{}
	harness.service.SetScans(scans)
	hello := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films", "/media/shows"}})
	away := nodeproto.Status{Disks: []nodeproto.Disk{
		{Path: "/media/films", Failure: "its disk is not answering", IsNotAnswering: true},
		{Path: "/media/shows", Failure: "permission denied"},
		{Path: "/prepared", Failure: "its disk is not answering", IsNotAnswering: true},
	}}
	harness.postStatus(t, hello.NodeID, away)
	harness.postStatus(t, hello.NodeID, away)
	harness.postStatus(t, hello.NodeID, nodeproto.Status{Disks: []nodeproto.Disk{{Path: "/media/films"}}})
	harness.postStatus(t, hello.NodeID, away)
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	if !slices.Equal(scans.stalled, []string{"/media/films", "/media/films"}) {
		t.Errorf("roots made unavailable: %q; want /media/films each time its disk stopped answering", scans.stalled)
	}
}

// A status measuring a root's disk that the one before said was not answering asks to bring the
// root back, with the facts the core tells its disk by: a full scan of every root on the disk stood
// between a share's return and its films (staging, 2026-10-21). A disk that answered all along, one
// failing otherwise, and a directory measured without facts, are not asked of.
func TestStatus_ARootWhoseDiskAnswersAgainIsAskedToBeBroughtBack(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	scans := &recordingScans{}
	harness.service.SetScans(scans)
	hello := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/media/films", "/media/shows"}})
	notAnswering := func(path string) nodeproto.Disk {
		return nodeproto.Disk{Path: path, Failure: "its disk is not answering", IsNotAnswering: true}
	}
	harness.postStatus(t, hello.NodeID, nodeproto.Status{Disks: []nodeproto.Disk{
		notAnswering("/media/films"), notAnswering("/media/shows"), notAnswering("/prepared"),
	}})
	harness.postStatus(t, hello.NodeID, nodeproto.Status{Disks: []nodeproto.Disk{
		{Path: "/media/films", Device: 41, IsMountPoint: true},
		{Path: "/media/shows", Failure: "permission denied"},
		{Path: "/prepared", TotalBytes: 1 << 30},
	}})
	harness.postStatus(t, hello.NodeID, nodeproto.Status{Disks: []nodeproto.Disk{
		{Path: "/media/films", Device: 41, IsMountPoint: true}, {Path: "/media/shows", Device: 41},
	}})
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	if !slices.Equal(scans.broughtBack, []string{"/media/films@41/mount"}) {
		t.Errorf("roots asked to be brought back: %q; want /media/films, on device 41 as a mount point, once",
			scans.broughtBack)
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
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/media/films"})
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Shows", Kind: "series", MetadataSource: "nfo", Root: "/media/shows"})
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
	// Each root a hello names is said to be named, though the hello scans none: one named again after a
	// hello that did not is brought back by its scan (D241 §2).
	if want := []string{"/media/films", "/media/shows", "/media/shows"}; !slices.Equal(scans.named, want) {
		t.Errorf("roots said named: %q; want %q", scans.named, want)
	}
}

// Library folders inside a root the hello names are its: a hello asks for their scans and leaves them
// available, one no longer naming the root makes them unavailable; a status whose disk of the root is
// not answering makes them unavailable, and one that answers again asks for their scans, which bring
// them back (D241 §2, D233 §10, D359 §4). Staging's config changes its roots to their parents so.
func TestHello_TheLibraryFoldersInsideARootItNamesAreItsAsTheRootsOwnWere(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	local := librarytest.Node(t, harness.database, "local")
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/tank/media/video/movies"})
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Sync", Kind: "movie", MetadataSource: "nfo", Root: "/home/synctest/movies"})
	scans := &recordingScans{}
	harness.service.SetScans(scans)
	hello := harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/tank/media", "/home/synctest"}})
	harness.hello(t, nodeproto.Hello{Name: "local", RootPaths: []string{"/tank/media"}})
	notAnswering := nodeproto.Disk{Path: "/tank/media", Failure: "its disk is not answering", IsNotAnswering: true}
	harness.postStatus(t, hello.NodeID, nodeproto.Status{Disks: []nodeproto.Disk{notAnswering}})
	harness.postStatus(t, hello.NodeID, nodeproto.Status{Disks: []nodeproto.Disk{{Path: "/tank/media", Device: 41}}})
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	if want := []string{
		"/tank/media/video/movies", "/home/synctest/movies", "/tank/media/video/movies", "/tank/media/video/movies",
	}; !slices.Equal(scans.roots, want) {
		t.Errorf("scans asked for %q; want both folders at the first hello, Films' at the second, and Films' "+
			"once its disk answers again", scans.roots)
	}
	if !slices.Equal(scans.notNamed, []string{"/home/synctest/movies"}) {
		t.Errorf("folders made unavailable for their root not named: %q; want /home/synctest/movies alone",
			scans.notNamed)
	}
	if !slices.Equal(scans.stalled, []string{"/tank/media/video/movies"}) {
		t.Errorf("folders made unavailable for their disk not answering: %q; want Films'", scans.stalled)
	}
}

// recordingScans keeps the scans asked of it, and the roots made unavailable for a hello not naming
// them or for their disk not answering.
type recordingScans struct {
	mutex    sync.Mutex
	roots    []string
	notNamed []string
	named    []string
	stalled  []string
	// broughtBack are root@device, with /mount for a mount point.
	broughtBack []string
}

func (scans *recordingScans) BringBackRoot(
	_ context.Context, _ int64, root string, device uint64, isMountPoint bool,
) (bool, error) {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	asked := root + "@" + strconv.FormatUint(device, 10)
	if isMountPoint {
		asked += "/mount"
	}
	scans.broughtBack = append(scans.broughtBack, asked)
	return true, nil
}

func (scans *recordingScans) MarkStalled(_ context.Context, _ int64, root, _ string) (string, time.Time, error) {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	scans.stalled = append(scans.stalled, root)
	return root, time.Time{}, nil
}

func (scans *recordingScans) MarkRootNotNamed(_ context.Context, _ int64, root string) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	scans.notNamed = append(scans.notNamed, root)
	return nil
}

func (scans *recordingScans) MarkRootNamed(_ context.Context, _ int64, root string) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	scans.named = append(scans.named, root)
	return nil
}

func (scans *recordingScans) RequestOnHello(_ context.Context, _ int64, root string) error {
	scans.mutex.Lock()
	defer scans.mutex.Unlock()
	scans.roots = append(scans.roots, root)
	return nil
}

func (scans *recordingScans) RequestOnReturn(ctx context.Context, nodeID int64, root string) error {
	return scans.RequestOnHello(ctx, nodeID, root)
}

func TestHello_ScansNothingWhenTheSettingSaysNot(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Millisecond)
	settingstest.Set(t, harness.settings, settings.ShouldScanOnNodeHello, "false")
	local := librarytest.Node(t, harness.database, "local")
	librarytest.LibraryOn(t, harness.database, local,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/media/films"})
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
