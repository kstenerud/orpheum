// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	corenodes "github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/build"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/host"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// fakeNodes is the node channel: node 1, "local", connected once connectNode is called, with the
// roots it named.
type fakeNodes struct {
	roots  []string
	queued []nodeproto.Command
	// scanned are the roots whose scan was asked for.
	scanned []string
	// scanning are the scans in progress.
	scanning map[library.ScanRoot]library.ScanProgress
	// confinement is what the connected node said about its FFmpeg's confinement.
	confinement []nodeproto.Confinement
	// build is what its hello said it was built from.
	build build.Provenance
	// hardware is what its hello said about hardware encoding.
	hardware nodeproto.Hardware
	// prepared is what its last status said its prepared directory holds, when it said.
	prepared *nodeproto.PreparedUsage
	// disks are what its last status said of its disks.
	disks []nodeproto.Disk
	// stuck is the work local's status says waits on a disk.
	stuck []nodeproto.Stuck
	// held is the work local's status says it holds, and statusSentAt when it sent that status.
	held         []nodeproto.HeldAttempt
	statusSentAt int64
	// commanded, when set, also receives each command, for a test that waits on one.
	commanded chan nodeproto.Command
	// isAway is a node that has not connected.
	isAway atomic.Bool
	// isReadRateUnknown is a node whose hello did not say how fast it reads.
	isReadRateUnknown bool
	// awayNodeID is another node than 1, enrolled and away while node 1 is connected; 0 for none.
	awayNodeID int64
	// devices are what its hello said its roots are on, with their reader limits.
	devices []corenodes.DeviceState
	// readerLimitsSet are the reader limits set, by "node/device"; a cleared one is 0.
	readerLimitsSet map[string]int
	// database is where a retired node is removed, at clock's time.
	database *store.Database
	clock    clock.Clock
	// takeResult is where the node's answers to a fetch's conversion go, which it answers at
	// once: text it cannot read, unreadableText, is refused as FFmpeg would fail on it.
	takeResult func(nodeID int64, result nodeproto.Result)
	// sentMutex guards sentByJobID: the last command sent for each job, which a test's answer names
	// as the node's does.
	sentMutex   sync.Mutex
	sentByJobID map[int64]nodeproto.Command
}

// lastSentFor is the last command sent for job.
func (nodes *fakeNodes) lastSentFor(job int64) nodeproto.Command {
	nodes.sentMutex.Lock()
	defer nodes.sentMutex.Unlock()
	return nodes.sentByJobID[job]
}

// RemoveNode removes a node an admin retired, as the node service does; node 1 is away from then
// on.
func (nodes *fakeNodes) RemoveNode(ctx context.Context, nodeID int64) error {
	if nodeID == 1 {
		nodes.isAway.Store(true)
	}
	if err := corenodes.Remove(ctx, nodes.database, nodes.clock, nodeID); err != nil {
		return fmt.Errorf("remove node %d: %w", nodeID, err)
	}
	return nil
}

// unreadableText is a fetched subtitle the fake node cannot convert.
const unreadableText = "not a subtitle"

func (nodes *fakeNodes) IsConnected(_ context.Context, nodeID int64) bool {
	return !nodes.isAway.Load() && (nodes.awayNodeID == 0 || nodeID != nodes.awayNodeID)
}

// NodeConnection is node 1's: connected unless it is away, as IsConnected says, last heard when its
// facts say; any other node has not said hello.
func (nodes *fakeNodes) NodeConnection(_ context.Context, nodeID int64) (corenodes.Connection, time.Time) {
	if nodeID != 1 {
		return corenodes.AwaitingHello, time.Time{}
	}
	if nodes.isAway.Load() {
		return corenodes.Silent, fakeNodeLastHeard
	}
	return corenodes.Connected, fakeNodeLastHeard
}

// fakeNodeLastHeard is when the fake node last spoke.
var fakeNodeLastHeard = time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC)

// Send parks the job on the node while it is away, as nodes.Service does.
func (nodes *fakeNodes) Send(ctx context.Context, nodeID int64, command nodeproto.Command) error {
	if err := nodes.Ready(ctx, nodeID); err != nil {
		return err
	}
	nodes.Enqueue(nodeID, command)
	return nil
}

// Ready parks the job on the node while it is away, as nodes.Service does.
func (nodes *fakeNodes) Ready(ctx context.Context, nodeID int64) error {
	if !nodes.IsConnected(ctx, nodeID) {
		return fmt.Errorf("send: %w", jobs.ParkOnNode(nodeID, fakeNodeLastHeard, corenodes.ErrNodeSilent))
	}
	return nil
}

// Withdraw withdraws nothing: the fake node takes each command at once.
func (*fakeNodes) Withdraw(int64, string) bool { return false }

func (*fakeNodes) Unanswered(_ context.Context, _ int64, err error) error { return err }

func (*fakeNodes) Unpark(context.Context, jobs.Destination) {}

func (*fakeNodes) Resources(context.Context, int64) (corenodes.Resources, bool, error) {
	return corenodes.Resources{}, false, nil
}

// SlowestReadBytesPerSecond is node 1's: a busy disk, unless its hello did not say.
func (nodes *fakeNodes) SlowestReadBytesPerSecond(int64) int64 {
	if nodes.isReadRateUnknown {
		return 0
	}
	return fakeNodeSlowestRead
}

// fakeNodeSlowestRead is how fast node 1 says it reads a whole file.
const fakeNodeSlowestRead = 20 << 20

func (nodes *fakeNodes) ConnectedNodeRoots(_ context.Context, name string) (int64, []string, bool) {
	if name != "local" || nodes.roots == nil {
		return 0, nil, false
	}
	return 1, nodes.roots, true
}

// Facts are what node 1 said: its hello, and a status once it said anything a status carries.
func (nodes *fakeNodes) DeviceStates(context.Context, int64) ([]corenodes.DeviceState, error) {
	return nodes.devices, nil
}

func (nodes *fakeNodes) SetReaderLimit(_ context.Context, nodeName, deviceKey string, limit *int) error {
	if nodeName != "local" {
		return corenodes.ErrNoNode
	}
	if nodes.readerLimitsSet == nil {
		nodes.readerLimitsSet = map[string]int{}
	}
	nodes.readerLimitsSet[nodeName+"/"+deviceKey] = 0
	if limit != nil {
		nodes.readerLimitsSet[nodeName+"/"+deviceKey] = *limit
	}
	return nil
}

func (nodes *fakeNodes) Facts(name string) (corenodes.Facts, bool) {
	if name != "local" || nodes.roots == nil {
		return corenodes.Facts{}, false
	}
	facts := corenodes.Facts{
		NodeID: 1, LastHeard: fakeNodeLastHeard,
		Hello: nodeproto.Hello{
			Name: "local", Build: nodes.build, FFmpegVersion: "ffmpeg version 7.1",
			Host: host.System{
				Platform: "linux/amd64", Description: "Debian GNU/Linux 13 (trixie)",
				Kernel: "6.12.0",
			},
			RootPaths: nodes.roots, Confinement: nodes.confinement, Hardware: nodes.hardware, UID: 1000, GID: 1000,
			Capacity: nodeproto.Capacity{EncodeSlotCount: 1, RemuxSlotCount: 2},
		},
	}
	if nodes.prepared != nil || nodes.disks != nil || nodes.held != nil {
		facts.Status = &nodeproto.Status{
			SentAt: nodes.statusSentAt, PreparedUsage: nodes.prepared, Disks: nodes.disks, Stuck: nodes.stuck,
			Held: nodes.held,
		}
	}
	return facts, true
}

func (nodes *fakeNodes) Enqueue(_ int64, command nodeproto.Command) {
	nodes.sentMutex.Lock()
	if nodes.sentByJobID == nil {
		nodes.sentByJobID = map[int64]nodeproto.Command{}
	}
	nodes.sentByJobID[command.JobID] = command
	nodes.sentMutex.Unlock()
	// A fetched subtitle's conversion is answered at once, unless the test takes the node's commands.
	if extract := command.ExtractSubtitle; extract != nil && extract.FetchedSubRip != nil &&
		nodes.commanded == nil && nodes.takeResult != nil {
		answer := nodeproto.SubtitleText{
			ASS: []byte("[Script Info]"), VTT: append([]byte("WEBVTT\n\n"), extract.FetchedSubRip...),
		}
		if string(extract.FetchedSubRip) == unreadableText {
			answer = nodeproto.SubtitleText{Failure: "Invalid data found when processing input"}
		}
		answered := nodeproto.Result{
			JobID: command.JobID, AttemptToken: command.AttemptToken, Work: &command.Work, Subtitle: &answer,
		}
		nodes.takeResult(1, answered)
		return
	}
	if nodes.commanded != nil {
		nodes.commanded <- command
		return
	}
	nodes.queued = append(nodes.queued, command)
}

// InProgress is the scans in progress the test set.
func (nodes *fakeNodes) InProgress(context.Context) (map[library.ScanRoot]library.ScanProgress, error) {
	return nodes.scanning, nil
}

// Request keeps a root whose scan was asked for.
func (nodes *fakeNodes) Request(_ context.Context, _ int64, root string, _ int64) error {
	nodes.scanned = append(nodes.scanned, root)
	return nil
}

// commandWait bounds a test's wait for the node's next command: long past any honest wait on a
// loaded machine, short of the package's ten-minute timeout, whose report names only the test.
const commandWait = time.Minute

// nextCommand is the next command sent to the node, from the test's own goroutine; a command that
// never comes fails the test with the queue's jobs, which say why.
func (harness testHarness) nextCommand(t *testing.T) nodeproto.Command {
	t.Helper()
	command, isSent := harness.commandWithin(t)
	if !isSent {
		t.FailNow()
	}
	return command
}

// commandWithin is the next command sent to the node, and whether one came within commandWait;
// when none does, the test fails with the queue's jobs. Safe from any goroutine.
func (harness testHarness) commandWithin(t *testing.T) (nodeproto.Command, bool) {
	t.Helper()
	select {
	case command := <-harness.nodes.commanded:
		return command, true
	case <-time.After(commandWait):
	}
	views, err := harness.queue.List(context.WithoutCancel(t.Context()), 20)
	var jobsNow []string
	for _, view := range views {
		jobsNow = append(jobsNow, fmt.Sprintf("job %d %s %s, attempt %d, last failure %q",
			view.ID, view.Kind, view.State, view.AttemptCount, view.LastFailure))
	}
	t.Errorf("no command reached the node within %s; the queue's jobs (err %v):\n%s", commandWait, err,
		strings.Join(jobsNow, "\n"))
	return nodeproto.Command{}, false
}

// connectNode enrols node 1, "local", and has it name its roots.
func (harness testHarness) connectNode(t *testing.T, roots ...string) {
	t.Helper()
	if nodeID := librarytest.Node(t, harness.database, "local"); nodeID != 1 {
		t.Fatalf("the node connected is %d, want 1, as fakeNodes has it", nodeID)
	}
	harness.nodes.roots = roots
}

func newShows(root string) map[string]any {
	return map[string]any{
		"name": "Shows", "kind": "shows", "metadataSource": "tmdb", "changeDetection": "manual",
		"isWritable": false, "nodeName": "local", "rootPaths": []string{root},
	}
}

// An admin sees a root's scan in progress beside it: how far it is, and whether one more follows
// (D236 §2, §3).
func TestLibraries_AnAdminSeesARootsScanInProgress(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/shows")
	if created := harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows")); created.status !=
		http.StatusCreated {
		t.Fatalf("POST /libraries = %d %s", created.status, created.body)
	}
	harness.nodes.scanning = map[library.ScanRoot]library.ScanProgress{
		{NodeID: 1, RootPath: "/media/shows"}: {
			IsListed: true, ListedCount: 40, ExamineCount: 3, ExaminedCount: 1, IsScanAgain: true,
		},
	}
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"scan":{"isListed":true,"listedCount":40,"examineCount":3,`+
		`"examinedCount":1,"isScanAgain":true}`) {
		t.Errorf("an admin's GET /libraries = %s, want the root's scan in progress", listed.body)
	}
}

func TestLibraries_AnAdminCreatesOneOverTheNodesRoots(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	wantError(t, harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows")),
		http.StatusServiceUnavailable, CodeNodeSilent)
	harness.connectNode(t, "/media/shows")
	wantInvalidAt(t, harness.do(t, http.MethodPost, "/libraries", admin, newShows("/etc")),
		http.StatusUnprocessableEntity, "body.rootPaths")
	interval := newShows("/media/shows")
	interval["changeDetection"] = "interval"
	withoutHours := harness.do(t, http.MethodPost, "/libraries", admin, interval)
	wantInvalidAt(t, withoutHours, http.StatusUnprocessableEntity, "body.checkEveryHours")
	if !strings.Contains(withoutHours.body, "checkEveryHours") {
		t.Errorf("an interval library with no hours = %s, want it to name checkEveryHours", withoutHours.body)
	}
	created := harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows"))
	if created.status != http.StatusCreated || !strings.Contains(created.body, `"id":`) {
		t.Fatalf("POST /libraries = %d %s", created.status, created.body)
	}
	if !slices.Equal(harness.nodes.scanned, []string{"/media/shows"}) {
		t.Errorf("scans asked for of %v, want the new root", harness.nodes.scanned)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows")),
		http.StatusConflict, CodeConflict)
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"roots":[{"nodeName":"local","rootPath":"/media/shows","lastScanOutcome":"",`+
		`"missingMountPaths":[]}]`) {
		t.Errorf("an admin's GET /libraries = %s", listed.body)
	}
	scanned := harness.do(t, http.MethodPost, "/libraries/1/scan", admin, nil)
	if scanned.status != http.StatusAccepted || !strings.Contains(scanned.body, `"askedRootCount":1`) {
		t.Errorf("POST /libraries/1/scan = %d %s", scanned.status, scanned.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries/9/scan", admin, nil), http.StatusNotFound, CodeNotFound)
}

func TestLibraries_AUserSeesTheLibrariesButNotTheirMachinery(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/shows")
	if created := harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows")); created.status !=
		http.StatusCreated {
		t.Fatalf("POST /libraries = %d %s", created.status, created.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	user := harness.signIn(t, "partner", "partner-password")
	listed := harness.do(t, http.MethodGet, "/libraries", user, nil)
	if listed.status != http.StatusOK || !strings.Contains(listed.body, `"name":"Shows"`) ||
		strings.Contains(listed.body, "/media/shows") {
		t.Errorf("a user's GET /libraries = %d %s; want the library without its roots", listed.status, listed.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries", user, newShows("/media/shows")),
		http.StatusForbidden, CodeForbidden)
	wantError(t, harness.do(t, http.MethodPost, "/libraries/1/scan", user, nil), http.StatusForbidden, CodeForbidden)
	wantError(t, harness.do(t, http.MethodGet, "/files?limit=10", user, nil), http.StatusForbidden, CodeForbidden)
}

func TestFiles_PagesWhatTheScannerSaw(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/m")
	librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo"})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO file (library_id, node_id, path, filing, filing_reason, first_seen_at, last_seen_at) VALUES
		 (1, 1, '/m/a.mkv', 'listed', NULL, 1, 2), (1, 1, '/m/b.txt', 'skipped', 'extension', 1, 2),
		 (1, 1, '/m/c.mkv', 'errored', 'permission_denied', 1, 2)`)
	var page struct {
		Files      []FileBody `json:"files"`
		NextCursor string     `json:"nextCursor"`
	}
	first := harness.do(t, http.MethodGet, "/files?limit=2", admin, nil)
	if err := json.Unmarshal([]byte(first.body), &page); err != nil || len(page.Files) != 2 || page.NextCursor == "" {
		t.Fatalf("first page = %d %s", first.status, first.body)
	}
	rest := harness.do(t, http.MethodGet, "/files?limit=2&cursor="+page.NextCursor, admin, nil)
	if !strings.Contains(rest.body, `"path":"/m/c.mkv"`) || strings.Contains(rest.body, "nextCursor") {
		t.Errorf("last page = %s", rest.body)
	}
	skipped := harness.do(t, http.MethodGet, "/files?limit=10&filing=skipped&libraryId=1", admin, nil)
	if !strings.Contains(skipped.body, `"filingReason":"extension"`) || strings.Contains(skipped.body, "a.mkv") {
		t.Errorf("filing=skipped = %s", skipped.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/files", admin, nil),
		http.StatusUnprocessableEntity, CodeInvalidRequest)
	wantInvalidAt(t, harness.do(t, http.MethodGet, "/files?limit=2&cursor=x", admin, nil),
		http.StatusUnprocessableEntity, "query.cursor")
}

func TestLibraries_AnAdminApprovesWithheldRemovalsAndForgetsAGoneDisk(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/m")
	librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/m"})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, withheld_scan_started_at,
		 withheld_reason)
		 VALUES (1, '/m', 'withheld', 5, 4, 'the scan would remove 21 of the root''s 40 files')`,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
		 VALUES (1, '/m', '/m/disk1', 21, 0)`)
	approved := harness.do(t, http.MethodPost, "/libraries/1/removals", admin,
		map[string]string{"nodeName": "local", "path": "/m"})
	if approved.status != http.StatusAccepted || !strings.Contains(approved.body, `"isScanning":true`) {
		t.Errorf("POST removals = %d %s", approved.status, approved.body)
	}
	wantInvalidAt(t, harness.do(t, http.MethodPost, "/libraries/1/removals", admin,
		map[string]string{"nodeName": "local", "path": "/other"}), http.StatusUnprocessableEntity, "body.path")
	gone := harness.do(t, http.MethodPost, "/libraries/1/mounts/gone", admin,
		map[string]string{"nodeName": "local", "path": "/m/disk1"})
	if gone.status != http.StatusAccepted {
		t.Errorf("POST mounts/gone = %d %s", gone.status, gone.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries/1/mounts/gone", admin,
		map[string]string{"nodeName": "local", "path": "/m/disk1"}), http.StatusConflict, CodeConflict)
}

func TestLibraries_ARootListsItsMissingMountsAndEachIsWhatADiskGoneTakes(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/m")
	librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "Films", Kind: "movies", MetadataSource: "nfo", Root: "/m"})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
		 VALUES (1, '/m', '/m/disk1', 21, 0), (1, '/m', '/m/disk2', 22, 1)`)
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"rootPath":"/m","lastScanOutcome":"","missingMountPaths":["/m/disk1"]`) {
		t.Fatalf("GET /libraries = %d %s; want the root to list /m/disk1 alone as missing", listed.status, listed.body)
	}
	// The page sends a missing mount's path, not the root's: the root's is refused.
	wantError(t, harness.do(t, http.MethodPost, "/libraries/1/mounts/gone", admin,
		map[string]string{"nodeName": "local", "path": "/m"}), http.StatusConflict, CodeConflict)
	gone := harness.do(t, http.MethodPost, "/libraries/1/mounts/gone", admin,
		map[string]string{"nodeName": "local", "path": "/m/disk1"})
	if gone.status != http.StatusAccepted {
		t.Errorf("POST mounts/gone = %d %s", gone.status, gone.body)
	}
	if after := harness.do(t, http.MethodGet, "/libraries", admin, nil); !strings.Contains(after.body,
		`"missingMountPaths":[]`) {
		t.Errorf("GET /libraries after the disk is gone = %s; want no missing mount", after.body)
	}
}

func TestNodes_AnAdminSeesEachNodeAndTheRootsItConfigured(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/films", "/media/shows")
	librarytest.Node(t, harness.database, "attic")
	listed := harness.do(t, http.MethodGet, "/nodes", admin, nil)
	want := `{"nodes":[{"name":"attic","isConnected":false,"rootPaths":[]},` +
		`{"name":"local","isConnected":true,"rootPaths":["/media/films","/media/shows"]}]}`
	if listed.status != http.StatusOK || !strings.Contains(listed.body, want) {
		t.Errorf("GET /nodes = %d %s", listed.status, listed.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodGet, "/nodes", harness.signIn(t, "partner", "partner-password"), nil),
		http.StatusForbidden, CodeForbidden)
}
