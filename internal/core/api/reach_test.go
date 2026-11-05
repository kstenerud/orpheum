// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
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
		{"how long a file's work will take", get("/files/1/waits"), served},
		{"subtitle candidates", get("/files/2/subtitle-candidates?language=de"), served},
		{"a new playback", send(http.MethodPost, "/playback", chrome(1)), served},
		{"the playing film's bytes", func() response {
			bytes := harness.getBytePath(t, playing.URL, partner, true, "")
			return response{status: bytes.status, body: bytes.body}
		}, served},
		{"the playing film's chunks", func() response {
			master := harness.getBytePath(t, api.Prefix+"/bytes/media/chunks/"+playing.MediaToken+"/master.m3u8",
				partner,
				true, "")
			return response{status: master.status, body: master.body}
		}, contains("picture/stream.m3u8")},
		{"a play report", send(http.MethodPost, reportPath, reportBody(1, 1000, 0)), served},
		{"a file's preferences", send(http.MethodPut, "/files/1/preferences", offset), served},
		{"hiding the film", send(http.MethodPost, "/items/30/hide", nil), served},
		{"marking the film watched", send(http.MethodPut, "/items/30/watched", nil), served},
		{"marking the film unwatched", send(http.MethodDelete, "/items/30/watched", nil), served},
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
		{http.MethodPost, "/items/30/match-by-link", map[string]any{"link": "tt0456789"}},
		{http.MethodPost, "/items/30/confirm-match", nil},
		{http.MethodGet, "/items/30/candidates?name=Civilisation", nil},
		{http.MethodPut, "/interest/page", map[string]int64{"item": 30}},
		{http.MethodPut, "/items/30/overrides/name", map[string]any{"value": "Other"}},
		{http.MethodDelete, "/items/30/overrides/name", nil},
		{http.MethodPost, "/items/30/refresh", nil},
	} {
		wantError(t, harness.do(t, request.method, request.path, editor, request.body), http.StatusNotFound,
			api.CodeNotFound)
	}
	// The tiles of a list it does not reach raise nothing (D362 §4).
	tiles := harness.do(t, http.MethodPut, "/interest/tiles", editor, map[string][]int64{"items": {30}})
	if looks := harness.looks.Looks(time.Minute); tiles.status != http.StatusNoContent || len(looks) != 1 ||
		len(looks[0].Tiles) != 0 {
		t.Errorf("PUT /interest/tiles of a title not reached = %d, looks %+v; want its tiles shown as none",
			tiles.status, looks)
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
		"getTitles", "getItem", "getSeason", "search", "getHome", "getItemsByIdentification",
		"getLibraries",
		"getFileAvailability", "getFileWaits", "getSubtitleCandidates", "postFileSubtitle", "startPlayback",
		"getDirect",
		"headDirect", "getChunksMasterPlaylist", "getChunkFile",
		"getSubtitle", "reportPlayback",
		"setFilePreferences", "hideProgress", "getArt", "getItemCandidates", "postItemMatch", "putOverride",
		"deleteOverride", "postItemRefresh", "postItemMatchByLink", "postItemConfirmMatch", "sayPageInterest",
		"sayTilesInterest",
		// TestOrderings_APersonChoosesAmongTheOrderingsASeriesHas.
		"getNotInOrdering", "setSeriesPreferences",
		// What plays after a file, which reaches the file (requireSignedInReachingFile).
		"getUpNext",
		// A list's letters and an other library's folders (TestBrowse_OneLibrarysListAndALibraryNotReached).
		"getTitleLetters", "getLibraryFolder",
		// A viewer's playback is reach-checked against the host's libraries
		// (TestTogether_AViewerPlaysOnlyTheFilmOnInTheHostsLibraries).
		"playTogether",
		// A cast's film is reach-checked as its sender casts it
		// (TestCast_ADeviceCastsOnlyToAnotherOfItsPersonsConnectedNow).
		"startCast",
		// What a cast moves on to is reach-checked as a playback is
		// (TestCast_MovesOnToTheNextEpisodeAndItsReceiverIsHandedIt).
		"moveCastOn",
		// A mark by hand (TestReach_ALibraryTakenBackIsGoneAtTheNextRequestEverywhere,
		// TestWatched_ASeriesNotReachedIsNoSuchSeriesToMark).
		"markTitleWatched", "markTitleUnwatched", "markSeriesWatched", "markSeriesUnwatched", "markSeasonWatched",
		"markSeasonUnwatched",
		// A film played in a cast is reach-checked as startCast's is (castFilmOf;
		// TestCast_IsASessionInWhichEachFilmPlayedGoesToTheReceiver).
		"playInCast",
		// A credited person, the genres and a grouped search are of the titles the caller reaches
		// (TestPerson_ListsTheReachedTitlesCreditingThem, TestGenres_CountTheReachedTitles,
		// TestSearchGrouped_FindsTitlesPeopleAndGenresFromTheFirstLetter).
		"getPerson", "getGenres", "searchGrouped",
	}
	noLibraryContent := []string{
		// The caller's own rows, which say nothing of what a library holds.
		"unhideProgress", "restoreWatched",
		"getMe", "freshLocator", "getOwnLink", "listMyDevices", "changeMyDevice", "removeMyDevice",
		"signOutElsewhere", "activateDevice", "answerActivation", "changePassword", "setMyPreferences",
		"getMyPreferences", "withdrawPageInterest", "withdrawTilesInterest",
		"getClientSettings",
		// A WebRTC connection, which grants nothing: each request over it checks its own reach (D273 §8).
		"postRTCOffer",
		// Signing in and setting up, before there is a caller.
		"getHealth", "setup", "getSetup", "signIn", "signOut", "register", "getLinkKind",
		"requestPairing", "getPairingState", "takePairedSession", "handOffSession", "redeemSession",
		"getClientManifest",
		// A Watch Together session's own management: a link is the grant (D117, D246 §3), and the host
		// puts on only what they reach (D122, TestHub_TheHostChangesWhatPlaysOnlyToAFilmTheyMayPlay).
		"openTogether", "getTogether", "addTogetherLink", "addTogetherLinkFor", "removeTogetherLink",
		"joinTogether", "checkTogetherLink", "leaveTogether",
		"listMyTogether",
		// A person's own casts between their devices: the receiver plays by its own playback, which
		// checks its reach (D337 §1).
		"listMyCasts", "endCast", "acceptCast",
		"getTogetherICE",
		"ejectFromTogether", "endTogether",
		// A reachability check's own request: it says only whether this server made the nonce.
		"echoReachability",
		// A page's failure, or how it chose its path to the core, which are only logged and answer nothing.
		"reportClientProblem", "reportClientPath",
		// What a lab page measured on a device, which is only logged and answers its name in the log or
		// nothing, and the throughput test's body, bytes of no meaning.
		"reportLabResult", "reportLabInputEvents", "getLabThroughput",
		// A player's word that it left its own playback, by a token only its holder may use (D258 §2):
		// it answers nothing (TestLeave_APlayerThatLeavesLetsGoOfTheWorkItsPlaybackAskedFor).
		"leavePlayback",
		// A player's word on which quality of its own playback it wants, by the same token (D309): it
		// answers nothing (TestInterest_ARunOfAQualityThePlayerLeftStops).
		"setPlaybackWants",
	}
	isAdminOnly := adminOnlyHandlers(t)
	harness := newHarness(t)
	harness.createAdminAndSignIn(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	for _, operation := range api.OperationsIn(harness.server.OpenAPI()) {
		switch {
		case slices.Contains(reachChecked, operation.ID()), slices.Contains(noLibraryContent, operation.ID()):
		case !isAdminOnly(operation.ID()):
			t.Errorf("%s (%s %s) neither checks the caller's reach nor is an admin's; check it, or list it as "+
				"holding no library content", operation.ID(), operation.Method(), operation.Path())
		case operation.Method() == http.MethodGet || operation.Method() == http.MethodDelete:
			concrete := strings.NewReplacer("{id}", "1", "{deviceId}", "1", "{key}", "x", "{name}", "x").
				Replace(operation.Path())
			// A request huma refuses for a missing parameter never reaches the handler, whose
			// requireAdmin the source check holds.
			answer := harness.do(t, operation.Method(), concrete, partner, nil)
			if answer.status != http.StatusForbidden && answer.status != http.StatusUnprocessableEntity {
				t.Errorf("%s (%s %s) = %d for a user; want it refused, an admin's", operation.ID(),
					operation.Method(), concrete, answer.status)
			}
		}
	}
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
			if _, err := code.Write(readSource(t, name)); err != nil {
				t.Fatal(err)
			}
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
