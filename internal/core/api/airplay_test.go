// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// airPlayGrant is what POST /airplay/grants answers.
type airPlayGrant struct {
	MasterPath string    `json:"masterPath"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// makeAirPlayGrant asks for an AirPlay grant for the playback mediaToken names, and gives its answer
// back read.
func (harness testHarness) makeAirPlayGrant(t *testing.T, user, mediaToken string) (int, airPlayGrant) {
	t.Helper()
	answer := harness.do(t, http.MethodPost, "/airplay/grants", user, map[string]string{"mediaToken": mediaToken})
	var made airPlayGrant
	if answer.status == http.StatusOK {
		if err := json.Unmarshal([]byte(answer.body), &made); err != nil {
			t.Fatalf("%s: %v", answer.body, err)
		}
	}
	return answer.status, made
}

// getAsAirPlayReceiver GETs path as an AirPlay receiver does: Apple's player, sending no cookie and no
// header of ours, from the test server's own address, 127.0.0.1.
func (harness testHarness) getAsAirPlayReceiver(t *testing.T, path string) byteAnswer {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, harness.httpServer.URL+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := harness.httpServer.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = answer.Body.Close() }() //nolint:errcheck // read to the end below
	text, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatal(err)
	}
	return byteAnswer{status: answer.StatusCode, header: answer.Header, body: string(text)}
}

// serveAsAirPlayReceiver serves a GET of path to a receiver whose connection is from remoteAddr,
// with header's headers, through the server's whole handler, as an address no test connection can
// come from; ctx ends the request as its receiver going away does.
func (harness testHarness) serveAsAirPlayReceiver(
	ctx context.Context, path, remoteAddr string, header http.Header,
) *httptest.ResponseRecorder {
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, path, http.NoBody)
	request.RemoteAddr = remoteAddr
	maps.Copy(request.Header, header)
	recorder := httptest.NewRecorder()
	harness.server.Handler().ServeHTTP(recorder, request)
	return recorder
}

// An AirPlay grant, in the media's path, serves its playback's playlists to a receiver that sends no
// credential, for a day; every URI in them is relative, so the grant carries on to each playlist and
// chunk the receiver fetches (D473 §1, §2).
func TestAirPlay_AGrantInThePathServesItsPlaybackToAReceiverWithNoCredential(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	status, made := harness.makeAirPlayGrant(t, user, harness.play(t, user, chrome(1)).MediaToken)
	if status != http.StatusOK || !strings.HasPrefix(made.MasterPath, api.Prefix+"/bytes/media/g/") ||
		!strings.HasSuffix(made.MasterPath, "/master.m3u8") {
		t.Fatalf("the grant: %d %+v; want a master playlist under /bytes/media/g/", status, made)
	}
	if want := harness.clock.Now().Add(24 * time.Hour); !made.ExpiresAt.Equal(want) {
		t.Errorf("the grant expires at %v; want a day from now, %v", made.ExpiresAt, want)
	}
	master := harness.getAsAirPlayReceiver(t, made.MasterPath)
	checkFilmOnesMaster(t, master, "sound0-96k-raw")
	if strings.Contains(master.body, "/") && strings.Contains(master.body, "://") ||
		strings.Contains(master.body, "/api/") {
		t.Errorf("the master playlist names an absolute URI, which the grant would not carry to:\n%s", master.body)
	}
	base := strings.TrimSuffix(made.MasterPath, "master.m3u8")
	stream := harness.getAsAirPlayReceiver(t, base+"q1200k480p/stream.m3u8")
	if stream.status != http.StatusOK || !strings.Contains(stream.body, "\n00002.m4s\n#EXT-X-ENDLIST") {
		t.Errorf("a quality's playlist under the grant: %d\n%s", stream.status, stream.body)
	}
	if refused := harness.getAsAirPlayReceiver(t, base+"q3000k720p/stream.m3u8"); refused.status !=
		http.StatusNotFound {
		t.Errorf("a quality the playback is not offered, under the grant: %d, want 404", refused.status)
	}
	unknown := harness.getAsAirPlayReceiver(t, api.Prefix+"/bytes/media/g/not-a-grant/master.m3u8")
	if unknown.status != http.StatusNotFound {
		t.Errorf("an unknown grant: %d, want 404", unknown.status)
	}
	harness.clock.Advance(24 * time.Hour)
	if expired := harness.getAsAirPlayReceiver(t, made.MasterPath); expired.status != http.StatusNotFound {
		t.Errorf("the grant once its day is over: %d, want 404", expired.status)
	}
}

// A grant is made only for a playback its caller's session holds, and serves only that playback's
// film, under /g/: it signs no one in, so presented as a credential on any other media path it is
// refused, and another session's playback gets none.
func TestAirPlay_AGrantIsForItsSessionsPlaybackAloneAndSignsNoOneIn(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	other := harness.signIn(t, "owner", "owner-password")
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	othersToken := harness.play(t, other, chrome(1)).MediaToken
	if status, _ := harness.makeAirPlayGrant(t, user, othersToken); status != http.StatusNotFound {
		t.Errorf("a grant for another session's playback: %d, want 404", status)
	}
	status, made := harness.makeAirPlayGrant(t, user, harness.play(t, user, chrome(1)).MediaToken)
	if status != http.StatusOK {
		t.Fatalf("the grant: %d", status)
	}
	grant := strings.TrimSuffix(strings.TrimPrefix(made.MasterPath, api.Prefix+"/bytes/media/g/"), "/master.m3u8")
	othersMaster := api.Prefix + "/bytes/media/chunks/" + othersToken + "/master.m3u8"
	for _, byCookie := range []bool{false, true} {
		refused := harness.getBytePath(t, othersMaster, grant, byCookie, "")
		if refused.status != http.StatusUnauthorized {
			t.Errorf("the grant as a credential (by cookie %v) on another playback's path: %d, want 401",
				byCookie, refused.status)
		}
	}
}

// Stop revokes the session's grants, and says how many: a receiver presenting one is refused from
// then on (D473 §2).
func TestAirPlay_StopRevokesTheGrant(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	_, made := harness.makeAirPlayGrant(t, user, harness.play(t, user, chrome(1)).MediaToken)
	if before := harness.getAsAirPlayReceiver(t, made.MasterPath); before.status != http.StatusOK {
		t.Fatalf("before Stop: %d", before.status)
	}
	stopped := harness.do(t, http.MethodDelete, "/airplay/grants", user, nil)
	if stopped.status != http.StatusOK || strings.TrimSpace(stopped.body) != `{"revoked":1}` {
		t.Errorf("Stop: %d %s; want one grant revoked", stopped.status, stopped.body)
	}
	if after := harness.getAsAirPlayReceiver(t, made.MasterPath); after.status != http.StatusNotFound {
		t.Errorf("after Stop: %d, want 404", after.status)
	}
	base := strings.TrimSuffix(made.MasterPath, "master.m3u8")
	if after := harness.getAsAirPlayReceiver(t, base+"q1200k480p/stream.m3u8"); after.status != http.StatusNotFound {
		t.Errorf("a stream after Stop: %d, want 404", after.status)
	}
}

// A grant ends with the sign-in that made it (D473 §2): signed out, the receiver presenting it is
// refused as for a grant unknown.
func TestAirPlay_AGrantEndsWithTheSignInThatMadeIt(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	_, made := harness.makeAirPlayGrant(t, user, harness.play(t, user, chrome(1)).MediaToken)
	if before := harness.getAsAirPlayReceiver(t, made.MasterPath); before.status != http.StatusOK {
		t.Fatalf("before signing out: %d", before.status)
	}
	if out := harness.do(t, http.MethodDelete, "/session", user, nil); out.status/100 != 2 {
		t.Fatalf("sign out: %d %s", out.status, out.body)
	}
	if after := harness.getAsAirPlayReceiver(t, made.MasterPath); after.status != http.StatusNotFound {
		t.Errorf("after its sign-in ended: %d, want 404", after.status)
	}
}

// A grant is bound to the first address that presents it, by the connection's own address and never
// a header's (D473 §3): from another IPv4 address it is answered exactly as a grant unknown, whatever
// Forwarded or X-Forwarded-For says, and from its own it is served whatever they say; the first IPv6
// address binds its /64 beside it.
func TestAirPlay_AGrantIsBoundToTheConnectionsAddressAndNoHeaders(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	_, made := harness.makeAirPlayGrant(t, user, harness.play(t, user, chrome(1)).MediaToken)
	if first := harness.getAsAirPlayReceiver(t, made.MasterPath); first.status != http.StatusOK {
		t.Fatalf("the first request, from 127.0.0.1: %d", first.status)
	}
	claimsTheFirst := http.Header{
		"X-Forwarded-For": {"127.0.0.1"}, "X-Real-Ip": {"127.0.0.1"}, "Forwarded": {"for=127.0.0.1"},
	}
	unknownPath := api.Prefix + "/bytes/media/g/not-a-grant/master.m3u8"
	unknown := harness.serveAsAirPlayReceiver(t.Context(), unknownPath, "192.0.2.50:4321", claimsTheFirst)
	refused := harness.serveAsAirPlayReceiver(t.Context(), made.MasterPath, "192.0.2.50:4321", claimsTheFirst)
	if refused.Code != http.StatusNotFound || refused.Code != unknown.Code ||
		refused.Body.String() != unknown.Body.String() {
		t.Errorf("from another IPv4 address claiming the first: %d %s; want what an unknown grant gets, %d %s",
			refused.Code, refused.Body, unknown.Code, unknown.Body)
	}
	claimsAnother := http.Header{"X-Forwarded-For": {"192.0.2.50"}, "Forwarded": {"for=192.0.2.50"}}
	if own := harness.serveAsAirPlayReceiver(t.Context(), made.MasterPath, "127.0.0.1:5555",
		claimsAnother); own.Code != http.StatusOK {
		t.Errorf("from its own address on another port, claiming another: %d %s; want it served", own.Code, own.Body)
	}
	if sixFirst := harness.serveAsAirPlayReceiver(t.Context(), made.MasterPath, "[2001:db8:1:2::10]:443",
		nil); sixFirst.Code != http.StatusOK {
		t.Errorf("the first IPv6 address: %d; want it bound and served", sixFirst.Code)
	}
	if sixSame := harness.serveAsAirPlayReceiver(t.Context(), made.MasterPath, "[2001:db8:1:2:bbbb::7]:443",
		nil); sixSame.Code != http.StatusOK {
		t.Errorf("another address in the first IPv6 address's /64: %d; want it served", sixSame.Code)
	}
	if sixOther := harness.serveAsAirPlayReceiver(t.Context(), made.MasterPath, "[2001:db8:1:3::10]:443",
		nil); sixOther.Code != http.StatusNotFound || sixOther.Body.String() != unknown.Body.String() {
		t.Errorf("an IPv6 address in another /64: %d %s; want what an unknown grant gets", sixOther.Code,
			sixOther.Body)
	}
}

// D154 §6, as D473 §1 amends it: no endpoint reads a token from a query string or path segment,
// except the media endpoints under /g/, which accept only an AirPlay grant. Every operation a request
// with no credential may call, and which reads a path or query parameter, is one of those two, or the
// reachability echo, whose nonce is no credential: it grants nothing, and is echoed back alone.
func TestAirPlay_OnlyTheMediaUnderGTakesACredentialInThePath(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	var found []string
	for path, item := range harness.server.OpenAPI().Paths {
		for method, operation := range map[string]*huma.Operation{
			http.MethodGet: item.Get, http.MethodPost: item.Post, http.MethodPut: item.Put,
			http.MethodPatch: item.Patch, http.MethodDelete: item.Delete,
		} {
			if operation == nil || !admitsNoCredential(operation.Security) {
				continue
			}
			for _, parameter := range operation.Parameters {
				if parameter.In == "path" || parameter.In == "query" {
					found = append(found, method+" "+path)
					break
				}
			}
		}
	}
	slices.Sort(found)
	want := []string{
		"GET /bytes/media/g/{grant}/master.m3u8",
		"GET /bytes/media/g/{grant}/{streamName}/{fileName}",
		"GET /reachability/{nonce}",
	}
	if !slices.Equal(found, want) {
		t.Errorf("the operations callable with no credential that read the path or query: %v; want %v", found, want)
	}
}

// admitsNoCredential is whether an operation's security lets a request with no credential call it:
// none declared, or one requirement of no scheme.
func admitsNoCredential(security []map[string][]string) bool {
	if len(security) == 0 {
		return true
	}
	for _, requirement := range security {
		if len(requirement) == 0 {
			return true
		}
	}
	return false
}

// A master playlist asked under a grant for a film with no grid yet is held while it is read through,
// for the grant's hold, before the receiver is told to ask again: AVPlayer does not honour the 425
// that hls.js is answered at once (airplay.md, the 2026-11-12 findings).
func TestAirPlay_AMasterWithNoGridIsHeldForTheReceiver(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	token := harness.play(t, user, chrome(1)).MediaToken
	_, made := harness.makeAirPlayGrant(t, user, token)
	askedAt := harness.clock.Now()
	held := harness.getAsAirPlayReceiver(t, made.MasterPath)
	if held.status != http.StatusTooEarly || harness.clock.Now().Sub(askedAt) < 30*time.Second {
		t.Errorf("under a grant, a film with no grid: %d after %v; want 425 after a 30 s hold", held.status,
			harness.clock.Now().Sub(askedAt))
	}
	askedAt = harness.clock.Now()
	early := harness.getBytePath(t, api.Prefix+"/bytes/media/chunks/"+token+"/master.m3u8", user, true, "")
	if early.status != http.StatusTooEarly || harness.clock.Now().Sub(askedAt) != 0 {
		t.Errorf("by the token, for hls.js: %d after %v; want 425 at once", early.status,
			harness.clock.Now().Sub(askedAt))
	}
}

// heldChunk sets up film 1's chunk 1 of q700k432p as one its node makes after asks quarter-second
// asks, and gives the AirPlay grant's path to it.
func (harness testHarness) heldChunk(t *testing.T, user string, asks int) (chunkPath, target string) {
	t.Helper()
	// Played before the film is cut, so not on its ladder: no run is started for its start (D260 §1).
	token := harness.play(t, user, chrome(1)).MediaToken
	harness.keepFilmOnesTimeline(t)
	_, made := harness.makeAirPlayGrant(t, user, token)
	grid, _, err := harness.chunks.Grid(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	target = nodeproto.PathBytesChunks + "1/chunks-q700k432p/" + harness.filmOnesStoreKey(t, "q700k432p",
		grid.ID()) + "/00001.m4s"
	harness.nodeBytes.mutex.Lock()
	harness.nodeBytes.files[target] = "chunk one"
	harness.nodeBytes.asksBeforeFound = map[string]int{target: asks}
	harness.nodeBytes.mutex.Unlock()
	return strings.TrimSuffix(made.MasterPath, "master.m3u8") + "q700k432p/00001.m4s", target
}

// A chunk not made yet is held under a grant until it is made, past the second or so the lab's
// second run saw 425s after, and served: hls.js's chunkHoldMs, 0 by default, does not cut it short
// (D473; the lab's second run, 2026-11-13).
func TestAirPlay_AChunkNotMadeYetIsHeldUntilItIsMade(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	chunkPath, _ := harness.heldChunk(t, user, 12)
	askedAt := harness.clock.Now()
	chunk := harness.getAsAirPlayReceiver(t, chunkPath)
	if chunk.status != http.StatusOK || chunk.body != "chunk one" {
		t.Errorf("a chunk made 3 s after it was asked, under a grant: %d %q; want it", chunk.status, chunk.body)
	}
	if held := harness.clock.Now().Sub(askedAt); held != 3*time.Second {
		t.Errorf("held %v; want the 3 s until it was made", held)
	}
	harness.logs.AwaitEvent(t, "chunkWaited", `"wantedChunk":1,`, `"outcome":"served"`, `"heldMs":3000,`)
	harness.logs.AwaitEvent(t, "airPlayRequest", `"file":"q700k432p/00001.m4s"`, `"outcome":"answered"`,
		`"status":200`)
}

// A request under a grant whose receiver goes away while it is held, as AVPlayer cancels a chunk it
// no longer wants, is logged as gone: never as answered 425, which the lab's second run logged
// (2026-11-13), nor as a fault.
func TestAirPlay_AReceiverGoneWhileHeldIsLoggedAsGone(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	chunkPath, target := harness.heldChunk(t, user, 100)
	ctx, leave := context.WithCancel(t.Context())
	defer leave()
	asks := 0
	harness.nodeBytes.mutex.Lock()
	harness.nodeBytes.whenAsked = func(asked string) {
		if asked != target {
			return
		}
		asks++
		if asks == 3 {
			leave()
		}
	}
	harness.nodeBytes.mutex.Unlock()
	gone := harness.serveAsAirPlayReceiver(ctx, chunkPath, "127.0.0.1:6000", nil)
	if gone.Code == http.StatusTooEarly || gone.Code == http.StatusInternalServerError {
		t.Errorf("a receiver gone while held was answered %d; want neither 425 nor 500", gone.Code)
	}
	line := harness.logs.AwaitEvent(t, "airPlayRequest", `"file":"q700k432p/00001.m4s"`, `"outcome":"clientGone"`)
	if strings.Contains(line, `"status"`) {
		t.Errorf("a gone receiver's line names a status, which never reached it:\n%s", line)
	}
	harness.logs.AwaitEvent(t, "chunkWaited", `"wantedChunk":1,`, `"outcome":"playerGone"`)
	for _, logged := range testutil.LinesWith(harness.logs.String(), "answered with an error") {
		if strings.Contains(logged, "00001.m4s") {
			t.Errorf("a gone receiver's request is logged as answered with an error:\n%s", logged)
		}
	}
}

// A request under a grant whose receiver went away before the core could read what it needed is no
// internal error (500), which the lab's second run logged 26 of (2026-11-13): it is logged as gone.
func TestAirPlay_AReceiverGoneBeforeItsAnswerIsNoInternalError(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	_, made := harness.makeAirPlayGrant(t, user, harness.play(t, user, chrome(1)).MediaToken)
	ctx, leave := context.WithCancel(t.Context())
	leave()
	gone := harness.serveAsAirPlayReceiver(ctx, made.MasterPath, "127.0.0.1:6001", nil)
	if gone.Code == http.StatusInternalServerError {
		t.Errorf("a receiver gone before its answer was answered %d %s; want no internal error", gone.Code,
			gone.Body)
	}
	harness.logs.AwaitEvent(t, "airPlayRequest", `"file":"master.m3u8"`, `"outcome":"clientGone"`)
	if logged := harness.logs.String(); strings.Contains(logged, `"status":"500"`) {
		t.Errorf("a gone receiver's request is logged with status 500:\n%s", logged)
	}
}

// Each request under a grant is logged once it ends, at debug: the receiver's address and its own
// words for itself, the file it asked for, how it ended, the status and bytes, and how long it was
// held and took; a refused grant's too. The grant, a bearer credential, is in no line of the log, the
// error line of a refused one included (D194; the owner, 2026-11-13).
func TestAirPlay_EachRequestUnderAGrantIsLoggedWithoutTheGrant(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	harness.shelveFilms(t)
	harness.keepFilmOnesTimeline(t)
	mediaToken := harness.play(t, user, chrome(1)).MediaToken
	_, made := harness.makeAirPlayGrant(t, user, mediaToken)
	grant := strings.TrimSuffix(strings.TrimPrefix(made.MasterPath, api.Prefix+"/bytes/media/g/"), "/master.m3u8")
	harness.getAsAirPlayReceiver(t, made.MasterPath)
	served := harness.logs.AwaitEvent(t, "airPlayRequest", `"file":"master.m3u8"`, `"outcome":"answered"`,
		`"status":200`, `"clientAddress":"127.0.0.1"`, `"userAgent":"Go-http-client/1.1"`, `"heldMs":`, `"tookMs":`)
	if strings.Contains(served, `"bytes":0,`) || !strings.Contains(served, `"bytes":`) {
		t.Errorf("the master's line names no bytes sent:\n%s", served)
	}
	harness.getAsAirPlayReceiver(t, strings.TrimSuffix(made.MasterPath, "master.m3u8")+"q1200k480p/stream.m3u8")
	harness.logs.AwaitEvent(t, "airPlayRequest", `"file":"q1200k480p/stream.m3u8"`, `"status":200`)
	harness.do(t, http.MethodDelete, "/airplay/grants", user, nil)
	harness.getAsAirPlayReceiver(t, made.MasterPath)
	harness.logs.AwaitEvent(t, "airPlayRequest", `"file":"master.m3u8"`, `"status":404`)
	if logged := harness.logs.String(); strings.Contains(logged, grant) {
		t.Errorf("the log names the grant:\n%s", logged)
	}
	for _, line := range testutil.LinesWith(harness.logs.String(), "airPlayRequest") {
		if strings.Contains(line, mediaToken) {
			t.Errorf("a request's line names the media token:\n%s", line)
		}
	}
}
