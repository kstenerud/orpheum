// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/unexpected"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// D535 §8: an admin's rescan that keeps what was made forgets, through the server's wiring, the
// failures of unknown cause of its films and files and the subtitle texts of its files that could
// not be made, so their work runs again this start.
func TestRescan_ForgetsTheUnknownFailuresAndSubtitleTextFailuresOfItsFilmsAndFiles(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.connectNode(t, "/media/shows")
	if created := harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows")); created.status !=
		http.StatusCreated {
		t.Fatalf("POST /libraries = %d %s", created.status, created.body)
	}
	seedTitles(t, harness.databasePath, titleRows{
		firstID: 40, count: 1, kind: "movie", state: "unidentified", addedAt: 2,
	})
	seedListedFiles(t, harness, []listedFile{{
		workID: 40, path: "/media/shows/a.mkv", sizeBytes: 10, versionCreatedAt: 2,
	}})
	file := strconv.FormatInt(nextID(t, harness.databasePath, "file")-1, 10)
	for _, failure := range []unexpected.Failure{
		{Kind: "fetch_metadata", Subject: "item 40", Error: "not JSON"},
		{Kind: "hash_whole_file", Subject: "file " + file, Error: "the read stopped"},
	} {
		if err := harness.unknownFailures.Record(t.Context(), failure); err != nil {
			t.Fatal(err)
		}
	}
	storetest.ExecRaw(t, harness.databasePath, `INSERT INTO subtitle_text (source_hash, stream_index, failure,
		made_at) VALUES ((SELECT payload_hash FROM file WHERE id = `+file+`), 2, 'ffmpeg exited 1', 1)`)
	if started := harness.do(t, http.MethodPost, "/libraries/1/rescan", admin, nil); started.status !=
		http.StatusAccepted {
		t.Fatalf("POST /libraries/1/rescan = %d %s", started.status, started.body)
	}
	if harness.unknownFailures.IsUnavailable("fetch_metadata", "item 40") ||
		harness.unknownFailures.IsUnavailable("hash_whole_file", "file "+file) {
		t.Error("after the rescan, a film's or a file's work is still unavailable this start")
	}
	if kept := storetest.QueryRaw(t, harness.databasePath,
		`SELECT count(*) FROM subtitle_text WHERE failure IS NOT NULL`); !slices.Equal(kept, []string{"0"}) {
		t.Errorf("after the rescan, %v subtitle texts that could not be made are kept; want none", kept)
	}
}

// D357 §1, §5, D392 §6, D535 §5: the rescans are an admin's. An admin's press scans the library,
// reading every file again, and starts its rescan, whose progress the Libraries page reads; a second
// press adds nothing, nor "Rescan everything" while it runs; a person of another role is refused
// every rescan.
func TestRescan_IsForAnAdminOnly(t *testing.T) {
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
	storetest.ExecRaw(t, harness.databasePath, // the user role reaches the library, as an admin gives it
		`INSERT INTO role_library (role_id, library_id) SELECT id, 1 FROM role WHERE name = 'user'`)
	for _, path := range []string{"/libraries/rescan", "/libraries/1/rescan", "/items/1/rescan"} {
		wantError(t, harness.do(t, http.MethodPost, path, user, nil), http.StatusForbidden, api.CodeForbidden)
	}
	harness.nodes.scanned, harness.nodes.readAgain = nil, nil
	started := harness.do(t, http.MethodPost, "/libraries/1/rescan", admin, nil)
	if started.status != http.StatusAccepted || !strings.Contains(started.body, `"isStarted":true`) {
		t.Fatalf("an admin's POST /libraries/1/rescan = %d %s", started.status, started.body)
	}
	if !slices.Equal(harness.nodes.readAgain, []string{"/media/shows:."}) {
		t.Errorf("read again: %v; want every file of the library's root", harness.nodes.readAgain)
	}
	again := harness.do(t, http.MethodPost, "/libraries/1/rescan", admin, nil)
	if again.status != http.StatusAccepted || !strings.Contains(again.body, `"isStarted":false`) {
		t.Errorf("a second press = %d %s; want nothing started", again.status, again.body)
	}
	everything := harness.do(t, http.MethodPost, "/libraries/rescan", admin, nil)
	if everything.status != http.StatusAccepted || !strings.Contains(everything.body, `"startedCount":0`) {
		t.Errorf("POST /libraries/rescan while it runs = %d %s; want none started", everything.status,
			everything.body)
	}
	listed := harness.do(t, http.MethodGet, "/libraries", admin, nil)
	if !strings.Contains(listed.body, `"rescan":{"isScanning":true,"itemCount":0,"matchedCount":0}`) {
		t.Errorf("an admin's GET /libraries = %s; want the library's rescan, waiting for its scan", listed.body)
	}
	if seen := harness.do(t, http.MethodGet, "/libraries", user, nil); strings.Contains(seen.body, `"rescan"`) {
		t.Errorf("a user's GET /libraries = %s; want no rescan shown", seen.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/libraries/9/rescan", admin, nil), http.StatusNotFound,
		api.CodeNotFound)
	wantError(t, harness.do(t, http.MethodPost, "/items/99/rescan", admin, nil), http.StatusNotFound,
		api.CodeNotFound)
}

// D535 §6: a rescan's choice to throw out what was made is the query's discard, named in the OpenAPI
// of every rescan; it is an admin's, and an admin's has its scan discard everything it reads again.
func TestRescan_DiscardIsAnAdminsAndNamedInTheOpenAPI(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	for _, path := range []string{"/libraries/rescan", "/libraries/{id}/rescan", "/items/{id}/rescan"} {
		operation := harness.server.OpenAPI().Paths[path].Post
		if operation == nil || !slices.ContainsFunc(operation.Parameters, func(parameter *huma.Param) bool {
			return parameter.Name == "discard" && parameter.In == "query"
		}) {
			t.Errorf("POST %s names no discard query in the OpenAPI", path)
		}
	}
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
	harness.nodes.readAgain = nil
	for _, path := range []string{"/libraries/rescan", "/libraries/1/rescan", "/items/1/rescan"} {
		wantError(t, harness.do(t, http.MethodPost, path+"?discard=true", user, nil), http.StatusForbidden,
			api.CodeForbidden)
	}
	if len(harness.nodes.discarded) != 0 || len(harness.nodes.readAgain) != 0 {
		t.Fatalf("a user's discarding rescans discarded %v; want nothing", harness.nodes.discarded)
	}
	started := harness.do(t, http.MethodPost, "/libraries/1/rescan?discard=true", admin, nil)
	if started.status != http.StatusAccepted || !strings.Contains(started.body, `"isStarted":true`) {
		t.Fatalf("an admin's POST /libraries/1/rescan?discard=true = %d %s", started.status, started.body)
	}
	if !slices.Equal(harness.nodes.discarded, []string{"/media/shows:."}) {
		t.Errorf("discarded: %v; want every file of the library's root", harness.nodes.discarded)
	}
}
