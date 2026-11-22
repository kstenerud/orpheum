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
		made_at) VALUES ((SELECT payload_hash FROM file WHERE id = `+file+`), 2, 'ffmpeg exited 1', 1)`,
		`INSERT INTO whole_file_hash (file_id, failure, hashed_at) VALUES (`+file+`, 'the read stopped', 1)`)
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
	if kept := storetest.QueryRaw(t, harness.databasePath,
		`SELECT count(*) FROM whole_file_hash WHERE failure IS NOT NULL`); !slices.Equal(kept, []string{"0"}) {
		t.Errorf("after the rescan, %v whole reads that failed are kept; want none", kept)
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

// aRowOf is one row of a table seeded for a rescan, and whether every rescan forgets it, discarding
// or not (D535 §8).
type aRowOf struct {
	table                    string
	seed                     func(t *testing.T, harness testHarness, file string)
	isForgottenByEveryRescan bool
}

// rowBySQL seeds a row of a table by an INSERT, the file's id written where it says FILE.
func rowBySQL(insert string) func(t *testing.T, harness testHarness, file string) {
	return func(t *testing.T, harness testHarness, file string) {
		t.Helper()
		storetest.ExecRaw(t, harness.databasePath, strings.ReplaceAll(insert, "FILE", file))
	}
}

// D535 §10: an admin's rescan that discards what was made throws out, through the server's wiring,
// every row the app made, measured, guessed or gave up on of its film and its file as it starts,
// successes too, each by the package that keeps it; what a person did of them stays. One that keeps
// what was made forgets only what was given up on (§8). Each table it seeds names this test in
// store.PersistentTablesInADiscard.
func TestRescan_ADiscardingRescanThrowsOutWhatTheAppMadeAndKeepsWhatAPersonDid(t *testing.T) {
	t.Parallel()
	const user = `(SELECT min(id) FROM user)`
	made := []aRowOf{
		{"fetch_failure", rowBySQL(`INSERT INTO fetch_failure (item_id, failure, failed_at)
			VALUES (40, 'no', 1)`), true},
		{"unknown_failure", func(t *testing.T, harness testHarness, file string) {
			t.Helper()
			err := harness.unknownFailures.Record(t.Context(), unexpected.Failure{
				Kind: "hash_whole_file", Subject: "file " + file, Error: "the read stopped",
			})
			if err != nil {
				t.Fatal(err)
			}
		}, true},
		{"whole_file_hash", rowBySQL(`INSERT INTO whole_file_hash (file_id, sha256, size_bytes, modified_at,
			hashed_at) VALUES (FILE, X'aa', 10, 1, 1)`), false},
		{"guessed_match", rowBySQL(`INSERT INTO guessed_match (item_id, guessed_at) VALUES (40, 1)`), false},
		{"own_picture_given_up", rowBySQL(`INSERT INTO own_picture_given_up (device_id, file_id, given_up_at)
			VALUES (900, FILE, 1)`), false},
	}
	personsDoing := []aRowOf{
		{"field_override", rowBySQL(`INSERT INTO field_override (item_id, field, override_value, origin, set_at)
			VALUES (40, 'title', '"A"', 'typed', 1)`), false},
		{"item_hidden", rowBySQL(`INSERT INTO item_hidden (user_id, item_id, hidden_at)
			VALUES (` + user + `, 40, 1)`), false},
		{"play_event", rowBySQL(`INSERT INTO play_event (user_id, file_id, kind, position_ms, happened_at)
			VALUES (` + user + `, FILE, 'started', 0, 1)`), false},
		{"play_position", rowBySQL(`INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at)
			VALUES (` + user + `, FILE, 600000, 1, 1)`), false},
		{"play_span", rowBySQL(`INSERT INTO play_span (user_id, file_id, from_position_ms, to_position_ms, started_at,
			ended_at) VALUES (` + user + `, FILE, 0, 600000, 1, 2)`), false},
		{"preference", rowBySQL(`INSERT INTO preference (user_id, scope, scope_id, axis, value_json, set_at)
			VALUES (` + user + `, 'item', 40, 'subtitle_language', '"en"', 1)`), false},
		{"purged_watching", rowBySQL(`INSERT INTO purged_watching (user_id, version_id, completed_count, watched_ms,
			last_watched_at) VALUES (` + user + `, (SELECT id FROM version WHERE work_id = 40), 1, 600000, 1)`), false},
		{"work_watched", rowBySQL(`INSERT INTO work_watched (user_id, work_id, watched_at)
			VALUES (` + user + `, 40, 1)`), false},
	}
	for _, isDiscarding := range []bool{true, false} {
		harness := newHarness(t)
		admin := harness.createAdminAndSignIn(t)
		harness.connectNode(t, "/media/shows")
		created := harness.do(t, http.MethodPost, "/libraries", admin, newShows("/media/shows"))
		if created.status != http.StatusCreated {
			t.Fatalf("POST /libraries = %d %s", created.status, created.body)
		}
		seedTitles(t, harness.databasePath, titleRows{
			firstID: 40, count: 1, kind: "movie", state: "identified",
			addedAt: 2,
		})
		seedListedFiles(t, harness, []listedFile{{
			workID: 40, path: "/media/shows/a.mkv", sizeBytes: 10, versionCreatedAt: 2,
		}})
		file := strconv.FormatInt(nextID(t, harness.databasePath, "file")-1, 10)
		storetest.ExecRaw(t, harness.databasePath, `INSERT INTO device (id, user_id, name, platform, added_at,
			last_signed_in_at) VALUES (900, `+user+`, 'tv', 'web', 1, 1)`)
		for _, row := range slices.Concat(made, personsDoing) {
			row.seed(t, harness, file)
		}
		path := "/libraries/1/rescan"
		if isDiscarding {
			path += "?discard=true"
		}
		if started := harness.do(t, http.MethodPost, path, admin, nil); started.status != http.StatusAccepted {
			t.Fatalf("POST %s = %d %s", path, started.status, started.body)
		}
		for _, row := range made {
			want := "1"
			if isDiscarding || row.isForgottenByEveryRescan {
				want = "0"
			}
			expectRowsLeft(t, harness, path, row.table, "what the app made", want)
		}
		for _, row := range personsDoing {
			expectRowsLeft(t, harness, path, row.table, "what a person did", "1")
		}
	}
}

// expectRowsLeft fails t unless table keeps want rows of what, after POST path.
func expectRowsLeft(t *testing.T, harness testHarness, path, table, what, want string) {
	t.Helper()
	left := storetest.QueryRaw(t, harness.databasePath, `SELECT count(*) FROM `+table)
	if !slices.Equal(left, []string{want}) {
		t.Errorf("after POST %s, %s keeps %v rows of %s; want %s", path, table, left, what, want)
	}
}

// D535 §7: a title's page says it is being rescanned from the press until the rescan ends, in place of
// a notice; before the press it says nothing of a rescan.
func TestRescan_ATitlesDetailsSayItIsBeingRescanned(t *testing.T) {
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
	before := harness.do(t, http.MethodGet, "/items/40", admin, nil)
	if before.status != http.StatusOK || strings.Contains(before.body, `"isRescanning"`) {
		t.Fatalf("GET /items/40 before a rescan = %d %s; want no isRescanning", before.status, before.body)
	}
	if started := harness.do(t, http.MethodPost, "/items/40/rescan", admin, nil); started.status !=
		http.StatusAccepted || !strings.Contains(started.body, `"isStarted":true`) {
		t.Fatalf("POST /items/40/rescan = %d %s", started.status, started.body)
	}
	during := harness.do(t, http.MethodGet, "/items/40", admin, nil)
	if during.status != http.StatusOK || !strings.Contains(during.body, `"isRescanning":true`) {
		t.Errorf("GET /items/40 while its rescan runs = %d %s; want isRescanning", during.status, during.body)
	}
}
