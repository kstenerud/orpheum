// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

// testZone is the owner's zone in the tests.
const testZone = "home.example"

// stage is a discovery behind an HTTP server, on a clock the test moves.
type stage struct {
	discovery *Discovery
	server    *httptest.Server
	logs      *testutil.Log
	mutex     sync.Mutex
	now       time.Time
}

func newStage(t *testing.T) *stage {
	t.Helper()
	built := &stage{now: time.Date(2026, 10, 11, 20, 0, 0, 0, time.UTC), logs: &testutil.Log{}}
	held, err := New(Parts{
		Logger: slog.New(slog.NewTextHandler(built.logs, nil)),
		Zone:   testZone, Store: FileStore{Path: filepath.Join(t.TempDir(), "discovery.json")},
		Now: built.clock, Random: rand.Reader,
		ICEServers: []discoveryproto.ICEServer{{URLs: []string{"stun:" + testZone + ":3478"}}},
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
		PublicKey: signer.public, Protocol: discoveryproto.ProtocolVersion, Nonce: nonce,
		Signature: ed25519.Sign(signer.private, proof.Message(discoveryproto.PurposeEnrol, nonce)),
	}, &enrolled)
	return status, enrolled
}

// session opens a session for the core coreID with signer, returning its token.
func (built *stage) session(t *testing.T, signer key, coreID string) string {
	t.Helper()
	nonce := built.nonce(t, signer)
	var opened discoveryproto.SessionResponse
	if status := built.call(t, http.MethodPost, discoveryproto.PathSession, "", discoveryproto.SessionRequest{
		CoreID: coreID, Nonce: nonce,
		Signature: ed25519.Sign(signer.private, proof.Message(discoveryproto.PurposeSession, nonce)),
	}, &opened); status != http.StatusOK {
		t.Fatalf("a session answered %d", status)
	}
	return opened.Token
}

func TestEnrol_HoldsOneCoreAndRefusesAnotherWhileItIsClaimed(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	first, second := newKey(t), newKey(t)
	status, enrolled := built.enrol(t, first)
	if status != http.StatusOK || len(enrolled.CoreID) != coreIDChars || len(enrolled.ClaimCode) != 9 {
		t.Fatalf("enrolling answered %d %+v; want a core's id and its claim code", status, enrolled)
	}
	// While it is pending, another key takes its place: the owner's core started over.
	if status, replaced := built.enrol(t, second); status != http.StatusOK || replaced.CoreID == enrolled.CoreID {
		t.Fatalf("a second key while the first was pending answered %d %+v; want a new core", status, replaced)
	}
	_, enrolled = built.enrol(t, second)
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
			CoreID: enrolled.CoreID, Nonce: nonce,
			Signature: ed25519.Sign(signer.private, proof.Message(purpose, nonce)),
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
			CoreID: enrolled.CoreID, Nonce: nonce,
			Signature: ed25519.Sign(signer.private, proof.Message(discoveryproto.PurposeSession, nonce)),
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

func TestOffer_PassesTheOfferToTheCoreAndReturnsItsAnswer(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	if _, err := built.discovery.Offer(t.Context(), "an offer"); !errors.Is(err, ErrCoreNotConnected) {
		t.Errorf("an offer with no core connected = %v; want ErrCoreNotConnected", err)
	}
	signer := newKey(t)
	_, enrolled := built.enrol(t, signer)
	token := built.session(t, signer, enrolled.CoreID)
	address := "ws" + strings.TrimPrefix(built.server.URL, "http") + discoveryproto.PathSignal
	dialing := &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + token}}}
	connection, _, err := websocket.Dial(t.Context(), address, dialing) //nolint:bodyclose // the library closes it
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.CloseNow() }) //nolint:errcheck // the test is over
	// The core: it answers the first offer and refuses the second.
	go func() {
		for index := 0; ; index++ {
			var offer discoveryproto.SignalMessage
			if wsjson.Read(t.Context(), connection, &offer) != nil {
				return
			}
			reply := discoveryproto.SignalMessage{
				Type: discoveryproto.SignalAnswer, OfferID: offer.OfferID, SDP: "answer to " + offer.SDP,
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
	answer, err := built.discovery.Offer(t.Context(), "an offer")
	if err != nil || answer != "answer to an offer" {
		t.Errorf("the answer was %q, %v", answer, err)
	}
	if _, err = built.discovery.Offer(t.Context(), "a second"); !errors.Is(err, ErrRefused) ||
		!strings.Contains(err.Error(), "too many connections") {
		t.Errorf("a refused offer = %v; want ErrRefused with the core's reason", err)
	}
}
