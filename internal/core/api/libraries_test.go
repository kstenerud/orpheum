// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/auth"
	corenodes "github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/build"
	"github.com/kstenerud/orpheum/internal/platform/host"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// fakeNodes is the node channel: node 1, "local", connected once connect is called, with the
// roots it named.
type fakeNodes struct {
	roots  []string
	queued []nodeproto.Command
	// confinement is what the connected node said about its FFmpeg's confinement.
	confinement []nodeproto.Confinement
	// build is what its hello said it was built from.
	build build.Info
	// hardware is what its hello said about hardware encoding.
	hardware nodeproto.Hardware
	// prepared is what its last status said its prepared directory holds, when it said.
	prepared *nodeproto.PreparedUsage
	// disks are what its last status said of its disks.
	disks []nodeproto.Disk
	// commanded, when set, also receives each command, for a test that waits on one.
	commanded chan nodeproto.Command
}

func (nodes *fakeNodes) Roots(name string) (int64, []string, bool) {
	if name != "local" || nodes.roots == nil {
		return 0, nil, false
	}
	return 1, nodes.roots, true
}

// Facts are what node 1 said: its hello, and a status once it said anything a status carries.
func (nodes *fakeNodes) Facts(name string) (corenodes.Facts, bool) {
	if name != "local" || nodes.roots == nil {
		return corenodes.Facts{}, false
	}
	facts := corenodes.Facts{
		NodeID: 1, LastHeard: time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC),
		Hello: nodeproto.Hello{
			Name: "local", Build: nodes.build, FFmpegVersion: "ffmpeg version 7.1",
			Host: host.Info{
				Platform: "linux/amd64", Description: "Debian GNU/Linux 13 (trixie)",
				Kernel: "6.12.0",
			},
			RootPaths: nodes.roots, Confinement: nodes.confinement, Hardware: nodes.hardware, UID: 1000, GID: 1000,
		},
	}
	if nodes.prepared != nil || nodes.disks != nil {
		facts.Status = &nodeproto.Status{Activity: "idle", PreparedUsage: nodes.prepared, Disks: nodes.disks}
	}
	return facts, true
}

func (nodes *fakeNodes) Enqueue(_ int64, command nodeproto.Command) {
	if nodes.commanded != nil {
		nodes.commanded <- command
		return
	}
	nodes.queued = append(nodes.queued, command)
}

// connectNode enrols node 1 and has it name its roots.
func (harness testHarness) connectNode(t *testing.T, roots ...string) {
	t.Helper()
	storetest.ExecRaw(t, harness.database, "INSERT INTO node (id, name, enrolled_at) VALUES (1, 'local', 1)")
	harness.nodes.roots = roots
}

func newShows(root string) map[string]any {
	return map[string]any{
		"name": "Shows", "kind": "shows", "metadataSource": "tmdb", "changeDetection": "manual",
		"isWritable": false, "nodeName": "local", "rootPaths": []string{root},
	}
}

func TestLibraries_AnAdminCreatesOneOverTheNodesRoots(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	wantError(t, harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows")),
		http.StatusServiceUnavailable, CodeNodeSilent)
	harness.connectNode(t, "/media/shows")
	wantError(t, harness.do(t, http.MethodPost, "/libraries", admin, newShows("/etc")),
		http.StatusUnprocessableEntity, CodeInvalidRequest)
	interval := newShows("/media/shows")
	interval["changeDetection"] = "interval"
	withoutHours := harness.do(t, http.MethodPost, "/libraries", admin, interval)
	wantError(t, withoutHours, http.StatusUnprocessableEntity, CodeInvalidRequest)
	if !strings.Contains(withoutHours.body, "checkEveryHours") {
		t.Errorf("an interval library with no hours = %s, want it to name checkEveryHours", withoutHours.body)
	}
	created := harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows"))
	if created.status != http.StatusCreated || !strings.Contains(created.body, `"id":`) {
		t.Fatalf("POST /libraries = %d %s", created.status, created.body)
	}
	if len(harness.nodes.queued) != 1 || harness.nodes.queued[0].Scan.RootPath != "/media/shows" {
		t.Errorf("queued %+v, want a scan of the new root", harness.nodes.queued)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows")),
		http.StatusConflict, CodeConflict)
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"roots":[{"nodeName":"local","rootPath":"/media/shows","lastScanOutcome":""}]`) {
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
	if _, err := harness.service.CreateUser(t.Context(),
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
	storetest.ExecRaw(t, harness.database,
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movies', 'nfo', 'manual', 0, 1)`,
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
	wantError(t, harness.do(t, http.MethodGet, "/files?limit=2&cursor=x", admin, nil),
		http.StatusUnprocessableEntity, CodeInvalidRequest)
}

func TestLibraries_AnAdminApprovesWithheldRemovalsAndForgetsAGoneDisk(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/m")
	storetest.ExecRaw(t, harness.database,
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, is_writable, created_at)
		 VALUES (1, 'Films', 'movies', 'nfo', 'manual', 0, 1)`,
		`INSERT INTO library_root (library_id, node_id, root_path) VALUES (1, 1, '/m')`,
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
	wantError(t, harness.do(t, http.MethodPost, "/libraries/1/removals", admin,
		map[string]string{"nodeName": "local", "path": "/other"}), http.StatusUnprocessableEntity, CodeInvalidRequest)
	gone := harness.do(t, http.MethodPost, "/libraries/1/mounts/gone", admin,
		map[string]string{"nodeName": "local", "path": "/m/disk1"})
	if gone.status != http.StatusAccepted {
		t.Errorf("POST mounts/gone = %d %s", gone.status, gone.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries/1/mounts/gone", admin,
		map[string]string{"nodeName": "local", "path": "/m/disk1"}), http.StatusConflict, CodeConflict)
}

func TestNodes_AnAdminSeesEachNodeAndTheRootsItConfigured(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	storetest.ExecRaw(t, harness.database, "INSERT INTO node (id, name, enrolled_at) VALUES (2, 'attic', 1)")
	harness.connectNode(t, "/media/films", "/media/shows")
	listed := harness.do(t, http.MethodGet, "/nodes", admin, nil)
	want := `{"nodes":[{"name":"attic","isConnected":false,"rootPaths":[]},` +
		`{"name":"local","isConnected":true,"rootPaths":["/media/films","/media/shows"]}]}`
	if listed.status != http.StatusOK || !strings.Contains(listed.body, want) {
		t.Errorf("GET /nodes = %d %s", listed.status, listed.body)
	}
	if _, err := harness.service.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	wantError(t, harness.do(t, http.MethodGet, "/nodes", harness.signIn(t, "partner", "partner-password"), nil),
		http.StatusForbidden, CodeForbidden)
}
