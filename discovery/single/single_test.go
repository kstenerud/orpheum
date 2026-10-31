// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// testZone is the owner's zone in the tests, and testIdentity the discovery's identity, which the
// cores' signatures are bound to.
const (
	testZone     = "home.example"
	testIdentity = "od1-discoveryhome"
)

// stage is a discovery behind an HTTP server, on a clock the test moves.
type stage struct {
	discovery *Discovery
	server    *httptest.Server
	logs      *testutil.Log
	mutex     sync.Mutex
	now       time.Time
	// waited is each wait the discovery asked for, which passes at once.
	waited []time.Duration
	// published is the operator's published directory, which the test writes.
	published string
}

func newStage(t *testing.T) *stage {
	t.Helper()
	return newStageWithRealm(t, "")
}

// newStageWithRealm is newStage at a discovery whose operator named realm (Parts.Realm).
func newStageWithRealm(t *testing.T, realm string) *stage {
	t.Helper()
	built := &stage{
		now: time.Date(2026, 10, 11, 20, 0, 0, 0, time.UTC), logs: &testutil.Log{},
		published: t.TempDir(),
	}
	held, err := New(Parts{
		Logger:   slog.New(slog.NewTextHandler(built.logs, nil)),
		Identity: testIdentity, Published: built.published, Zone: testZone,
		Store: FileStore{Path: filepath.Join(t.TempDir(), "discovery.json")},
		Now:   built.clock, Random: rand.Reader,
		ICEServers: []discoveryproto.ICEServer{{URLs: []string{"stun:" + testZone + ":3478"}}},
		Realm:      realm,
		Wait: func(_ context.Context, wait time.Duration) {
			built.mutex.Lock()
			defer built.mutex.Unlock()
			built.waited = append(built.waited, wait)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	built.discovery = held
	built.server = httptest.NewServer(built.discovery.Handler())
	t.Cleanup(built.server.Close)
	return built
}

func (built *stage) clock() time.Time {
	built.mutex.Lock()
	defer built.mutex.Unlock()
	return built.now
}

func (built *stage) advance(step time.Duration) {
	built.mutex.Lock()
	defer built.mutex.Unlock()
	built.now = built.now.Add(step)
}

// call makes a call as a core does, returning its status and decoding its answer into into.
func (built *stage) call(t *testing.T, method, path, token string, body, into any) int {
	t.Helper()
	var sent bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&sent).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequestWithContext(t.Context(), method, built.server.URL+path, &sent)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(discoveryproto.ProtocolHeader, discoveryproto.ProtocolVersion)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := built.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read in full below
	if into != nil && response.StatusCode == http.StatusOK {
		if err = json.NewDecoder(response.Body).Decode(into); err != nil {
			t.Fatal(err)
		}
	}
	return response.StatusCode
}

// key is a core's identity key in the tests.
type key struct {
	public  ed25519.PublicKey
	private ed25519.PrivateKey
}

// sign signs a nonce for purpose, bound to the test's discovery and the core, as a core does.
func (signer key) sign(purpose, core, nonce string) []byte {
	return ed25519.Sign(signer.private, proof.Message(purpose, testIdentity, core, nonce))
}

func newKey(t *testing.T) key {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key{public: public, private: private}
}

// nonce asks for a challenge's nonce.
func (built *stage) nonce(t *testing.T, signer key) string {
	t.Helper()
	var challenge discoveryproto.ChallengeResponse
	if status := built.call(t, http.MethodPost, discoveryproto.PathChallenge, "",
		discoveryproto.ChallengeRequest{PublicKey: signer.public}, &challenge); status != http.StatusOK {
		t.Fatalf("a challenge answered %d", status)
	}
	return challenge.Nonce
}

// enrol enrols signer, returning the status and the answer.
func (built *stage) enrol(t *testing.T, signer key) (int, discoveryproto.EnrolResponse) {
	t.Helper()
	nonce := built.nonce(t, signer)
	var enrolled discoveryproto.EnrolResponse
	status := built.call(t, http.MethodPost, discoveryproto.PathEnrol, "", discoveryproto.EnrolRequest{
		PublicKey: signer.public, Nonce: nonce,
		Signature: signer.sign(discoveryproto.PurposeEnrol, proof.KeyLabel(signer.public), nonce),
	}, &enrolled)
	return status, enrolled
}

// session opens a session for the core coreID with signer, as its first instance, returning its token.
func (built *stage) session(t *testing.T, signer key, coreID string) string {
	t.Helper()
	status, token := built.sessionAs(t, signer, coreID, "first")
	if status != http.StatusOK {
		t.Fatalf("a session answered %d", status)
	}
	return token
}

// sessionAs opens a session for the core coreID with signer, as the instance instanceID, returning the
// status and the token.
func (built *stage) sessionAs(t *testing.T, signer key, coreID, instanceID string) (int, string) {
	t.Helper()
	nonce := built.nonce(t, signer)
	var opened discoveryproto.SessionResponse
	status := built.call(t, http.MethodPost, discoveryproto.PathSession, "", discoveryproto.SessionRequest{
		CoreID: coreID, InstanceID: instanceID, Nonce: nonce,
		Signature: signer.sign(discoveryproto.PurposeSession, coreID, nonce),
	}, &opened)
	return status, opened.Token
}

func TestEnrol_HoldsOneCoreAndRefusesAnotherWhileItIsClaimed(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	first, second := newKey(t), newKey(t)
	status, enrolled := built.enrol(t, first)
	if status != http.StatusOK || len(enrolled.CoreID) != coreIDChars || len(enrolled.ClaimCode) != 9 {
		t.Fatalf("enrolling answered %d %+v; want a core's id and its claim code", status, enrolled)
	}
	// While its code lives, another key cannot take a pending core's place: a stranger enrolling
	// first would otherwise hold it and take the owner's signalling (the security audit's A3).
	if pending, _ := built.enrol(t, second); pending != http.StatusConflict {
		t.Fatalf("a second key while the first was pending answered %d; want %d", pending, http.StatusConflict)
	}
	// Once the code expired unclaimed, another key takes its place, and the first's sessions end.
	token := built.session(t, first, enrolled.CoreID)
	built.advance(claimLasts)
	status, replaced := built.enrol(t, second)
	if status != http.StatusOK || replaced.CoreID == enrolled.CoreID {
		t.Fatalf("a second key once the first's code expired answered %d %+v; want a new core", status, replaced)
	}
	asked := built.call(t, http.MethodGet, discoveryproto.PathStatus, token, nil, nil)
	if asked != http.StatusUnauthorized {
		t.Errorf("the replaced core's session answered %d; want %d", asked, http.StatusUnauthorized)
	}
	enrolled = replaced
	if err := built.discovery.Claim(enrolled.ClaimCode); err != nil {
		t.Fatal(err)
	}
	if status, _ := built.enrol(t, first); status != http.StatusConflict {
		t.Errorf("another key while the core is claimed answered %d; want %d", status, http.StatusConflict)
	}
	if status, again := built.enrol(t, second); status != http.StatusOK || again.CoreID != enrolled.CoreID {
		t.Errorf("the claimed core enrolling again answered %d %+v; want its own id", status, again)
	}
}

func TestSession_ANonceWorksOnceAndOnlyForItsPurpose(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	signer := newKey(t)
	_, enrolled := built.enrol(t, signer)
	nonce := built.nonce(t, signer)
	opening := func(purpose string) int {
		return built.call(t, http.MethodPost, discoveryproto.PathSession, "", discoveryproto.SessionRequest{
			CoreID: enrolled.CoreID, InstanceID: "first", Nonce: nonce,
			Signature: signer.sign(purpose, enrolled.CoreID, nonce),
		}, nil)
	}
	if status := opening(discoveryproto.PurposeEnrol); status != http.StatusForbidden {
		t.Errorf("a signature made to enrol opened a session: %d", status)
	}
	nonce = built.nonce(t, signer)
	if status := opening(discoveryproto.PurposeSession); status != http.StatusOK {
		t.Errorf("a fresh nonce signed for a session answered %d", status)
	}
	if status := opening(discoveryproto.PurposeSession); status != http.StatusForbidden {
		t.Errorf("a nonce used twice answered %d; want %d", status, http.StatusForbidden)
	}
	nonce = built.nonce(t, signer)
	built.advance(nonceLasts)
	if status := opening(discoveryproto.PurposeSession); status != http.StatusForbidden {
		t.Errorf("an expired nonce answered %d; want %d", status, http.StatusForbidden)
	}
}

// A stranger asking for challenges again and again keeps nothing on the discovery, and the core still
// opens its session; a nonce the discovery did not make, or one altered, is refused (the security
// audit's S13, 2026-10-13: a cap of 256 outstanding nonces let a stranger refuse the core's sessions).
func TestSession_NoCountOfStrangersChallengesKeepsTheCoreFromItsSession(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	signer := newKey(t)
	_, enrolled := built.enrol(t, signer)
	stranger := newKey(t)
	for range 1000 {
		built.nonce(t, stranger)
	}
	opening := func(nonce string) int {
		return built.call(t, http.MethodPost, discoveryproto.PathSession, "", discoveryproto.SessionRequest{
			CoreID: enrolled.CoreID, InstanceID: "first", Nonce: nonce,
			Signature: signer.sign(discoveryproto.PurposeSession, enrolled.CoreID, nonce),
		}, nil)
	}
	if status := opening(built.nonce(t, signer)); status != http.StatusOK {
		t.Errorf("the core's session after a stranger's thousand challenges answered %d", status)
	}
	issued := []byte(built.nonce(t, signer))
	issued[len(issued)/2] ^= 1
	for _, made := range []string{string(issued), "bm90LWEtbm9uY2UtaXQtbWFkZQ"} {
		if status := opening(made); status != http.StatusForbidden {
			t.Errorf("a nonce the discovery did not make, %q, answered %d", made, status)
		}
	}
}

// The operator's published directory is served under /.well-known/ as it is, byte for byte: the
// address document, the root chain and the release record, each beside its signature, which is what a
// core trusts (the root key audit's §3.1). A file absent, a directory, or a path that leaves the
// directory is not found; a pending core reads them as a claimed one does, with no session.
func TestWellKnown_ServesTheOperatorsSignedDocumentAsItIs(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	server := httptest.NewServer(built.discovery.WellKnownHandler())
	t.Cleanup(server.Close)
	get := func(path string) (int, string) {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }() //nolint:errcheck // read below
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, string(body)
	}
	if status, _ := get("/.well-known/orpheum-discovery"); status != http.StatusNotFound {
		t.Fatalf("with nothing published: %d, want 404", status)
	}
	files := map[string]string{
		"orpheum-discovery": "orpheum-signed address 1\n", "orpheum-discovery.sig": "-----BEGIN SSH SIGNATURE-----",
		"orpheum/root/1": "orpheum-signed root 1\n", "orpheum/release": "orpheum-signed release 1\n",
	}
	for name, contents := range files {
		path := filepath.Join(built.published, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, contents := range files {
		if status, body := get("/.well-known/" + name); status != http.StatusOK || body != contents {
			t.Errorf("%s: %d %q; want the file as written", name, status, body)
		}
	}
	for _, path := range []string{"/.well-known/orpheum/root", "/.well-known/orpheum/root/2", "/.well-known/%2e%2e/x"} {
		if status, _ := get(path); status != http.StatusNotFound {
			t.Errorf("%s: %d; want 404", path, status)
		}
	}
}

func TestClaim_TakesOnlyThePendingCoresOwnCodeInItsTime(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	if err := built.discovery.Claim("abcd-efgh"); !errors.Is(err, ErrNoCore) {
		t.Errorf("claiming with no core = %v; want ErrNoCore", err)
	}
	signer := newKey(t)
	_, enrolled := built.enrol(t, signer)
	if err := built.discovery.Claim("0000-0000"); !errors.Is(err, ErrWrongCode) {
		t.Errorf("another code = %v; want ErrWrongCode", err)
	}
	built.advance(claimLasts)
	if err := built.discovery.Claim(enrolled.ClaimCode); !errors.Is(err, ErrCodeExpired) {
		t.Errorf("an expired code = %v; want ErrCodeExpired", err)
	}
	token := built.session(t, signer, enrolled.CoreID)
	var status discoveryproto.StatusResponse
	built.call(t, http.MethodGet, discoveryproto.PathStatus, token, nil, &status)
	if status.State != discoveryproto.StatePending || status.ClaimCode == enrolled.ClaimCode ||
		status.ClaimCode == "" {
		t.Fatalf("asked after its code expired, the status is %+v; want pending with a new code", status)
	}
	// Typed as a person types it: upper case, no dash.
	if err := built.discovery.Claim(strings.ToUpper(strings.ReplaceAll(status.ClaimCode, "-", ""))); err != nil {
		t.Fatal(err)
	}
	status = discoveryproto.StatusResponse{}
	built.call(t, http.MethodGet, discoveryproto.PathStatus, token, nil, &status)
	if status.State != discoveryproto.StateClaimed || status.Zone != testZone || status.ClaimCode != "" ||
		len(status.ICEServers) != 1 {
		t.Errorf("once claimed the status is %+v; want claimed, the zone and the STUN servers", status)
	}
	if err := built.discovery.Claim(status.ClaimCode); !errors.Is(err, ErrWrongCode) &&
		!errors.Is(err, ErrNotPending) {
		t.Errorf("claiming a claimed core = %v; want it refused", err)
	}
}

func TestHandler_RefusesASignedCallWithoutASessionsToken(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	signer := newKey(t)
	_, enrolled := built.enrol(t, signer)
	for _, call := range [][2]string{
		{http.MethodGet, discoveryproto.PathStatus},
		{http.MethodPut, discoveryproto.PathAddresses},
		{http.MethodDelete, discoveryproto.PathCore},
		{http.MethodPut, discoveryproto.PathACMEChallenge},
	} {
		status := built.call(t, call[0], call[1], "not-a-token", struct{}{}, nil)
		if status != http.StatusUnauthorized {
			t.Errorf("%s %s without a session answered %d", call[0], call[1], status)
		}
	}
	token := built.session(t, signer, enrolled.CoreID)
	built.advance(sessionLasts)
	status := built.call(t, http.MethodGet, discoveryproto.PathStatus, token, nil, nil)
	if status != http.StatusUnauthorized {
		t.Errorf("an expired session answered %d", status)
	}
}

func TestHandler_AnswersAnyMinorOfItsMajorAndRefusesAnotherByName(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	signer, coreID := built.claimed(t)
	token := built.session(t, signer, coreID)
	for _, tc := range []struct {
		path, version string
		wantStatus    int
		wantCode      string
	}{
		{discoveryproto.PathStatus, "1.7", http.StatusOK, ""},
		{discoveryproto.PathStatus, "2.0", http.StatusBadRequest, discoveryproto.RefusalVersion},
		{discoveryproto.PathStatus, "1", http.StatusBadRequest, discoveryproto.RefusalVersion},
		{discoveryproto.PathStatus, "", http.StatusBadRequest, discoveryproto.RefusalVersion},
		{discoveryproto.PathSignal, "2.0", http.StatusBadRequest, discoveryproto.RefusalVersion},
	} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, built.server.URL+tc.path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		if tc.version != "" {
			request.Header.Set(discoveryproto.ProtocolHeader, tc.version)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := built.server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var refusal discoveryproto.Refusal
		_ = json.NewDecoder(response.Body).Decode(&refusal) //nolint:errcheck // a refusal's body, or none
		_ = response.Body.Close()                           //nolint:errcheck // read above
		if response.StatusCode != tc.wantStatus || refusal.Code != tc.wantCode {
			t.Errorf("GET %s saying %q answered %d %q; want %d %q", tc.path, tc.version, response.StatusCode,
				refusal.Code, tc.wantStatus, tc.wantCode)
		}
	}
}

func TestOffer_PassesTheOfferToTheCoreAndReturnsItsAnswer(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	_, err := built.discovery.Offer(t.Context(), discoveryproto.ClientOffer{SDP: "an offer"}, "a-hash", netip.Prefix{})
	if !errors.Is(err, ErrCoreNotConnected) {
		t.Errorf("an offer with no core connected = %v; want ErrCoreNotConnected", err)
	}
	signer := newKey(t)
	_, enrolled := built.enrol(t, signer)
	if err = built.discovery.Claim(enrolled.ClaimCode); err != nil {
		t.Fatal(err)
	}
	token := built.session(t, signer, enrolled.CoreID)
	connection := built.dial(t, token)
	// The core: it answers the first offer and refuses the second.
	go func() {
		for index := 0; ; index++ {
			var offer discoveryproto.SignalMessage
			if wsjson.Read(t.Context(), connection, &offer) != nil {
				return
			}
			// The answer tells what the core was passed: the offer, its hash and its proof.
			reply := discoveryproto.SignalMessage{
				Type: discoveryproto.SignalAnswer, OfferID: offer.OfferID,
				SDP: "answer to " + offer.SDP + " under " + offer.LocatorHash + " proven " + offer.Proof,
			}
			if index > 0 {
				reply = discoveryproto.SignalMessage{
					Type: discoveryproto.SignalRefusal, OfferID: offer.OfferID, Reason: "too many connections",
				}
			}
			if wsjson.Write(t.Context(), connection, reply) != nil {
				return
			}
		}
	}()
	built.logs.AwaitLine(t, "the core's signalling connected")
	answer, err := built.discovery.Offer(t.Context(), discoveryproto.ClientOffer{SDP: "an offer", Proof: "its proof"},
		"a-hash", netip.Prefix{})
	if err != nil || answer.SDP != "answer to an offer under a-hash proven its proof" {
		t.Errorf("the answer was %q, %v; want the offer passed with its hash and proof", answer.SDP, err)
	}
	_, err = built.discovery.Offer(t.Context(), discoveryproto.ClientOffer{SDP: "a second"}, "a-hash", netip.Prefix{})
	if !errors.Is(err, ErrRefused) ||
		!strings.Contains(err.Error(), "too many connections") {
		t.Errorf("a refused offer = %v; want ErrRefused with the core's reason", err)
	}
}

// dial opens the signalling connection with token, closed when the test ends.
func (built *stage) dial(t *testing.T, token string) *websocket.Conn {
	t.Helper()
	address := "ws" + strings.TrimPrefix(built.server.URL, "http") + discoveryproto.PathSignal
	dialing := &websocket.DialOptions{HTTPHeader: http.Header{
		"Authorization": {"Bearer " + token}, discoveryproto.ProtocolHeader: {discoveryproto.ProtocolVersion},
	}}
	connection, _, err := websocket.Dial(t.Context(), address, dialing) //nolint:bodyclose // the library closes it
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.CloseNow() }) //nolint:errcheck // the test is over
	return connection
}

// claimed is a core of a new key, enrolled and claimed.
func (built *stage) claimed(t *testing.T) (key, string) {
	t.Helper()
	signer := newKey(t)
	_, enrolled := built.enrol(t, signer)
	if err := built.discovery.Claim(enrolled.ClaimCode); err != nil {
		t.Fatal(err)
	}
	return signer, enrolled.CoreID
}

// refusalOf is the refusal's code a call answered with, opening a session for a second instance.
func (built *stage) refusalOf(t *testing.T, method, path, token string, body any) (int, string) {
	t.Helper()
	var sent bytes.Buffer
	if err := json.NewEncoder(&sent).Encode(body); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), method, built.server.URL+path, &sent)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(discoveryproto.ProtocolHeader, discoveryproto.ProtocolVersion)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := built.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read in full below
	var refusal discoveryproto.Refusal
	_ = json.NewDecoder(response.Body).Decode(&refusal) //nolint:errcheck // a refusal's body, or none
	return response.StatusCode, refusal.Code
}

// A pending core asks how it stands and may leave, but publishes no address, gets no certificate
// record and holds no signalling connection until it is claimed: a stranger holding a pending core's
// place gains nothing (the security audit's A3).
func TestHandler_APendingCorePublishesNothingUntilItIsClaimed(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	built.discovery.parts.TXT = &txtRecords{}
	signer := newKey(t)
	_, enrolled := built.enrol(t, signer)
	token := built.session(t, signer, enrolled.CoreID)
	name := "_acme-challenge." + enrolled.CoreID + "." + testZone
	for _, call := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPut, discoveryproto.PathAddresses, discoveryproto.AddressesRequest{}},
		{http.MethodPut, discoveryproto.PathLocateHashes, discoveryproto.LocateHashesRequest{}},
		{http.MethodPut, discoveryproto.PathLocators, discoveryproto.LocatorsRequest{}},
		{
			http.MethodPut, discoveryproto.PathACMEChallenge,
			discoveryproto.ACMEChallengeRequest{Name: name, Value: dns01Value},
		},
	} {
		status, code := built.refusalOf(t, call.method, call.path, token, call.body)
		if status != http.StatusForbidden || code != discoveryproto.RefusalNotClaimed {
			t.Errorf("%s %s from a pending core answered %d %q; want %d %q", call.method, call.path, status, code,
				http.StatusForbidden, discoveryproto.RefusalNotClaimed)
		}
	}
	address := "ws" + strings.TrimPrefix(built.server.URL, "http") + discoveryproto.PathSignal
	dialing := &websocket.DialOptions{HTTPHeader: http.Header{
		"Authorization": {"Bearer " + token}, discoveryproto.ProtocolHeader: {discoveryproto.ProtocolVersion},
	}}
	//nolint:bodyclose // the library closes it
	connection, response, err := websocket.Dial(t.Context(), address, dialing)
	if err == nil {
		_ = connection.CloseNow() //nolint:errcheck // it should not have opened
		t.Error("a pending core opened the signalling connection")
	} else if response == nil || response.StatusCode != http.StatusForbidden {
		t.Errorf("a pending core's signalling answered %v; want %d", err, http.StatusForbidden)
	}
	if status := built.call(t, http.MethodGet, discoveryproto.PathStatus, token, nil, nil); status != http.StatusOK {
		t.Errorf("a pending core asking how it stands answered %d", status)
	}
	if err := built.discovery.Claim(enrolled.ClaimCode); err != nil {
		t.Fatal(err)
	}
	if status := built.call(t, http.MethodPut, discoveryproto.PathACMEChallenge, token,
		discoveryproto.ACMEChallengeRequest{Name: name, Value: dns01Value}, nil); status != http.StatusOK {
		t.Errorf("the claimed core's certificate record answered %d", status)
	}
	// A claimed core reports its ACME account, which the owner's own DNS needs nothing of.
	if status := built.call(t, http.MethodPut, discoveryproto.PathACMEAccount, token,
		discoveryproto.ACMEAccountRequest{AccountURI: "https://ca.example/acct/1"}, nil); status !=
		http.StatusNoContent {
		t.Errorf("the claimed core's ACME account answered %d; want it taken", status)
	}
}

// dns01Value is a TXT record's value as DNS-01 makes one: a SHA-256 digest in unpadded base64url.
const dns01Value = "LoqXcYV8q5ONbJQxbmR7SCTNo3tiAXDfowyjxAjEuX0"

// txtRecords takes every record it is given, keeping them.
type txtRecords struct {
	mutex   sync.Mutex
	records map[string]string
}

func (records *txtRecords) SetTXT(_ context.Context, name, value string) error {
	records.mutex.Lock()
	defer records.mutex.Unlock()
	if records.records == nil {
		records.records = map[string]string{}
	}
	records.records[name] = value
	return nil
}

func (records *txtRecords) ClearTXT(_ context.Context, name string) error {
	records.mutex.Lock()
	defer records.mutex.Unlock()
	delete(records.records, name)
	return nil
}

func (records *txtRecords) value(name string) string {
	records.mutex.Lock()
	defer records.mutex.Unlock()
	return records.records[name]
}

func TestACMEChallenge_SetsOnlyTheCoresOwnRecordToADigestAndClearsItByItsName(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	records := &txtRecords{}
	built.discovery.parts.TXT = records
	signer, coreID := built.claimed(t)
	token := built.session(t, signer, coreID)
	name := "_acme-challenge." + coreID + "." + testZone
	for _, tc := range []struct {
		why  string
		sent discoveryproto.ACMEChallengeRequest
		want int
	}{
		{"another core's name", discoveryproto.ACMEChallengeRequest{
			Name: "_acme-challenge.other." + testZone, Value: dns01Value,
		}, http.StatusForbidden},
		{
			"a value DNS-01 never makes",
			discoveryproto.ACMEChallengeRequest{Name: name, Value: "v"},
			http.StatusBadRequest,
		},
		{
			"the core's own record",
			discoveryproto.ACMEChallengeRequest{Name: name, Value: dns01Value},
			http.StatusOK,
		},
	} {
		status := built.call(t, http.MethodPut, discoveryproto.PathACMEChallenge, token, tc.sent, nil)
		if status != tc.want {
			t.Errorf("setting %s answered %d, want %d", tc.why, status, tc.want)
		}
	}
	if got := records.value(name); got != dns01Value {
		t.Fatalf("the record is %q; want the digest set", got)
	}
	status := built.call(t, http.MethodDelete, discoveryproto.PathACMEChallenge+"/"+name, token, nil, nil)
	if status != http.StatusOK {
		t.Errorf("clearing the record by its name answered %d", status)
	}
	if got := records.value(name); got != "" {
		t.Errorf("after clearing, the record is %q; want none", got)
	}
}

// A session for a core the discovery does not hold says to enrol again, which the core acts on
// (the protocol's §2.2, C12).
func TestSession_ForACoreItDoesNotHoldSaysToEnrolAgain(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	signer := newKey(t)
	nonce := built.nonce(t, signer)
	status, code := built.refusalOf(t, http.MethodPost, discoveryproto.PathSession, "", discoveryproto.SessionRequest{
		CoreID: "notheld", InstanceID: "first", Nonce: nonce,
		Signature: signer.sign(discoveryproto.PurposeSession, "notheld", nonce),
	})
	if status != http.StatusForbidden || code != discoveryproto.RefusalEnrolAgain {
		t.Errorf("a session for a core it does not hold answered %d %q; want %d %q", status, code,
			http.StatusForbidden, discoveryproto.RefusalEnrolAgain)
	}
}

// A second instance of the core is admitted at once when the first holds no signalling connection or
// does not answer a ping on it, the first's sessions and connection ending; while the first answers,
// the second is refused as a copy, so a crash-restart waits seconds and two copies never take turns
// (D293 §1).
func TestSession_ASecondInstanceTakesThePlaceOfOneThatDoesNotAnswerAndIsRefusedWhileItDoes(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	built.discovery.pingWait = 200 * time.Millisecond
	signer, coreID := built.claimed(t)
	first := built.session(t, signer, coreID)
	// The first holds no connection: the second takes its place, and the first's session ends.
	status, second := built.sessionAs(t, signer, coreID, "second")
	if status != http.StatusOK {
		t.Fatalf("a second instance while the first held no connection answered %d", status)
	}
	asked := built.call(t, http.MethodGet, discoveryproto.PathStatus, first, nil, nil)
	if asked != http.StatusUnauthorized {
		t.Errorf("the replaced instance's session answered %d; want %d", asked, http.StatusUnauthorized)
	}
	// The second holds its connection and answers pings, as a live core's reader does: the first,
	// asking again, is refused as a copy.
	answering := built.dial(t, second)
	answeringEnded := make(chan struct{})
	go func() {
		defer close(answeringEnded)
		for {
			if _, _, err := answering.Read(t.Context()); err != nil {
				return
			}
		}
	}()
	built.logs.AwaitLine(t, "the core's signalling connected")
	nonce := built.nonce(t, signer)
	status, code := built.refusalOf(t, http.MethodPost, discoveryproto.PathSession, "", discoveryproto.SessionRequest{
		CoreID: coreID, InstanceID: "first", Nonce: nonce,
		Signature: signer.sign(discoveryproto.PurposeSession, coreID, nonce),
	})
	if status != http.StatusConflict || code != discoveryproto.RefusalAnotherInstance {
		t.Fatalf("a copy while the live instance answers got %d %q; want %d %q", status, code,
			http.StatusConflict, discoveryproto.RefusalAnotherInstance)
	}
	// The second crashed: its connection is still open to the discovery, but nothing reads it, so
	// no ping is answered. The first is admitted, and the second's connection closed.
	silent := built.dial(t, second)
	// The silent connection holds the discovery's place once the one it replaced has been closed.
	testutil.Receive(t, answeringEnded, "the answering connection to be replaced")
	if status, _ := built.sessionAs(t, signer, coreID, "first"); status != http.StatusOK {
		t.Fatalf("an instance after the live one stopped answering got %d", status)
	}
	if _, _, err := silent.Read(t.Context()); err == nil {
		t.Error("the silent instance's connection still carries messages; want it dropped")
	}
}

func TestLinks_AClaimedCoresFullSetsReplaceTheLast(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	signer, coreID := built.claimed(t)
	token := built.session(t, signer, coreID)
	for _, sent := range []discoveryproto.LocateHashesRequest{
		{Hashes: []discoveryproto.LocateHash{{Hash: "first", ExpiresIn: 60}, {Hash: "second", ExpiresIn: 60}}},
		{Hashes: []discoveryproto.LocateHash{{Hash: "second", ExpiresIn: 120}}},
	} {
		status := built.call(t, http.MethodPut, discoveryproto.PathLocateHashes, token, sent, nil)
		if status != http.StatusNoContent {
			t.Fatalf("sending the links answered %d", status)
		}
	}
	if status := built.call(t, http.MethodPut, discoveryproto.PathLocators, token,
		discoveryproto.LocatorsRequest{Hashes: []string{"a-locator"}}, nil); status != http.StatusNoContent {
		t.Fatalf("sending the locators answered %d", status)
	}
	built.discovery.mutex.Lock()
	defer built.discovery.mutex.Unlock()
	kept := built.discovery.locateHashes
	if len(kept) != 1 || !kept["second"].expires.Equal(built.clock().Add(2*time.Minute)) {
		t.Errorf("the links kept are %v; want the last set alone, each ending when it said", kept)
	}
	if !built.discovery.locators["a-locator"] || len(built.discovery.locators) != 1 {
		t.Errorf("the locators kept are %v; want the one sent", built.discovery.locators)
	}
}
