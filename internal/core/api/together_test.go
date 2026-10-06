// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/together"
)

// socket is one device's session WebSocket, in a test.
type socket struct {
	t          *testing.T
	connection *websocket.Conn
}

// dialTogether opens a device's session WebSocket and says hello with credential, a playing device.
func (harness testHarness) dialTogether(t *testing.T, credential string) socket {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	address := "ws" + strings.TrimPrefix(harness.httpServer.URL, "http") + togetherSocketPath
	connection, _, err := websocket.Dial(ctx, address, nil) //nolint:bodyclose // coder/websocket closes it
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.CloseNow() }) //nolint:errcheck // closing what may be closed
	opened := socket{t: t, connection: connection}
	opened.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageHello, Credential: credential,
		Role: together.RolePlayback,
	})
	return opened
}

// send sends one message.
func (opened socket) send(message together.Inbound) {
	opened.t.Helper()
	ctx, cancel := context.WithTimeout(opened.t.Context(), 5*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, opened.connection, message); err != nil {
		opened.t.Fatal(err)
	}
}

// awaitGroup reads until a group message in state, which fails the test when none comes in time.
func (opened socket) awaitGroup(state together.GroupState) together.Outbound {
	opened.t.Helper()
	ctx, cancel := context.WithTimeout(opened.t.Context(), 5*time.Second)
	defer cancel()
	for {
		var message together.Outbound
		if err := wsjson.Read(ctx, opened.connection, &message); err != nil {
			opened.t.Fatalf("no group message in state %s: %v", state, err)
		}
		if message.Type == together.MessageGroup && message.Group.State == state {
			return message
		}
	}
}

// awaitWelcome reads until the device's welcome, which fails the test when none comes in time.
func (opened socket) awaitWelcome() together.Outbound {
	opened.t.Helper()
	ctx, cancel := context.WithTimeout(opened.t.Context(), 5*time.Second)
	defer cancel()
	for {
		var message together.Outbound
		if err := wsjson.Read(ctx, opened.connection, &message); err != nil {
			opened.t.Fatalf("no welcome: %v", err)
		}
		if message.Type == together.MessageWelcome {
			return message
		}
	}
}

// openTogether opens the session host hosts, and returns what it answered.
func (harness testHarness) openTogether(t *testing.T, host string) OpenTogetherOutput {
	t.Helper()
	answer := harness.do(t, http.MethodPost, "/together", host, nil)
	var opened OpenTogetherOutput
	if err := json.Unmarshal([]byte(answer.body), &opened.Body); err != nil || answer.status != http.StatusOK {
		t.Fatalf("POST /together = %d %s", answer.status, answer.body)
	}
	return opened
}

// joinByLink makes a link in the host's session, noted note, and joins by it as name.
func (harness testHarness) joinByLink(
	t *testing.T, host string, sessionID int64, note, name string,
) JoinTogetherOutput {
	t.Helper()
	made := harness.do(t, http.MethodPost, fmt.Sprintf("/together/%d/links", sessionID), host,
		map[string]string{"adminNote": note})
	var link TogetherLinkOutput
	if err := json.Unmarshal([]byte(made.body), &link.Body); err != nil || made.status != http.StatusCreated {
		t.Fatalf("POST /together/%d/links = %d %s", sessionID, made.status, made.body)
	}
	joined := harness.do(t, http.MethodPost, "/together/join", "",
		map[string]any{"secret": link.Body.Secret, "name": name})
	var membership JoinTogetherOutput
	if err := json.Unmarshal([]byte(joined.body), &membership.Body); err != nil || joined.status != http.StatusOK {
		t.Fatalf("POST /together/join = %d %s", joined.status, joined.body)
	}
	return membership
}

// putOn has the host's device put fileID on and say it has loaded it, as the host's player does,
// and waits until the viewer's device hears it playing.
func putOn(host, viewer socket, fileID int64) {
	host.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandChangeItem, FileID: fileID,
	})
	host.send(together.Inbound{V: together.ProtocolVersion, Type: together.MessageState})
	for {
		if playing := viewer.awaitGroup(together.GroupPlaying); playing.Group.FileID == fileID {
			return
		}
	}
}

// viewerPlayback starts a viewer's playback with their credential, and returns the answer.
func (harness testHarness) viewerPlayback(t *testing.T, credential string) response {
	t.Helper()
	body := chrome(0)
	delete(body, "fileId")
	body["audioOutput"] = "headphones"
	return harness.do(t, http.MethodPost, "/together/playback", credential, body)
}

// viewerBytes asks for a playback's bytes with only a viewer's cookie, as their player does.
func (harness testHarness) viewerBytes(t *testing.T, url, credential string) int {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, harness.httpServer.URL+url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	cookie := viewerCookie(credential, false)
	request.AddCookie(&cookie)
	answer, err := harness.httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = answer.Body.Close() //nolint:errcheck // the status is what is asked
	return answer.StatusCode
}

// The host opens a session with nothing on, a viewer joins by their own link with a name of their
// choosing, and what the host puts on plays for both and pauses for both; the viewer's player
// plays it by their membership (cut-a §5.1, D246 §3, §5).
func TestTogether_AHostPutsOnAFilmAndAViewerPlaysIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	opened := harness.openTogether(t, admin)
	membership := harness.joinByLink(t, admin, opened.Body.SessionID, "my partner", "Anna")
	host := harness.dialTogether(t, opened.Body.MembershipCredential)
	if welcome := host.awaitWelcome(); welcome.SessionID != opened.Body.SessionID ||
		!welcome.IsHost || welcome.Group.State != together.GroupIdle {
		t.Fatalf("the host's welcome = %+v", welcome)
	}
	viewer := harness.dialTogether(t, membership.Body.MembershipCredential)
	viewer.awaitWelcome()
	wantError(t, harness.viewerPlayback(t, membership.Body.MembershipCredential), http.StatusNotFound, CodeNotFound)
	putOn(host, viewer, 1)
	started := harness.viewerPlayback(t, membership.Body.MembershipCredential)
	var playing decision
	if err := json.Unmarshal([]byte(started.body), &playing); err != nil || started.status != http.StatusOK {
		t.Fatalf("POST /together/playback = %d %s", started.status, started.body)
	}
	harness.logs.AwaitLine(t, `"msg":"a playback started"`, fmt.Sprintf(`"memberId":%d`, membership.Body.MemberID),
		`"userAgent":"Go-http-client/1.1"`) // which browser, as an iPhone's player differs
	if status := harness.viewerBytes(t, playing.URL, membership.Body.MembershipCredential); status != http.StatusOK {
		t.Errorf("the viewer's bytes, by their cookie alone = %d, want 200", status)
	}
	host.send(together.Inbound{
		V: together.ProtocolVersion, Type: together.MessageCommand, Command: together.CommandPause,
	})
	if paused := viewer.awaitGroup(together.GroupPaused); paused.Group.ByMemberID == membership.Body.MemberID {
		t.Errorf("the viewer's group = %+v; want paused by the host", paused.Group)
	}
	details := harness.do(t, http.MethodGet, fmt.Sprintf("/together/%d", opened.Body.SessionID), admin, nil)
	if !strings.Contains(details.body, `"name":"Anna","adminNote":"my partner"`) {
		t.Errorf("GET /together/%d = %s; want Anna, noted as the host's partner", opened.Body.SessionID, details.body)
	}
}

// A viewer plays the film that is on, in the host's libraries, and nothing else: the one before
// is gone once another is on, and none plays once the host's role no longer reaches its library
// (D246 §3, D243 §4).
func TestTogether_AViewerPlaysOnlyTheFilmOnInTheHostsLibraries(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "partner", Password: "partner-password", Role: auth.RoleUser}); err != nil {
		t.Fatal(err)
	}
	partner := harness.signIn(t, "partner", "partner-password")
	opened := harness.openTogether(t, partner)
	membership := harness.joinByLink(t, partner, opened.Body.SessionID, "a friend", "Sam")
	host := harness.dialTogether(t, opened.Body.MembershipCredential)
	host.awaitWelcome()
	viewer := harness.dialTogether(t, membership.Body.MembershipCredential)
	viewer.awaitWelcome()
	putOn(host, viewer, 1)
	var first decision
	firstPlayback := harness.viewerPlayback(t, membership.Body.MembershipCredential)
	if err := json.Unmarshal([]byte(firstPlayback.body), &first); err != nil {
		t.Fatal(err)
	}
	putOn(host, viewer, 2)
	status := harness.viewerBytes(t, first.URL, membership.Body.MembershipCredential)
	if status != http.StatusNotFound {
		t.Errorf("the film before's bytes once another is on = %d, want 404", status)
	}
	if again := harness.viewerPlayback(t, membership.Body.MembershipCredential); again.status != http.StatusOK {
		t.Errorf("a playback of the film now on = %d %s", again.status, again.body)
	}
	harness.giveRoleLibraries(t, admin, "user") // the host's role reaches no library now
	wantError(t, harness.viewerPlayback(t, membership.Body.MembershipCredential), http.StatusNotFound, CodeNotFound)
}

// Only a role that may host Watch Together opens a session, only the host makes links, each with a
// note, and each admits one person once; a viewer may leave, and only the host ends it (D246 §3).
func TestTogether_OnlyAHostOpensMakesLinksAndEnds(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	if again := harness.openTogether(t, admin); again.Body.SessionID != opened.Body.SessionID || again.Body.IsNew {
		t.Errorf("opening again = %+v; want the session the host already has", again.Body)
	}
	made := harness.do(t, http.MethodPost, "/roles", admin, map[string]any{
		"name": "watchers", "capabilities": []string{}, "libraries": []map[string]any{},
	})
	if made.status != http.StatusCreated {
		t.Fatalf("POST /roles = %d %s", made.status, made.body)
	}
	if _, err := harness.authService.CreateUser(t.Context(),
		auth.NewUser{Name: "watcher", Password: "watcher-password", Role: "watchers"}); err != nil {
		t.Fatal(err)
	}
	watcher := harness.signIn(t, "watcher", "watcher-password")
	for token, canHost := range map[string]bool{admin: true, watcher: false} {
		var me UserBody
		if err := json.Unmarshal([]byte(harness.do(t, http.MethodGet, "/me", token, nil).body), &me); err != nil ||
			me.CanHostWatchTogether != canHost {
			t.Errorf("GET /me for %s says it may host: %v (%v); want %v",
				me.Name, me.CanHostWatchTogether, err, canHost)
		}
	}
	wantError(t, harness.do(t, http.MethodPost, "/together", watcher, nil), http.StatusForbidden, CodeForbidden)
	path := fmt.Sprintf("/together/%d", opened.Body.SessionID)
	wantError(t, harness.do(t, http.MethodPost, path+"/links", watcher, map[string]string{"adminNote": "x"}),
		http.StatusForbidden, CodeForbidden)
	wantError(t, harness.do(t, http.MethodPost, path+"/links", admin, map[string]string{"adminNote": ""}),
		http.StatusUnprocessableEntity, CodeInvalidRequest)
	link := harness.do(t, http.MethodPost, path+"/links", admin, map[string]string{"adminNote": "a cousin"})
	var made2 TogetherLinkOutput
	if err := json.Unmarshal([]byte(link.body), &made2.Body); err != nil {
		t.Fatal(err)
	}
	joined := harness.do(t, http.MethodPost, "/together/join", "",
		map[string]any{"secret": made2.Body.Secret, "name": "Kim"})
	var membership JoinTogetherOutput
	if err := json.Unmarshal([]byte(joined.body), &membership.Body); err != nil || joined.status != http.StatusOK {
		t.Fatalf("POST /together/join = %d %s", joined.status, joined.body)
	}
	wantError(t, harness.do(t, http.MethodPost, "/together/join", "",
		map[string]any{"secret": made2.Body.Secret, "name": "stranger"}), http.StatusNotFound, CodeNotFound)
	left := harness.do(t, http.MethodPost, "/together/leave", membership.Body.MembershipCredential, nil)
	if left.status != http.StatusNoContent {
		t.Errorf("POST /together/leave = %d %s", left.status, left.body)
	}
	wantError(t, harness.do(t, http.MethodDelete, path, watcher, nil), http.StatusForbidden, CodeForbidden)
	if ended := harness.do(t, http.MethodDelete, path, admin, nil); ended.status != http.StatusNoContent {
		t.Errorf("DELETE %s = %d %s", path, ended.status, ended.body)
	}
}

// Every change to who is in a session is logged, with its ids: a member who vanished from a host's
// list on staging could be explained only by reading the code (2026-10-06).
func TestTogether_LogsWhoOpenedJoinedLeftAndEnded(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	path := fmt.Sprintf("/together/%d", opened.Body.SessionID)
	var link TogetherLinkOutput
	if err := json.Unmarshal([]byte(harness.do(t, http.MethodPost, path+"/links", admin,
		map[string]string{"adminNote": "a cousin"}).body), &link.Body); err != nil {
		t.Fatal(err)
	}
	var membership JoinTogetherOutput
	if err := json.Unmarshal([]byte(harness.do(t, http.MethodPost, "/together/join", "",
		map[string]any{"secret": link.Body.Secret, "name": "Kim"}).body), &membership.Body); err != nil {
		t.Fatal(err)
	}
	harness.do(t, http.MethodPost, "/together/leave", membership.Body.MembershipCredential, nil)
	harness.do(t, http.MethodDelete, path, admin, nil)
	logged := harness.logs.String()
	for _, want := range []string{
		"watch together: a host opened a session", "watch together: the host made a link",
		"watch together: someone joined by a link", "watch together: a member left at their own asking",
		"watch together: the host ended the session",
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("the log does not say %q", want)
		}
	}
	if !strings.Contains(logged, fmt.Sprintf(`"memberId":%d`, membership.Body.MemberID)) {
		t.Errorf("the log does not name the member who joined and left, %d", membership.Body.MemberID)
	}
}

// A device's connection is logged with whose it is and which browser: the log is read once its welcome came, which
// the server sends after saying so, and before anything else could write.
func TestTogether_LogsADeviceConnecting(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	membership := harness.joinByLink(t, admin, opened.Body.SessionID, "my partner", "Anna")
	harness.dialTogether(t, membership.Body.MembershipCredential).awaitWelcome()
	ids := fmt.Sprintf(`"sessionId":%d,"memberId":%d`, opened.Body.SessionID, membership.Body.MemberID)
	for line := range strings.Lines(harness.logs.String()) {
		if strings.Contains(line, `"watch together: a device connected"`) && strings.Contains(line, ids) &&
			strings.Contains(line, `"userAgent":"Go-http-client/1.1"`) {
			return
		}
	}
	t.Errorf("the log does not say a device of %s connected:\n%s", ids, harness.logs.String())
}

// A device that sends nothing for the silence limit is taken for gone: its socket is closed, it is
// detached, and the log says why. A phone's connection cut on an airport's network stayed attached,
// and was waited for, though it said nothing more (staging, 2026-10-06).
func TestTogether_ASilentDeviceIsClosedAndSaysWhy(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	harness.server.togetherSilence.Store(int64(50 * time.Millisecond))
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	membership := harness.joinByLink(t, admin, opened.Body.SessionID, "my partner", "Anna")
	viewer := harness.dialTogether(t, membership.Body.MembershipCredential)
	viewer.awaitWelcome()
	harness.logs.AwaitLine(t, `"watch together: a device disconnected"`,
		fmt.Sprintf(`"memberId":%d`, membership.Body.MemberID), "sent nothing for 50ms")
}

// The server takes a device for gone after three of the pings the web client sends, so a client
// pinging on time is never closed for silence.
func TestTogether_TheSilenceLimitIsThreeOfTheClientsPings(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "..", "web", "packages", "sdk", "src", "together-connection.ts")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`export const pingEveryMs = ([\d_]+);`).FindSubmatch(source)
	if found == nil {
		t.Fatal("together-connection.ts has no pingEveryMs this test can read; say where the client's ping went")
	}
	milliseconds, err := strconv.Atoi(strings.ReplaceAll(string(found[1]), "_", ""))
	if err != nil {
		t.Fatal(err)
	}
	if web := time.Duration(milliseconds) * time.Millisecond; web != clientPingEvery || silenceLimit != 3*web {
		t.Errorf("the client pings every %v and the server waits %v; want the server's clientPingEvery %v, "+
			"and a limit of three pings", web, silenceLimit, clientPingEvery)
	}
}

// Each member, the host and a viewer, is handed relay credentials of their own for the session
// they are in, by their membership credential, and nothing for another session (D243 §8).
func TestTogether_EachMemberIsHandedRelayCredentialsOfTheirOwn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	admin := harness.createAdminAndSignIn(t)
	opened := harness.openTogether(t, admin)
	membership := harness.joinByLink(t, admin, opened.Body.SessionID, "my partner", "Anna")
	path := fmt.Sprintf("/together/%d/ice", opened.Body.SessionID)
	usernames := map[string]bool{}
	for _, credential := range []string{opened.Body.MembershipCredential, membership.Body.MembershipCredential} {
		answer := harness.do(t, http.MethodGet, path, credential, nil)
		var ice ICEOutput
		if err := json.Unmarshal([]byte(answer.body), &ice.Body); err != nil || answer.status != http.StatusOK ||
			len(ice.Body.ICEServers) != 1 || ice.Body.ICEServers[0].Credential == "" {
			t.Fatalf("GET %s = %d %s", path, answer.status, answer.body)
		}
		if urls := strings.Join(ice.Body.ICEServers[0].URLs, " "); !strings.Contains(urls, "turn:203.0.113.7:3478") {
			t.Errorf("the relay's URLs = %s; want the relay's public address", urls)
		}
		usernames[ice.Body.ICEServers[0].Username] = true
	}
	if len(usernames) != 2 {
		t.Errorf("the host and the viewer were handed %d usernames; want one each", len(usernames))
	}
	wantError(t, harness.do(t, http.MethodGet, fmt.Sprintf("/together/%d/ice", opened.Body.SessionID+100),
		membership.Body.MembershipCredential, nil), http.StatusNotFound, CodeNotFound)
	wantError(t, harness.do(t, http.MethodGet, path, "not-a-credential", nil), http.StatusNotFound, CodeNotFound)
}
