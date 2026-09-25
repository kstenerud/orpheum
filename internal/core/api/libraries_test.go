// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// fakeNodes is the node channel: node 1, "local", connected once connect is called, with the
// roots it named.
type fakeNodes struct {
	roots  []string
	queued []nodeproto.Command
}

func (nodes *fakeNodes) Roots(name string) (int64, []string, bool) {
	if name != "local" || nodes.roots == nil {
		return 0, nil, false
	}
	return 1, nodes.roots, true
}

func (nodes *fakeNodes) Enqueue(_ int64, command nodeproto.Command) {
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
		"writable": false, "node": "local", "roots": []string{root},
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
	wantError(t, harness.do(t, http.MethodPost, "/libraries", admin, interval),
		http.StatusUnprocessableEntity, CodeInvalidRequest)
	created := harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows"))
	if created.status != http.StatusCreated || !strings.Contains(created.body, `"id":`) {
		t.Fatalf("POST /libraries = %d %s", created.status, created.body)
	}
	if len(harness.nodes.queued) != 1 || harness.nodes.queued[0].Scan.Root != "/media/shows" {
		t.Errorf("queued %+v, want a scan of the new root", harness.nodes.queued)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows")),
		http.StatusConflict, CodeConflict)
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"roots":[{"node":"local","path":"/media/shows","outcome":""}]`) {
		t.Errorf("an admin's GET /libraries = %s", listed.body)
	}
	scanned := harness.do(t, http.MethodPost, "/libraries/1/scan", admin, nil)
	if scanned.status != http.StatusAccepted || !strings.Contains(scanned.body, `"roots":1`) {
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
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, writable, created_at)
		 VALUES (1, 'Films', 'movies', 'nfo', 'manual', 0, 1)`,
		`INSERT INTO file (library_id, node_id, path, filing, filing_reason, first_seen, last_seen) VALUES
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
	skipped := harness.do(t, http.MethodGet, "/files?limit=10&filing=skipped&library=1", admin, nil)
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
		`INSERT INTO library (id, name, kind, metadata_source, change_detection, writable, created_at)
		 VALUES (1, 'Films', 'movies', 'nfo', 'manual', 0, 1)`,
		`INSERT INTO library_root (library_id, node_id, path) VALUES (1, 1, '/m')`,
		`INSERT INTO root_state (node_id, path, outcome, outcome_at, withheld_since, withheld_reason)
		 VALUES (1, '/m', 'withheld', 5, 4, 'the scan would remove 21 of the root''s 40 files')`,
		`INSERT INTO root_mount (node_id, root, path, device, present) VALUES (1, '/m', '/m/disk1', 21, 0)`)
	approved := harness.do(t, http.MethodPost, "/libraries/1/removals", admin,
		map[string]string{"node": "local", "path": "/m"})
	if approved.status != http.StatusAccepted || !strings.Contains(approved.body, `"scanning":true`) {
		t.Errorf("POST removals = %d %s", approved.status, approved.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries/1/removals", admin,
		map[string]string{"node": "local", "path": "/other"}), http.StatusUnprocessableEntity, CodeInvalidRequest)
	gone := harness.do(t, http.MethodPost, "/libraries/1/mounts/gone", admin,
		map[string]string{"node": "local", "path": "/m/disk1"})
	if gone.status != http.StatusAccepted {
		t.Errorf("POST mounts/gone = %d %s", gone.status, gone.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries/1/mounts/gone", admin,
		map[string]string{"node": "local", "path": "/m/disk1"}), http.StatusConflict, CodeConflict)
}
