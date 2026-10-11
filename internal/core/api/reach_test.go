// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// artSha256 is the source hash of the film's poster in shelveReachableFilm.
const artSha256 = "abababababababababababababababababababababababababababababababab"

// shelveReachableFilm is shelveFilms with film 2 a listed, named, searchable title with a poster
// and a position the partner reached in it, in library 1.
func (harness testHarness) shelveReachableFilm(t *testing.T, partnerID int64) {
	t.Helper()
	harness.shelveFilms(t)
	harness.identifyFilm(t)
	sorter, err := textindex.NewSorter("en")
	if err != nil {
		t.Fatal(err)
	}
	storetest.ExecRaw(t, harness.databasePath,
		"INSERT INTO item_text (item_id, language, is_original, name, search_alternates) "+
			"VALUES (30, 'und', 1, 'Film', 'film')",
		"INSERT INTO sort_key (item_id, locale, listing, collation_key) VALUES (30, 'en', 'movie', X'"+
			hex.EncodeToString(sorter.Key("Film"))+"')",
		"INSERT INTO artwork (item_id, kind, source, source_ref, source_sha256, is_chosen) "+
			"VALUES (30, 'poster', 'tmdb', '/p.jpg', X'"+artSha256+"', 1)",
		fmt.Sprintf("INSERT INTO play_position (user_id, file_id, position_ms, report_seq, updated_at, "+
			"position_read_at) VALUES (%d, 2, 60000, 1, 1, 1)", partnerID))
	if err = harness.directories.WriteArt(artSha256, "portrait-s.webp", []byte("RIFF....WEBP")); err != nil {
		t.Fatal(err)
	}
}

// A library taken back from a role is gone for its members at their next request, as if it did not
// exist: from every list, search and detail, from Continue Watching, and from a playback started
// before, whose bytes and reports stop (D243 §4, scenario S40).
func TestReach_ALibraryTakenBackIsGoneAtTheNextRequestEverywhere(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	partnerID, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	harness.shelveReachableFilm(t, partnerID)
	harness.keepFilmOnesTimeline(t)
	partner := harness.signIn(t, "partner", "partner-password")
	playing := harness.play(t, partner, chrome(1))
	reportPath := "/playback/" + playing.MediaToken + "/report"

	type check struct {
		what  string
		ask   func() response
		shows func(response) bool // whether the answer shows the film, or serves it
	}
	contains := func(text string) func(response) bool {
		return func(answer response) bool {
			return answer.status == http.StatusOK && strings.Contains(answer.body, text)
		}
	}
	served := func(answer response) bool { return answer.status < 400 }
	send := func(method, path string, body any) func() response {
		return func() response { return harness.do(t, method, path, partner, body) }
	}
	get := func(path string) func() response { return send(http.MethodGet, path, nil) }
	offset := map[string]any{"subtitleOffsetMs": 100}
	checks := []check{
		{"the films list", get("/titles?kind=movie&sort=name&limit=10"), contains(`"name":"Film"`)},
		{"the newest films", get("/titles?kind=movie&sort=added&limit=10"), contains(`"name":"Film"`)},
		{"the film's page", get("/items/30"), contains(`"name":"Film"`)},
		{"search", get("/search?query=film&limit=10"), contains(`"name":"Film"`)},
		{"Continue Watching", get("/home"), contains(`"name":"Film"`)},
		{"the identified list", get("/items?identificationState=identified"), contains(`"id":30`)},
		{"the libraries", get("/libraries"), contains(`"name":"Films"`)},
		{"whether a file can be read", get("/files/1/availability"), served},
		{"subtitle candidates", get("/files/2/subtitle-candidates?language=de"), served},
		{"a new playback", send(http.MethodPost, "/playback", chrome(1)), served},
		{"the playing film's bytes", func() response {
			bytes := harness.getBytePath(t, playing.URL, partner, true, "")
			return response{status: bytes.status, body: bytes.body}
		}, served},
		{"the playing film's chunks", func() response {
			master := harness.getBytePath(t, Prefix+"/bytes/media/chunks/"+playing.MediaToken+"/master.m3u8", partner,
				true, "")
			return response{status: master.status, body: master.body}
		}, contains("picture/stream.m3u8")},
		{"a play report", send(http.MethodPost, reportPath, reportBody(1, 1000, 0)), served},
		{"a file's preferences", send(http.MethodPut, "/files/1/preferences", offset), served},
		{"hiding the film", send(http.MethodPost, "/items/30/hide", nil), served},
		{"its poster", func() response {
			art := harness.getBytePath(t, "/api/v1/bytes/art/"+artSha256+"/portrait-s.webp", partner, true, "")
			return response{status: art.status, body: art.body}
		}, contains("RIFF....WEBP")},
	}
	for _, each := range checks {
		if answer := each.ask(); !each.shows(answer) {
			t.Errorf("before: %s = %d %s; want the film", each.what, answer.status, answer.body)
		}
	}

	harness.giveRoleLibraries(t, admin, "user") // the user role reaches no library now
	for _, each := range checks {
		answer := each.ask()
		if each.shows(answer) {
			t.Errorf("after the library was taken back: %s = %d %s; want it gone", each.what, answer.status,
				answer.body)
		}
		if answer.status >= 400 && answer.status != http.StatusNotFound {
			t.Errorf("after the library was taken back: %s = %d %s; want 404, as for what does not exist",
				each.what, answer.status, answer.body)
		}
	}
}

// A metadata editor whose role does not reach a title's library cannot choose its match, change its
// fields or refresh it: it is answered as a title that does not exist (D243 §4).
func TestReach_AnEditorReachesOnlyTheLibrariesTheirRoleLists(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.identifyFilm(t)
	made := harness.do(t, http.MethodPost, "/roles", admin, map[string]any{
		"name": "editors", "capabilities": []string{"edit_metadata"}, "libraries": []any{},
	})
	if made.status != http.StatusCreated {
		t.Fatalf("POST /roles = %d %s", made.status, made.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "editor", Password: "editor-password", Role: "editors"}); err != nil {
		t.Fatal(err)
	}
	editor := harness.signIn(t, "editor", "editor-password")
	for _, request := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/items/30/candidates", nil},
		{http.MethodPost, "/items/30/match", map[string]any{"provider": "tmdb", "providerId": "1"}},
		{http.MethodPut, "/items/30/overrides/name", map[string]any{"value": "Other"}},
		{http.MethodDelete, "/items/30/overrides/name", nil},
		{http.MethodPost, "/items/30/refresh", nil},
	} {
		wantError(t, harness.do(t, request.method, request.path, editor, request.body), http.StatusNotFound,
			CodeNotFound)
	}
}

// Every operation is one of three kinds, so a new one cannot serve a library's content unchecked:
// those that check the caller's reach (TestReach_ALibraryTakenBackIsGoneAtTheNextRequestEverywhere
// and TestReach_AnEditorReachesOnlyTheLibrariesTheirRoleLists show each), those only an admin may
// call, who reaches every library (their handlers ask requireAdmin, and those with no body are
// refused to a user here), and those that hold no library content.
func TestReach_EveryOperationChecksReachIsAnAdminsOrHoldsNoLibraryContent(t *testing.T) {
	t.Parallel()
	reachChecked := []string{
		"getTitles", "getTitle", "getSeries", "search", "getHome", "getItemsByIdentification", "getLibraries",
		"getFileAvailability", "getSubtitleCandidates", "postFileSubtitle", "startPlayback", "getDirect",
		"headDirect", "getMasterPlaylist", "getRenditionFile", "getChunksMasterPlaylist", "getChunkFile",
		"getSubtitle", "reportPlayback",
		"setFilePreferences", "hideProgress", "getArt", "getItemCandidates", "postItemMatch", "putOverride",
		"deleteOverride", "postItemRefresh",
		// A viewer's playback is reach-checked against the host's libraries
		// (TestTogether_AViewerPlaysOnlyTheFilmOnInTheHostsLibraries).
		"playTogether",
	}
	noLibraryContent := []string{
		// The caller's own rows, which say nothing of what a library holds.
		"unhideProgress", "getMe", "listMyDevices", "changeMyDevice", "removeMyDevice", "signOutElsewhere",
		"getWaitingCode", "issueDeviceCode", "changePassword", "setMyPreferences", "getClientSettings",
		// A WebRTC connection, which grants nothing: each request over it checks its own reach (D273 §8).
		"postRTCOffer",
		// Signing in and setting up, before there is a caller.
		"getHealth", "setup", "signIn", "signOut", "signInWithCode", "register", "getLinkKind",
		// A Watch Together session's own management: a link is the grant (D117, D246 §3), and the host
		// puts on only what they reach (D122, TestHub_TheHostChangesWhatPlaysOnlyToAFilmTheyMayPlay).
		"openTogether", "getTogether", "addTogetherLink", "removeTogetherLink", "joinTogether", "leaveTogether",
		"listMyTogether",
		"getTogetherICE",
		"ejectFromTogether", "endTogether",
		// A reachability check's own request: it says only whether this server made the nonce.
		"echoReachability",
		// A player's word that it left its own playback, by a token only its holder may use (D258 §2):
		// it answers nothing (TestLeave_APlayerThatLeavesLetsGoOfTheWorkItsPlaybackAskedFor).
		"leavePlayback",
	}
	isAdminOnly := adminOnlyHandlers(t)
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	for _, operation := range operationsOf(harness.server.OpenAPI()) {
		switch {
		case slices.Contains(reachChecked, operation.id), slices.Contains(noLibraryContent, operation.id):
		case !isAdminOnly(operation.id):
			t.Errorf("%s (%s %s) neither checks the caller's reach nor is an admin's; check it, or list it as "+
				"holding no library content", operation.id, operation.method, operation.path)
		case operation.method == http.MethodGet || operation.method == http.MethodDelete:
			concrete := strings.NewReplacer("{id}", "1", "{deviceId}", "1", "{key}", "x", "{name}", "x").
				Replace(operation.path)
			// A request huma refuses for a missing parameter never reaches the handler, whose
			// requireAdmin the source check holds.
			answer := harness.do(t, operation.method, concrete, partner, nil)
			if answer.status != http.StatusForbidden && answer.status != http.StatusUnprocessableEntity {
				t.Errorf("%s (%s %s) = %d for a user; want it refused, an admin's", operation.id,
					operation.method, concrete, answer.status)
			}
		}
	}
}

// registeredOperation is one operation the API registers.
type registeredOperation struct{ id, method, path string }

// operationsOf is every operation the document holds.
func operationsOf(document *huma.OpenAPI) []registeredOperation {
	var operations []registeredOperation
	for path, item := range document.Paths {
		for method, operation := range map[string]*huma.Operation{
			http.MethodGet: item.Get, http.MethodPost: item.Post, http.MethodPut: item.Put,
			http.MethodDelete: item.Delete, http.MethodHead: item.Head, http.MethodPatch: item.Patch,
		} {
			if operation != nil {
				operations = append(operations, registeredOperation{operation.OperationID, method, path})
			}
		}
	}
	return operations
}

// adminOnlyHandlers says whether a handler, named for its operation, asks requireAdmin in this
// package's source.
func adminOnlyHandlers(t *testing.T) func(handler string) bool {
	t.Helper()
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var code strings.Builder
	for _, name := range sources {
		if !strings.HasSuffix(name, "_test.go") {
			code.Write(readSource(t, name))
		}
	}
	text := code.String()
	return func(handler string) bool {
		start := strings.Index(text, "\nfunc (server *Server) "+handler+"(")
		if start < 0 {
			return false
		}
		body := text[start+1:]
		if end := strings.Index(body, "\n}\n"); end >= 0 {
			body = body[:end]
		}
		return strings.Contains(body, "server.requireAdmin(")
	}
}

// readSource is one of this package's source files.
func readSource(t *testing.T, name string) []byte {
	t.Helper()
	text, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return text
}
