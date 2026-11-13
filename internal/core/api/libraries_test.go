// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/home"
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
	roots []string
	// systemFolder is node 1's system folder, as its hello named it; empty for none (D435 §4).
	systemFolder string
	queued       []nodeproto.Command
	// scanned are the roots whose scan was asked for.
	scanned []string
	// readAgain are what the scans asked for read again, as root:path.
	readAgain []string
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
	// selfUses are what the connected nodes' statuses said they use of their machines.
	selfUses []corenodes.NodeSelfUse
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
	// lockedConfinement, when set, is node 1 started locked, lacking it (D450 §1).
	lockedConfinement []nodeproto.Confinement
	// leave is node 1's leave to run unconfined, nil for none; leaveGivenBy who gave it.
	leave        *corenodes.UnconfinedLeave
	leaveGivenBy []int64
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
func (nodes *fakeNodes) RootWatches(int64) map[string]nodeproto.RootWatch {
	return map[string]nodeproto.RootWatch{"/media/shows": {
		RootPath: "/media/shows", Cause: nodeproto.WatchCauseWatchLimit, Reason: "too many folders",
	}}
}

// WatchedFoldersChanged tells no node anything: the API's tests have none watching.
func (*fakeNodes) WatchedFoldersChanged() {}

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
	if nodes.lockedConfinement != nil {
		return corenodes.Locked, fakeNodeLastHeard
	}
	return corenodes.Connected, fakeNodeLastHeard
}

func (nodes *fakeNodes) LockedConfinement(nodeID int64) ([]nodeproto.Confinement, time.Time, bool) {
	return nodes.lockedConfinement, fakeNodeLastHeard, nodeID == 1 && nodes.lockedConfinement != nil
}

func (nodes *fakeNodes) UnconfinedLeave(_ context.Context, nodeID int64) (corenodes.UnconfinedLeave, bool, error) {
	if nodeID != 1 || nodes.leave == nil {
		return corenodes.UnconfinedLeave{}, false, nil
	}
	return *nodes.leave, true, nil
}

func (nodes *fakeNodes) AcceptUnconfined(_ context.Context, nodeName string, userID int64) error {
	if nodeName != "local" {
		return corenodes.ErrNoNode
	}
	nodes.leave = &corenodes.UnconfinedLeave{AcceptedBy: "admin", AcceptedAt: fakeNodeLastHeard}
	nodes.leaveGivenBy = append(nodes.leaveGivenBy, userID)
	return nil
}

// fakeNodeLastHeard is when the fake node last spoke.
var fakeNodeLastHeard = time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC)

// Send parks the job on the node while it is away, as nodes.Service does.
func (nodes *fakeNodes) Send(ctx context.Context, nodeID int64, command nodeproto.Command) error {
	if err := nodes.ParkUnlessConnected(ctx, nodeID); err != nil {
		return err
	}
	nodes.Enqueue(nodeID, command)
	return nil
}

// ParkUnlessConnected parks the job on the node while it is away, as nodes.Service does.
func (nodes *fakeNodes) ParkUnlessConnected(ctx context.Context, nodeID int64) error {
	if !nodes.IsConnected(ctx, nodeID) {
		return fmt.Errorf("send: %w", jobs.ParkOnNode(nodeID, fakeNodeLastHeard, corenodes.ErrNodeSilent))
	}
	return nil
}

// Withdraw withdraws nothing: the fake node takes each command at once.
func (*fakeNodes) Withdraw(int64, string) bool { return false }

func (*fakeNodes) StartOf(int64) string { return "" }

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

// ConnectedSystemFolders is node 1's system folder, while it is connected and named one.
func (nodes *fakeNodes) ConnectedSystemFolders(context.Context) []corenodes.SystemFolder {
	if nodes.roots == nil || nodes.systemFolder == "" {
		return nil
	}
	return []corenodes.SystemFolder{{NodeID: 1, NodeName: "local", Path: nodes.systemFolder}}
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

// ConnectedNodeID is node 1, while it is connected.
func (nodes *fakeNodes) ConnectedNodeID(ctx context.Context) (int64, bool) {
	return 1, nodes.IsConnected(ctx, 1)
}

// EncodesOnAGPU is false: the fake node encodes on its processor.
func (*fakeNodes) EncodesOnAGPU(int64) bool { return false }

// EncodeSpeedPercent is node 1's, as the test set its hardware; zero, unmeasured, for any other.
func (nodes *fakeNodes) EncodeSpeedPercent(nodeID int64) int {
	if nodeID != 1 {
		return 0
	}
	return nodes.hardware.EncodeSpeedPercent
}

// SelfUses are what the connected nodes said they use, as the test set them.
func (nodes *fakeNodes) SelfUses(context.Context) []corenodes.NodeSelfUse { return nodes.selfUses }

// SetResultSink takes nothing: the node's answers go where takeResult says.
func (*fakeNodes) SetResultSink(corenodes.ResultSink) {}

// SetScans takes nothing: no hello asks the fake node for a scan.
func (*fakeNodes) SetScans(corenodes.ScanRequester) {}

// SetChanges takes nothing: the fake node watches nothing change.
func (*fakeNodes) SetChanges(corenodes.ChangeTaker) {}

// SetStoreTiers takes nothing: the fake node keeps no chunk stores.
func (*fakeNodes) SetStoreTiers(corenodes.StoreTiers) {}

// InProgress is the scans in progress the test set.
func (nodes *fakeNodes) InProgress(context.Context) (map[library.ScanRoot]library.ScanProgress, error) {
	return nodes.scanning, nil
}

// Request keeps a root whose scan was asked for.
func (nodes *fakeNodes) Request(_ context.Context, _ int64, root string, _ int64) error {
	nodes.scanned = append(nodes.scanned, root)
	return nil
}

// RequestUnder keeps a folder whose scan was asked for, as root:folder.
func (nodes *fakeNodes) RequestUnder(_ context.Context, _ int64, root, scope string, _ int64) error {
	nodes.scanned = append(nodes.scanned, root+":"+scope)
	return nil
}

// RequestReadingAgain keeps a folder whose scan was asked for as RequestUnder does, and what it reads
// again, as root:path, each path its own.
func (nodes *fakeNodes) RequestReadingAgain(
	ctx context.Context, nodeID int64, root, scope string, readAgain library.ReadAgain, priority int64,
) error {
	for _, path := range readAgain {
		nodes.readAgain = append(nodes.readAgain, root+":"+path)
	}
	return nodes.RequestUnder(ctx, nodeID, root, scope, priority)
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
	views, err := harness.queue.List(context.WithoutCancel(t.Context()), 0, 20)
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
		"name": "Shows", "kind": "series", "metadataSource": "tmdb", "changeDetection": "manual",
		"isWritable": false, "nodeName": "local", "rootPaths": []string{root},
	}
}

// A library's metadata source is said in camelCase, as every value is, though the database keeps
// it in snake case (the owner, 2026-11-09): the API takes nfoThenTmdb and says it back.
func TestLibraries_AMetadataSourceIsSaidInCamelCase(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/shows")
	shows := newShows("/media/shows")
	shows["metadataSource"] = "nfoThenTmdb"
	if created := harness.do(t, http.MethodPost, "/libraries", admin, shows); created.status != http.StatusCreated {
		t.Fatalf("POST /libraries with nfoThenTmdb = %d %s", created.status, created.body)
	}
	if listed := harness.do(t, http.MethodGet, "/libraries", admin, nil); !strings.Contains(listed.body,
		`"metadataSource":"nfoThenTmdb"`) {
		t.Errorf("GET /libraries = %s, want its metadataSource nfoThenTmdb", listed.body)
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
			IsListed: true, ListedCount: 40, ToExamineCount: 3, ExaminedCount: 1, IsAnotherScanQueued: true,
		},
	}
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"scan":{"isListed":true,"listedCount":40,"toExamineCount":3,`+
		`"examinedCount":1,"isAnotherScanQueued":true}`) {
		t.Errorf("an admin's GET /libraries = %s, want the root's scan in progress", listed.body)
	}
}

// How a library notices changes is chosen again after it is made, by an admin alone, with hours only
// for an interval (D184; the owner, 2026-10-09).
func TestLibraries_AnAdminChoosesAgainHowALibraryNoticesChanges(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/shows")
	if created := harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows")); created.status !=
		http.StatusCreated {
		t.Fatalf("POST /libraries = %d %s", created.status, created.body)
	}
	const path = "/libraries/1/change-detection"
	wantInvalidAt(t, harness.do(t, http.MethodPut, path, admin, map[string]any{"changeDetection": "interval"}),
		http.StatusUnprocessableEntity, "body.checkEveryHours")
	wantInvalidAt(t, harness.do(t, http.MethodPut, path, admin,
		map[string]any{"changeDetection": "watch", "checkEveryHours": 6}),
		http.StatusUnprocessableEntity, "body.checkEveryHours")
	for _, change := range []map[string]any{
		{"changeDetection": "interval", "checkEveryHours": 6}, {"changeDetection": "watch"},
	} {
		if answered := harness.do(t, http.MethodPut, path, admin, change); answered.status != http.StatusNoContent {
			t.Fatalf("PUT %s %v = %d %s", path, change, answered.status, answered.body)
		}
	}
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"changeDetection":"watch"`) || strings.Contains(listed.body, "checkEveryHours") {
		t.Errorf("after watching was chosen, GET /libraries = %s; want watch and no hours", listed.body)
	}
	wantError(t, harness.do(t, http.MethodPut, "/libraries/9/change-detection", admin,
		map[string]any{"changeDetection": "manual"}), http.StatusNotFound, api.CodeNotFound)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodPut, path, harness.signIn(t, "partner", "partner-password"),
		map[string]any{"changeDetection": "manual"}), http.StatusForbidden, api.CodeForbidden)
}

func TestLibraries_AnAdminCreatesOneOverTheNodesRoots(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	wantError(t, harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows")),
		http.StatusServiceUnavailable, api.CodeNodeSilent)
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
		http.StatusConflict, api.CodeConflict)
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"roots":[{"nodeName":"local","rootPath":"/media/shows","lastScanOutcome":"",`+
		`"missingMountPaths":[],"watch":{"isWatching":false,"cause":"watchLimit","reason":"too many folders"}}]`) {
		t.Errorf("an admin's GET /libraries = %s", listed.body)
	}
	// From away, where the admin does not administer, no roots (the Plex-chain review's T2).
	harness.server.SetHome(home.Listed(netip.MustParsePrefix("192.168.1.0/24")))
	if away := harness.do(t, http.MethodGet, "/libraries", admin, nil); strings.Contains(away.body, "/media/shows") {
		t.Errorf("an admin's GET /libraries from away = %s; want no roots", away.body)
	}
	harness.server.SetHome(home.Listed([]netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"),
	}...))
	scanned := harness.do(t, http.MethodPost, "/libraries/1/scan", admin, nil)
	if scanned.status != http.StatusAccepted || !strings.Contains(scanned.body, `"askedRootCount":1`) {
		t.Errorf("POST /libraries/1/scan = %d %s", scanned.status, scanned.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries/9/scan", admin, nil), http.StatusNotFound, api.CodeNotFound)
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
	// A new library is the admin's alone until a role is given it (D243 §4).
	if unseen := harness.do(t, http.MethodGet, "/libraries", user, nil); strings.Contains(unseen.body, "Shows") {
		t.Errorf("a user's GET /libraries before their role reaches it = %s; want it absent", unseen.body)
	}
	harness.giveRoleLibraries(t, admin, "user", 1)
	listed := harness.do(t, http.MethodGet, "/libraries", user, nil)
	if listed.status != http.StatusOK || !strings.Contains(listed.body, `"name":"Shows"`) ||
		strings.Contains(listed.body, "/media/shows") {
		t.Errorf("a user's GET /libraries = %d %s; want the library without its roots", listed.status, listed.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries", user, newShows("/media/shows")),
		http.StatusForbidden, api.CodeForbidden)
	wantError(t, harness.do(t, http.MethodPost, "/libraries/1/scan", user, nil), http.StatusForbidden,
		api.CodeForbidden)
	wantError(t, harness.do(t, http.MethodGet, "/files?limit=10", user, nil), http.StatusForbidden, api.CodeForbidden)
}

func TestFiles_PagesWhatTheScannerSaw(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/m")
	librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/m"})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 1, LibraryID: 1, NodeID: 1, Path: "/m/a.mkv", Filing: "listed", FirstSeenAt: 1, LastSeenAt: 2,
	})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 2, LibraryID: 1, NodeID: 1, Path: "/m/b.txt", Filing: "skipped", FilingReason: "extension",
		FirstSeenAt: 1, LastSeenAt: 2,
	})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 3, LibraryID: 1, NodeID: 1, Path: "/m/c.mkv", Filing: "errored", FilingReason: "permissionDenied",
		FirstSeenAt: 1, LastSeenAt: 2,
	})
	otherID := librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "Other", Kind: "other", MetadataSource: "nfo", Root: "/o"})
	librarytest.File(t, harness.database, librarytest.FileSpec{
		ID: 4, LibraryID: otherID, NodeID: 1, Path: "/o/d.mkv", Filing: "skipped", FilingReason: "extension",
		FirstSeenAt: 1, LastSeenAt: 2,
	})
	var page struct {
		Files      []api.FileBody `json:"files"`
		NextCursor string         `json:"nextCursor"`
	}
	first := harness.do(t, http.MethodGet, "/files?limit=2", admin, nil)
	if err := json.Unmarshal([]byte(first.body), &page); err != nil || len(page.Files) != 2 || page.NextCursor == "" {
		t.Fatalf("first page = %d %s", first.status, first.body)
	}
	rest := harness.do(t, http.MethodGet, "/files?limit=3&cursor="+page.NextCursor, admin, nil)
	if !strings.Contains(rest.body, `"path":"/m/c.mkv"`) || strings.Contains(rest.body, "nextCursor") {
		t.Errorf("second page = %s", rest.body)
	}
	skipped := harness.do(t, http.MethodGet, "/files?limit=10&filing=skipped&library=1", admin, nil)
	if !strings.Contains(skipped.body, `"filingReason":"extension"`) || strings.Contains(skipped.body, "a.mkv") ||
		strings.Contains(skipped.body, "d.mkv") {
		t.Errorf("filing=skipped&library=1 = %s", skipped.body)
	}
	// The library filter is library, as on every list (D416 §6): the other library's file alone.
	other := harness.do(t, http.MethodGet, fmt.Sprintf("/files?limit=10&library=%d", otherID), admin, nil)
	if !strings.Contains(other.body, `"path":"/o/d.mkv"`) || strings.Contains(other.body, "/m/") {
		t.Errorf("library=2 = %s", other.body)
	}
	wantError(t, harness.do(t, http.MethodGet, "/files", admin, nil),
		http.StatusUnprocessableEntity, api.CodeInvalidRequest)
	wantInvalidAt(t, harness.do(t, http.MethodGet, "/files?limit=2&cursor=x", admin, nil),
		http.StatusUnprocessableEntity, "query.cursor")
}

func TestLibraries_AnAdminApprovesWithheldRemovalsAndForgetsAGoneDisk(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/m")
	librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/m"})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, withheld_scan_started_at,
		 withheld_reason)
		 VALUES (1, '/m', 'withheld', 5, 4, 'the scan would remove 21 of the root''s 40 files')`,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
		 VALUES (1, '/m', '/m/disk1', 21, 0)`)
	approved := harness.do(t, http.MethodPost, "/libraries/1/roots/removals", admin,
		map[string]string{"nodeName": "local", "path": "/m"})
	if approved.status != http.StatusAccepted || !strings.Contains(approved.body, `"isScanning":true`) {
		t.Errorf("POST removals = %d %s", approved.status, approved.body)
	}
	wantInvalidAt(t, harness.do(t, http.MethodPost, "/libraries/1/roots/removals", admin,
		map[string]string{"nodeName": "local", "path": "/other"}), http.StatusUnprocessableEntity, "body.path")
	gone := harness.do(t, http.MethodPost, "/libraries/1/roots/mount-gone", admin,
		map[string]string{"nodeName": "local", "path": "/m/disk1"})
	if gone.status != http.StatusAccepted {
		t.Errorf("POST roots/mount-gone = %d %s", gone.status, gone.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries/1/roots/mount-gone", admin,
		map[string]string{"nodeName": "local", "path": "/m/disk1"}), http.StatusConflict, api.CodeConflict)
}

func TestLibraries_ARootListsItsMissingMountsAndEachIsWhatADiskGoneTakes(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/m")
	librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/m"})
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
		 VALUES (1, '/m', '/m/disk1', 21, 0), (1, '/m', '/m/disk2', 22, 1)`)
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"rootPath":"/m","lastScanOutcome":"","missingMountPaths":["/m/disk1"]`) {
		t.Fatalf("GET /libraries = %d %s; want the root to list /m/disk1 alone as missing", listed.status, listed.body)
	}
	// The page sends a missing mount's path, not the root's: the root's is refused.
	wantError(t, harness.do(t, http.MethodPost, "/libraries/1/roots/mount-gone", admin,
		map[string]string{"nodeName": "local", "path": "/m"}), http.StatusConflict, api.CodeConflict)
	gone := harness.do(t, http.MethodPost, "/libraries/1/roots/mount-gone", admin,
		map[string]string{"nodeName": "local", "path": "/m/disk1"})
	if gone.status != http.StatusAccepted {
		t.Errorf("POST roots/mount-gone = %d %s", gone.status, gone.body)
	}
	if after := harness.do(t, http.MethodGet, "/libraries", admin, nil); !strings.Contains(after.body,
		`"missingMountPaths":[]`) {
		t.Errorf("GET /libraries after the disk is gone = %s; want no missing mount", after.body)
	}
}

// A library folder its node was not given says so on its row, for the Libraries page to show
// (the owner, 2026-11-13); a folder its node holds says nothing of it.
func TestLibraries_AFolderItsNodeWasNotGivenSaysSo(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/m")
	librarytest.LibraryOn(t, harness.database, 1,
		librarytest.Library{Name: "Films", Kind: "movie", MetadataSource: "nfo", Root: "/m"})
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if listed.status != http.StatusOK || strings.Contains(listed.body, "isNotGivenToNode") {
		t.Fatalf("GET /libraries = %d %s; want the folder its node holds to say nothing of it", listed.status,
			listed.body)
	}
	storetest.ExecRaw(t, harness.databasePath, `INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		VALUES (1, '/m', '/m', 1, 'not_named')`)
	listed = harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"rootPath":"/m",`) || !strings.Contains(listed.body, `"isNotGivenToNode":true`) {
		t.Errorf("GET /libraries = %d %s; want /m to say its node was not given it", listed.status, listed.body)
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
		http.StatusForbidden, api.CodeForbidden)
}
