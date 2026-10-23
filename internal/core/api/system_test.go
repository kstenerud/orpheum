// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library/librarytest"
	corenodes "github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/build"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// finishPrepare records a prepare node 1 finished of file, made with encoder at speed.
func finishPrepare(t *testing.T, harness testHarness, fileID int64, encoder string, realtimeFactor float64) {
	t.Helper()
	err := harness.cache.Write(t.Context(), func(queries *cachegen.Queries) error {
		_, err := queries.InsertRendition(t.Context(), cachegen.InsertRenditionParams{
			SourceFile: fileID, SourcePayloadHash: []byte{1}, Profile: "h264", NodeID: 1, SegmentLengthSeconds: 6,
			RunID: "0123456789abcdef", CreatedAt: 1,
		})
		if err != nil {
			return fmt.Errorf("insert the rendition: %w", err)
		}
		size, finished := int64(2048), int64(1_790_000_000_000)
		_, err = queries.SetRenditionComplete(t.Context(), cachegen.SetRenditionCompleteParams{
			SizeBytes: &size, FinishedAt: &finished, VideoEncoder: &encoder, RealtimeFactor: &realtimeFactor,
			SourceFile: fileID, Profile: "h264", RunID: "0123456789abcdef",
		})
		if err != nil {
			return fmt.Errorf("complete the rendition: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSystem_ShowsTheServersFactsEachNodesAndDoctorsFindings(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/films")
	librarytest.Node(t, harness.database, "attic")
	harness.nodes.confinement = []nodeproto.Confinement{{Mechanism: "landlock", IsInForce: true}}
	harness.nodes.hardware = nodeproto.Hardware{
		GPUs: []nodeproto.GPU{{DevicePath: "/dev/dri/renderD128", Driver: "i915"}},
		Trials: []nodeproto.EncoderTrial{
			{Encoder: "h264_qsv"}, {Encoder: "h264_nvenc", Failure: "Cannot load libcuda.so.1"},
		},
		ChosenEncoder: "h264_qsv",
	}
	harness.nodes.build = harnessServerBuild
	harness.nodes.disks = []nodeproto.Disk{{Path: "/media/films", TotalBytes: 4000, AvailableBytes: 1000}}
	harness.nodes.stuck = []nodeproto.Stuck{{Tool: "ffprobe", PID: 42, Paths: []string{"/media/films/A.mkv"}, Since: 9}}
	harness.nodes.prepared = &nodeproto.PreparedUsage{
		UsedBytes: 10, BudgetBytes: 100, DiskUsedPercent: 75, DiskUsedLimitPercent: 95,
	}
	finishPrepare(t, harness, 42, "h264_qsv", 6.5)

	got := harness.do(t, http.MethodGet, "/system", admin, nil)
	if got.status != http.StatusOK {
		t.Fatalf("GET /system = %d %s", got.status, got.body)
	}
	var system struct {
		Server   ServerBody       `json:"server"`
		Findings []FindingBody    `json:"findings"`
		Nodes    []SystemNodeBody `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(got.body), &system); err != nil {
		t.Fatal(err)
	}
	checkServerFacts(t, system.Server)
	if len(system.Nodes) != 2 || system.Nodes[0].Name != "attic" || system.Nodes[0].IsConnected ||
		system.Nodes[0].Connection != "awaiting_hello" || system.Nodes[0].LastHeardAt != nil {
		t.Fatalf("the nodes are %+v; want attic, awaiting its hello, then local", system.Nodes)
	}
	checkLocalFacts(t, system.Nodes[1])
	if !strings.Contains(got.body, `"check":"build on node local","verdict":"ok"`) ||
		!strings.Contains(got.body, `"check":"hardware encoding on node local","verdict":"ok"`) ||
		!strings.Contains(got.body, `"check":"network shares on node local","verdict":"ok"`) {
		t.Errorf("the findings are %+v; want doctor's, with the node's hardware encoding", system.Findings)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodGet, "/system", harness.signIn(t, "partner", "partner-password"), nil),
		http.StatusForbidden, CodeForbidden)
}

// checkServerFacts checks the server's own facts on the System page.
func checkServerFacts(t *testing.T, server ServerBody) {
	t.Helper()
	if server.Build.Name != "v0.3.0" || server.Host.Platform == "" || server.Host.Kernel == "" ||
		server.GoVersion == "" || len(server.Disks) != 2 ||
		server.Disks[0].TotalBytes <= 0 || len(server.Databases) != 2 || server.Databases[0].SizeBytes <= 0 {
		t.Errorf("the server's facts are %+v; want its version, platform, both disks and both databases", server)
	}
}

// checkLocalFacts checks what the connected node said about itself, as the System page shows it.
func checkLocalFacts(t *testing.T, local SystemNodeBody) {
	t.Helper()
	checkBuildAndHost(t, local)
	if !local.IsConnected || local.Connection != "connected" || local.LastHeardAt == nil ||
		local.FFmpegVersion != "ffmpeg version 7.1" ||
		len(local.Confinement) != 1 || !local.Confinement[0].IsInForce || local.PreparedUsage == nil ||
		len(local.Disks) != 1 || local.Disks[0].AvailableBytes != 1000 {
		t.Errorf("local's facts are %+v; want what it said about itself (D225)", local)
	}
	checkHardware(t, local.Hardware)
	checkStuck(t, local.Stuck)
	if len(local.RecentPrepares) != 1 || local.RecentPrepares[0].SourceFileID != 42 ||
		local.RecentPrepares[0].VideoEncoder != "h264_qsv" || local.RecentPrepares[0].RealtimeFactor != 6.5 {
		t.Errorf("local's recent prepares are %+v; want the one it finished, with Quick Sync at 6.5x (D225 §3)",
			local.RecentPrepares)
	}
}

// checkStuck checks the connected node's work waiting on a disk, as the System page shows it.
func checkStuck(t *testing.T, stuck []StuckBody) {
	t.Helper()
	if len(stuck) != 1 || stuck[0].Program != "ffprobe" || stuck[0].Paths[0] != "/media/films/A.mkv" ||
		stuck[0].GaveUpAt != 9 {
		t.Errorf("local's stuck work is %+v; want the ffprobe waiting on /media/films' disk (D233 §10)", stuck)
	}
}

// checkBuildAndHost checks the connected node's build, against the server's, and its system.
func checkBuildAndHost(t *testing.T, local SystemNodeBody) {
	t.Helper()
	if local.Host == nil || local.Host.Description != "Debian GNU/Linux 13 (trixie)" ||
		local.Build == nil || local.Build.Name != "v0.3.0" || !local.IsServersBuild {
		t.Errorf("local's build is %+v and host %+v; want the server's build and its system (D225)",
			local.Build, local.Host)
	}
}

// checkHardware checks the connected node's hardware encoding, as the System page shows it.
func checkHardware(t *testing.T, hardware *HardwareBody) {
	t.Helper()
	if hardware == nil || hardware.ChosenEncoder != "h264_qsv" || len(hardware.GPUs) != 1 ||
		hardware.GPUs[0].Driver != "i915" || len(hardware.Trials) != 2 ||
		hardware.Trials[1].Failure != "Cannot load libcuda.so.1" {
		t.Errorf("local's hardware is %+v; want its GPU, each trial and the encoder chosen (D225 §3)", hardware)
	}
}

func TestHardwareFinding_WarnsWhenANodeHasAGPUButEncodesOnTheProcessor(t *testing.T) {
	t.Parallel()
	quickSync := hardwareFinding("local", nodeproto.Hardware{
		GPUs: []nodeproto.GPU{{DevicePath: "/dev/dri/renderD128"}}, ChosenEncoder: "h264_qsv",
	})
	if quickSync.Verdict != VerdictOK || !strings.Contains(quickSync.Detail, "h264_qsv") {
		t.Errorf("a node encoding with Quick Sync: %+v, want ok, naming it", quickSync)
	}
	noGPU := hardwareFinding("local", nodeproto.Hardware{ChosenEncoder: "x264"})
	if noGPU.Verdict != VerdictOK || !strings.Contains(noGPU.Detail, "no GPU") {
		t.Errorf("a node with no GPU: %+v, want ok, saying it has none", noGPU)
	}
	blocked := hardwareFinding("local", nodeproto.Hardware{
		GPUs:          []nodeproto.GPU{{DevicePath: "/dev/dri/renderD128", NoAccessReason: "permission denied"}},
		Trials:        []nodeproto.EncoderTrial{{Encoder: "h264_qsv", Failure: "Failed to initialise VAAPI"}},
		ChosenEncoder: "x264",
	})
	if blocked.Verdict != VerdictWarning || !strings.Contains(blocked.Detail, "Failed to initialise VAAPI") ||
		!strings.Contains(blocked.Action, "/dev/dri/renderD128") {
		t.Errorf("a node whose user cannot open its GPU: %+v, want a warning with the trial's words and the device "+
			"to give it (D225 §3)", blocked)
	}
}

func TestNodeBuildFinding_WarnsWhenANodeRunsAnotherBuildThanTheServer(t *testing.T) {
	t.Parallel()
	server := build.Provenance{Version: "dev", Commit: "abc", HasUncommittedChanges: true, BinarySha256: "h1"}
	if same := nodeBuildFinding("local", server, server); same.Verdict != VerdictOK {
		t.Errorf("a node on the server's own binary: %+v, want ok", same)
	}
	older := nodeBuildFinding("local", build.Provenance{Version: "v0.2.0", Commit: "def"}, server)
	if older.Verdict != VerdictWarning || !strings.Contains(older.Detail, "v0.2.0") ||
		!strings.Contains(older.Detail, "dev-abc-modified") || older.Action == "" {
		t.Errorf("a node on an older release: %+v, want a warning naming both builds (D225)", older)
	}
	rebuilt := nodeBuildFinding("local",
		build.Provenance{Version: "dev", Commit: "abc", HasUncommittedChanges: true, BinarySha256: "h2"}, server)
	if rebuilt.Verdict != VerdictWarning || !strings.Contains(rebuilt.Detail, "uncommitted changes") {
		t.Errorf("a node built from the same commit with other changes: %+v, want a warning saying so", rebuilt)
	}
}

func TestSystem_ListsWhatIsDownAndEachOutsideHostsProvider(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.queue.Park(t.Context(), jobs.Parking{Destination: jobs.DestinationTMDBAPI, Reason: "unreachable"})
	harness.queue.Park(t.Context(), jobs.Parking{
		Destination: jobs.RootDestination(1, "/media"), Reason: "missing", AwaitsSignal: true,
	})
	got := harness.do(t, http.MethodGet, "/system", admin, nil)
	var system SystemOutput
	if err := json.Unmarshal([]byte(got.body), &system.Body); err != nil || got.status != http.StatusOK {
		t.Fatalf("GET /system = %d %s", got.status, got.body)
	}
	names := make([]string, 0, len(system.Body.Destinations))
	for _, down := range system.Body.Destinations {
		names = append(names, down.Name)
	}
	if want := []string{"/media on node 1", "TMDB"}; !slices.Equal(names, want) {
		t.Errorf("the destinations down are %q, want %q (D231 §12)", names, want)
	}
	providersByName := map[string]ProviderBody{}
	for _, provider := range system.Body.Providers {
		providersByName[provider.Name] = provider
	}
	api, subtitles := providersByName["TMDB"], providersByName["OpenSubtitles"]
	if len(system.Body.Providers) != 3 || api.RatePerSecond != 40 || api.BackgroundRatePerSecond != 32 ||
		api.Down == nil || api.Down.Reason != "unreachable" || api.Credentials != "unknown" {
		t.Errorf("the providers are %+v; want three, TMDB at 40 a second, 32 for the background, down", providersByName)
	}
	if subtitles.RatePerSecond != 5 || subtitles.Down != nil || subtitles.Quota != nil {
		t.Errorf("OpenSubtitles is %+v; want 5 a second, up, its quota not said yet", subtitles)
	}
}

func TestSystem_SaysSinceWhenEachRootAndDiskHasBeenOffline(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at)
		 VALUES (1, '/media', 'unreadable', 9000)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media', '/media', 5000, 'unread')`,
		`INSERT INTO root_mount (node_id, root_path, mount_path, filesystem_device, is_present)
		 VALUES (1, '/media', '/media/disk1', 21, 0)`,
		`INSERT INTO unavailable_place (node_id, path, root_path, since, cause)
		 VALUES (1, '/media/disk1', '/media', 3000, 'missing')`)
	got := harness.do(t, http.MethodGet, "/system", admin, nil)
	var system SystemOutput
	if err := json.Unmarshal([]byte(got.body), &system.Body); err != nil || got.status != http.StatusOK {
		t.Fatalf("GET /system = %d %s", got.status, got.body)
	}
	want := []OfflineBody{
		{
			LibraryID: 1, LibraryName: "Films", NodeName: "local", RootPath: "/media", MountPath: "/media/disk1",
			OfflineAt: 3000,
		},
		{LibraryID: 1, LibraryName: "Films", NodeName: "local", RootPath: "/media", OfflineAt: 5000},
	}
	if !slices.Equal(system.Body.Offline, want) {
		t.Errorf("offline = %+v, want the disk since 3000 and the root since its first missing scan, 5000 (D41)",
			system.Body.Offline)
	}
}

// The API document names every connection a node can have, so a client can say each: needs_update
// among them, a node on another version of the node protocol (D233 §9).
func TestSystemNode_TheDocumentNamesEveryConnectionANodeCanHave(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	served := harness.do(t, http.MethodGet, "/openapi.json", "", nil)
	var document struct {
		Components struct {
			Schemas map[string]documentedSchema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(served.body), &document); err != nil {
		t.Fatalf("GET /openapi.json = %d: %v", served.status, err)
	}
	named := document.Components.Schemas["SystemNodeBody"].Properties["connection"].Enum
	slices.Sort(named)
	every := []string{
		string(corenodes.AwaitingHello), string(corenodes.Connected), string(corenodes.NeedsUpdate),
		string(corenodes.Silent),
	}
	if !slices.Equal(named, every) {
		t.Errorf("the document names the connections %v, want every one a node can have: %v", named, every)
	}
}

// The System page lists what each node's roots are on, with how many may read each at once and
// whether an admin set that; an admin sets or clears a device's limit by the node's and the
// device's names (D240 §5).
func TestSystem_ListsEachNodesDevicesAndAnAdminSetsADevicesReaderLimit(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/films")
	harness.nodes.devices = []corenodes.DeviceState{{
		Device: nodeproto.Device{
			Key: "disk:sdc", Kind: nodeproto.DeviceUnknown, Filesystem: "ext4", Disks: []string{"sdc"},
			MountPaths: []string{"/media/films"},
		},
		ReaderLimit: 4, IsReaderLimitSet: true,
	}}
	got := harness.do(t, http.MethodGet, "/system", admin, nil)
	if !strings.Contains(got.body, `"devices":[{"key":"disk:sdc","kind":"unknown","filesystem":"ext4",`+
		`"disks":["sdc"],"mountPaths":["/media/films"],"readerLimit":4,"isReaderLimitSet":true}]`) {
		t.Errorf("GET /system = %d %s; want local's disk, its limit set to 4", got.status, got.body)
	}
	four := 4
	for _, set := range []struct {
		limit *int
		want  int
	}{{&four, 4}, {nil, 0}} {
		answer := harness.do(t, http.MethodPut, "/nodes/device-reader-limit", admin, map[string]any{
			"nodeName": "local", "deviceKey": "disk:sdc", "readerLimit": set.limit,
		})
		if answer.status != http.StatusNoContent || harness.nodes.readerLimitsSet["local/disk:sdc"] != set.want {
			t.Errorf("setting sdc's limit to %v = %d %s, set %v; want 204, %d", set.limit, answer.status,
				answer.body, harness.nodes.readerLimitsSet, set.want)
		}
	}
	wantError(t, harness.do(t, http.MethodPut, "/nodes/device-reader-limit", admin, map[string]any{
		"nodeName": "attic", "deviceKey": "disk:sdc", "readerLimit": 2,
	}), http.StatusNotFound, CodeNotFound)
	wantError(t, harness.do(t, http.MethodPut, "/nodes/device-reader-limit", admin, map[string]any{
		"nodeName": "local", "deviceKey": "disk:sdc", "readerLimit": 0,
	}), http.StatusUnprocessableEntity, CodeInvalidRequest)
}

// The System page shows how the queue is doing, by kind: queued, the oldest of them, and failed in
// the last day (D233 §8).
func TestSystem_ShowsTheQueuesHealth(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	queuedAt := harness.clock.Now()
	for _, film := range []int64{1, 2} {
		if _, err := harness.queue.Enqueue(t.Context(), jobs.Request{
			Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: film}, Reason: "test",
		}); err != nil {
			t.Fatal(err)
		}
	}
	system := harness.system(t, admin)
	if len(system.Body.Queue) != 1 || system.Body.Queue[0].Kind != string(jobs.KindReadThrough) ||
		system.Body.Queue[0].QueuedCount != 2 || system.Body.Queue[0].OldestQueuedAt == nil ||
		*system.Body.Queue[0].OldestQueuedAt != queuedAt.UnixMilli() {
		t.Errorf("the queue's health = %+v; want two measurements queued since %v", system.Body.Queue, queuedAt)
	}
}

// The System page counts a kind's failed jobs back as far as job_health_window_hours says, a day to
// start (D233 §8; the constants sweep, 2026-10-20).
func TestSystem_CountsFailedJobsBackAsFarAsTheSettingSays(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	if _, err := harness.queue.Enqueue(t.Context(), jobs.Request{
		Kind: jobs.KindReadThrough, Target: jobs.Target{FileID: 1}, Reason: "test",
	}); err != nil {
		t.Fatal(err)
	}
	failing := harness.queue.NewRunner(jobs.KindReadThrough, func(context.Context, jobs.Job) error {
		return jobs.MarkPermanent(errors.New("the file is unreadable"))
	})
	if err := failing.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	harness.clock.Advance(2 * time.Hour)
	failedOf := func() int64 {
		for _, kind := range harness.system(t, admin).Body.Queue {
			if kind.Kind == string(jobs.KindReadThrough) {
				return kind.FailedLastDayCount
			}
		}
		return 0
	}
	if failed := failedOf(); failed != 1 {
		t.Errorf("two hours after it failed, the day's count is %d; want the 1 failed", failed)
	}
	harness.setSetting(t, string(settings.JobHealthWindowHours), "1")
	if failed := failedOf(); failed != 0 {
		t.Errorf("with the window an hour, a job failed two hours ago is counted: %d", failed)
	}
}

// The System page shows what each node is doing: its slots running work, the work waiting for one,
// when its running work last moved (D233 §8), and the faults the core saw in what it holds (D234 §4).
func TestSystem_ShowsWhatEachNodeIsDoing(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/films")
	harness.nodes.statusSentAt = 1_000_000
	harness.nodes.held = []nodeproto.HeldAttempt{
		{JobID: 1, State: nodeproto.HeldRunning, SinceMovedMs: 5000},
		{JobID: 2, State: nodeproto.HeldRunning, SinceMovedMs: 2000},
		{JobID: 3, State: nodeproto.HeldWaiting},
		{JobID: 4, State: nodeproto.HeldEnded},
	}
	// The node says it holds work the core never gave it: a broken invariant (D234 §4).
	harness.exchanges.Reconcile(t.Context(), 1, []nodeproto.HeldAttempt{
		{JobID: 99, AttemptToken: "never-given", State: nodeproto.HeldRunning},
	}, nil)
	work := harness.system(t, admin).Body.Nodes[0].Work
	if work == nil || work.RunningCount != 2 || work.WaitingCount != 1 || work.SlotCount != 6 ||
		work.LastMovedAt == nil || *work.LastMovedAt != 1_000_000-2000 || work.BrokenInvariantCount != 1 {
		t.Errorf("the node's work = %+v; want two of its six slots running, one waiting, last moved 2 s "+
			"before its status, and the one broken invariant", work)
	}
}

// sizedAt is a database of a fixed size.
type sizedAt uint64

func (size sizedAt) SizeBytes(context.Context) (uint64, error) { return uint64(size), nil }

// expiringAt is a certificate service whose certificate expires at a moment, or that has none.
type expiringAt struct {
	moment     time.Time
	isObtained bool
}

func (certificate expiringAt) ExpiresAt() (time.Time, bool) {
	return certificate.moment, certificate.isObtained
}

// The System page shows the public name's certificate and when it expires, says when there is none
// yet, and shows nothing of one for a server with no public name (D243 §6).
func TestSystem_ShowsWhenThePublicNamesCertificateExpires(t *testing.T) {
	t.Parallel()
	expiry := time.Date(2026, 12, 31, 12, 0, 0, 0, time.UTC)
	system := System{
		PersistentDirectory: t.TempDir(), CacheDirectory: t.TempDir(),
		PersistentDatabase: sizedAt(1), CacheDatabase: sizedAt(1),
		PublicName: "media.orpheum-app.com", Certificate: expiringAt{moment: expiry, isObtained: true},
	}
	facts, err := serverFacts(t.Context(), system)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Certificate == nil || facts.Certificate.Name != "media.orpheum-app.com" ||
		facts.Certificate.ExpiresAt == nil || *facts.Certificate.ExpiresAt != expiry.UnixMilli() {
		t.Errorf("an obtained certificate = %+v; want the name, expiring %v", facts.Certificate, expiry)
	}
	system.Certificate = expiringAt{}
	if facts, err = serverFacts(t.Context(), system); err != nil || facts.Certificate == nil ||
		facts.Certificate.ExpiresAt != nil {
		t.Errorf("no certificate yet = %+v (%v); want the name with no expiry", facts.Certificate, err)
	}
	system.PublicName, system.Certificate = "", nil
	if facts, err = serverFacts(t.Context(), system); err != nil || facts.Certificate != nil {
		t.Errorf("no public name = %+v (%v); want no certificate shown", facts.Certificate, err)
	}
}

// How many of a held-back scan's removals are named is a setting (the constants sweep, 2026-10-20):
// at two, two are, and all three counted.
func TestSystem_NamesAsManyOfAHeldBackScansRemovalsAsTheSettingSays(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, withheld_scan_started_at,
		 withheld_reason) VALUES (1, '/media', 'withheld', 9000, 5000, 'the scan would remove 3 of 5 files')`)
	harness.setSetting(t, string(settings.ScanWithheldRemovalsNamed), "2")
	got := harness.do(t, http.MethodGet, "/system", admin, nil)
	var system SystemOutput
	if err := json.Unmarshal([]byte(got.body), &system.Body); err != nil || got.status != http.StatusOK {
		t.Fatalf("GET /system = %d %s", got.status, got.body)
	}
	want := []string{"/media/Film (2001).mp4", "/media/Film (2002).mkv"}
	if len(system.Body.Waiting) != 1 || !slices.Equal(system.Body.Waiting[0].FirstPaths, want) ||
		system.Body.Waiting[0].FileCount != 3 {
		t.Errorf("waiting, two named = %+v; want the first two of three", system.Body.Waiting)
	}
}

func TestSystem_SaysWhatAHeldBackScanWouldRemoveAndWhatCouldNotBeRead(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	storetest.ExecRaw(t, harness.databasePath,
		`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, filing, filing_reason, error_detail_json,
		 first_seen_at, last_seen_at) VALUES
		 (4, 1, 1, '/media/Locked', CAST('/media/Locked' AS BLOB), 'errored', 'permission_denied',
		  '{"text":"permission denied","ownerUid":0,"groupGid":0,"modeText":"drwxr-x---","occurrenceCount":1}',
		  7000, 9000)`,
		`INSERT INTO file (id, library_id, node_id, path, reported_path_raw, filing, first_seen_at, last_seen_at)
		 VALUES (5, 1, 1, '/media/Locked/Inside.mkv', CAST('/media/Locked/Inside.mkv' AS BLOB), 'listed', 1, 1)`,
		`INSERT INTO root_state (node_id, root_path, last_scan_outcome, last_scan_at, withheld_scan_started_at,
		 withheld_reason) VALUES (1, '/media', 'withheld', 9000, 5000, 'the scan would remove 3 of 5 files')`)
	got := harness.do(t, http.MethodGet, "/system", admin, nil)
	var system SystemOutput
	if err := json.Unmarshal([]byte(got.body), &system.Body); err != nil || got.status != http.StatusOK {
		t.Fatalf("GET /system = %d %s", got.status, got.body)
	}
	if len(system.Body.Offline) != 0 {
		t.Errorf("offline = %+v while only removals wait; want nothing (D277 §1)", system.Body.Offline)
	}
	wantPaths := []string{"/media/Film (2001).mp4", "/media/Film (2002).mkv", "/media/notes.txt"}
	if len(system.Body.Waiting) != 1 || system.Body.Waiting[0].FileCount != 3 ||
		!slices.Equal(system.Body.Waiting[0].FirstPaths, wantPaths) || system.Body.Waiting[0].WaitingAt != 5000 {
		t.Errorf("waiting = %+v; want /media's 3 unseen files, not what is under the folder it could not read",
			system.Body.Waiting)
	}
	if len(system.Body.Unread) != 1 || system.Body.Unread[0].Path != "/media/Locked" ||
		system.Body.Unread[0].ErrorDetail.ModeText != "drwxr-x---" || system.Body.Unread[0].FoundAt != 7000 {
		t.Errorf("unread = %+v; want /media/Locked with its mode", system.Body.Unread)
	}
	counted := harness.do(t, http.MethodGet, "/needs-attention", admin, nil)
	if !strings.Contains(counted.body, `"waitingRootCount":1`) || !strings.Contains(counted.body, `"unreadCount":1`) {
		t.Errorf("GET /needs-attention = %s; want one folder waiting and one path unread", counted.body)
	}
}

// A node reading a share mounted to wait for its server for good is warned of, with how to mount it
// to give up; a filesystem passed on from elsewhere is noted; local disks are fine (D233 §10).
func TestShareFinding_WarnsOfAShareThatWaitsForItsServerForGood(t *testing.T) {
	t.Parallel()
	local := shareFinding("local", []nodeproto.Device{{Key: "disk:sda", MountPaths: []string{"/media"}}})
	if local.Verdict != VerdictOK {
		t.Errorf("a node on a local disk: %+v, want ok", local)
	}
	hard := shareFinding("local", []nodeproto.Device{
		{
			Key: "network:nas:/films", Filesystem: "nfs4", MountPaths: []string{"/media/films"},
			WhenGone: nodeproto.ShareWaitsForever,
		},
	})
	if hard.Verdict != VerdictWarning || !strings.Contains(hard.Detail, "/media/films") ||
		!strings.Contains(hard.Action, "soft,timeo=100,retrans=3") {
		t.Errorf("a node on a hard NFS share: %+v, want a warning naming it, saying to mount it soft", hard)
	}
	passed := shareFinding("local", []nodeproto.Device{
		{
			Key: "unknown:0:45", Filesystem: "virtiofs", MountPaths: []string{"/tank/media"},
			WhenGone: nodeproto.ShareMayWaitForever,
		},
	})
	if passed.Verdict != VerdictOK || !strings.Contains(passed.Detail, "/tank/media (virtiofs)") ||
		!strings.Contains(passed.Detail, "soft,timeo=100,retrans=3") {
		t.Errorf("a node on a filesystem passed on: %+v, want a note naming it and the mount to use behind it", passed)
	}
}

// Every encoder a node may report is one the schema names, for its trials and its choice: one the
// schema lacks reaches a client whose types do not know it (D225 §3, D332).
func TestSchema_NamesEveryEncoderANodeMayReport(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	schemas := harness.server.OpenAPI().Components.Schemas.Map()
	hardware := []string{nodeproto.EncoderQuickSync, nodeproto.EncoderNVENC, nodeproto.EncoderVAAPI}
	for _, check := range []struct {
		schema, property string
		encoders         []string
	}{
		{"EncoderTrialBody", "encoder", hardware},
		{"HardwareBody", "chosenEncoder", append(slices.Clone(hardware), nodeproto.EncoderX264)},
	} {
		schema, found := schemas[check.schema]
		if !found {
			t.Fatalf("the schema has no %s", check.schema)
		}
		var named []string
		for _, value := range schema.Properties[check.property].Enum {
			named = append(named, fmt.Sprint(value))
		}
		for _, encoder := range check.encoders {
			if !slices.Contains(named, encoder) {
				t.Errorf("%s's %s names %v, lacking %q", check.schema, check.property, named, encoder)
			}
		}
	}
}
