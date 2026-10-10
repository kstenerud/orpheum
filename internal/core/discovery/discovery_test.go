// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/internal/core/certificate"
	"github.com/kstenerud/orpheum/internal/core/certificate/certificatetest"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/discoveryproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// memoryFiles is a persistent directory in memory.
type memoryFiles struct {
	mutex sync.Mutex
	files map[string][]byte
}

func (files *memoryFiles) ReadPersistentFile(name string) ([]byte, error) {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	contents, found := files.files[name]
	if !found {
		return nil, instancedir.ErrNotFound
	}
	return contents, nil
}

func (files *memoryFiles) WritePersistentFile(name string, contents []byte) error {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	files.files[name] = contents
	return nil
}

func (files *memoryFiles) RemovePersistentFile(name string) error {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	delete(files.files, name)
	return nil
}

// fakeDiscovery speaks the discovery's side of the protocol, checking every signature, and keeps
// what the core sent.
type fakeDiscovery struct {
	t     *testing.T
	mutex sync.Mutex
	// Guarded by mutex:
	nonces       map[string]bool
	publicKey    ed25519.PublicKey
	state        discoveryproto.State
	tokens       map[string]bool
	heartbeats   []discoveryproto.AddressesRequest
	deprecated   string
	isLeft       bool
	sessionCount int
	// txt are the certificate challenges' records the core had the discovery set, by name.
	txt map[string]string
	// signals are the offers the test has the discovery pass on, and replies the core's answers.
	signals chan discoveryproto.SignalMessage
	replies chan discoveryproto.SignalMessage
}

// fakeCoreID and fakeZone are the core's id and zone at the fake discovery.
const (
	fakeCoreID = "k7xq2"
	fakeZone   = "orpheum-core.example"
	fakeSTUN   = "stun:discovery.orpheum-core.example:3478"
)

func startDiscovery(t *testing.T) (*fakeDiscovery, *httptest.Server) {
	t.Helper()
	fake := &fakeDiscovery{
		t: t, nonces: map[string]bool{}, tokens: map[string]bool{}, txt: map[string]string{},
		signals: make(chan discoveryproto.SignalMessage), replies: make(chan discoveryproto.SignalMessage, 4),
	}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return fake, server
}

func (fake *fakeDiscovery) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == discoveryproto.PathSignal {
		fake.serveSignal(writer, request)
		return
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if request.Header.Get(discoveryproto.ProtocolHeader) != discoveryproto.ProtocolVersion {
		http.Error(writer, "say the protocol", http.StatusBadRequest)
		return
	}
	if fake.deprecated != "" {
		writer.Header().Set(discoveryproto.DeprecatedHeader, fake.deprecated)
	}
	route := request.Method + " " + request.URL.Path
	switch route {
	case "POST /challenge":
		nonce := rand.Text()
		fake.nonces[nonce] = true
		fake.answer(writer, discoveryproto.ChallengeResponse{Nonce: nonce})
	case "POST /enrol":
		var enrol discoveryproto.EnrolRequest
		fake.read(request, &enrol)
		if !fake.signedFresh(enrol.PublicKey, discoveryproto.PurposeEnrol, enrol.Nonce, enrol.Signature) {
			http.Error(writer, "bad signature", http.StatusForbidden)
			return
		}
		fake.publicKey, fake.state = enrol.PublicKey, discoveryproto.StatePending
		fake.answer(writer, discoveryproto.EnrolResponse{CoreID: fakeCoreID, ClaimCode: "abcd-efgh", ClaimExpiresAt: 1})
	case "POST /session":
		var opened discoveryproto.SessionRequest
		fake.read(request, &opened)
		if opened.CoreID != fakeCoreID ||
			!fake.signedFresh(fake.publicKey, discoveryproto.PurposeSession, opened.Nonce, opened.Signature) {
			http.Error(writer, "bad signature", http.StatusForbidden)
			return
		}
		fake.sessionCount++
		token := rand.Text()
		fake.tokens[token] = true
		fake.answer(writer, discoveryproto.SessionResponse{Token: token, ExpiresAt: 1 << 62})
	default:
		fake.serveSigned(writer, request, route)
	}
}

// serveSigned answers the calls a session's token is needed for.
func (fake *fakeDiscovery) serveSigned(writer http.ResponseWriter, request *http.Request, route string) {
	if !fake.tokens[strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")] {
		http.Error(writer, "no session", http.StatusUnauthorized)
		return
	}
	switch route {
	case "GET /status":
		fake.answer(writer, discoveryproto.StatusResponse{
			State: fake.state, Zone: fakeZone, ICEServers: []discoveryproto.ICEServer{{URLs: []string{fakeSTUN}}},
		})
	case "PUT /addresses":
		var heartbeat discoveryproto.AddressesRequest
		fake.read(request, &heartbeat)
		fake.heartbeats = append(fake.heartbeats, heartbeat)
		fake.answer(writer, discoveryproto.AddressesResponse{
			Published:     []string{"2a02-8070-0-0-0-0-0-1." + fakeCoreID + ".orpheum-core.example"},
			Probe:         []discoveryproto.ProbeResult{{Candidate: "2a0f:1000::1", Result: "reached"}},
			ReachableFrom: discoveryproto.ReachableIPv6OrWebRTC,
		})
	case "DELETE /core":
		fake.isLeft = true
	case "PUT /acme-challenge", "DELETE /acme-challenge":
		var challenge discoveryproto.ACMEChallengeRequest
		fake.read(request, &challenge)
		if challenge.Name != "_acme-challenge."+fakeCoreID+"."+fakeZone {
			http.Error(writer, "not this core's name", http.StatusForbidden)
			return
		}
		if request.Method == http.MethodDelete {
			delete(fake.txt, challenge.Name)
			return
		}
		fake.txt[challenge.Name] = challenge.Value
	default:
		http.NotFound(writer, request)
	}
}

// signedFresh reports whether signature is publicKey's over a nonce the discovery issued and no call
// has used, for purpose.
func (fake *fakeDiscovery) signedFresh(publicKey ed25519.PublicKey, purpose, nonce string, signature []byte) bool {
	if !fake.nonces[nonce] || len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	delete(fake.nonces, nonce)
	return ed25519.Verify(publicKey, identity.SignedMessage(purpose, nonce), signature)
}

func (fake *fakeDiscovery) read(request *http.Request, into any) {
	if err := json.NewDecoder(request.Body).Decode(into); err != nil {
		fake.t.Error(err)
	}
}

func (fake *fakeDiscovery) answer(writer http.ResponseWriter, value any) {
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		fake.t.Error(err)
	}
}

func (fake *fakeDiscovery) claim() {
	fake.become(discoveryproto.StateClaimed)
}

// become puts the core in state at the discovery.
func (fake *fakeDiscovery) become(state discoveryproto.State) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.state = state
}

// TXT is the record of name the discovery holds, empty for none: certificatetest.TXTRecords.
func (fake *fakeDiscovery) TXT(name string) string {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return fake.txt[name]
}

// world is a core's discovery service against a fake discovery, on a fake clock.
type world struct {
	service    *Service
	files      *memoryFiles
	key        identity.Key
	fake       *fakeDiscovery
	baseURL    string
	clock      *clocktest.Fake
	candidates []discoveryproto.Candidate
	// claimedCount is how many times the service said the core was claimed.
	claimedCount atomic.Int32
}

func newWorld(t *testing.T) *world {
	t.Helper()
	key, err := identity.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fake, server := startDiscovery(t)
	built := &world{
		files: &memoryFiles{files: map[string][]byte{}}, key: key, fake: fake, baseURL: server.URL,
		clock:      clocktest.NewFake(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)),
		candidates: []discoveryproto.Candidate{{Address: "2a0f:1000::1", Port: 443, Source: "interface"}},
	}
	built.service = built.restart(t)
	return built
}

// restart builds the service again over the same files, as a core that started again does.
func (built *world) restart(t *testing.T) *Service {
	t.Helper()
	service, err := New(Parts{
		Files: built.files, Key: built.key, HTTP: &http.Client{}, Clock: built.clock,
		Logger: slog.New(slog.DiscardHandler), Version: "v0.1.0", APIMajor: 1,
		Candidates: func() []discoveryproto.Candidate { return built.candidates },
		OnClaimed:  func(context.Context) { built.claimedCount.Add(1) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestConnect_EnrolsWithASignatureAndKeepsTheDiscoveryAcrossARestart(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	status, err := built.service.Connect(t.Context(), built.baseURL)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != discoveryproto.StatePending || status.ClaimCode != "abcd-efgh" || status.CoreID != fakeCoreID {
		t.Errorf("after connecting the status is %+v; want pending, with its claim code", status)
	}
	if _, err = built.service.Connect(t.Context(), built.baseURL); !errors.Is(err, ErrAlreadyEnrolled) {
		t.Errorf("connecting again = %v; want ErrAlreadyEnrolled", err)
	}
	if again := built.restart(t).Status(); again.BaseURL != built.baseURL || again.CoreID != fakeCoreID {
		t.Errorf("after a restart the status is %+v; want the discovery and the core's id kept", again)
	}
}

func TestStep_WaitsToBeClaimedThenSendsItsAddressesWhenTheyChangeOrEveryFiveMinutes(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.baseURL); err != nil {
		t.Fatal(err)
	}
	if wait := built.service.step(ctx); wait != pendingEvery || len(built.fake.heartbeats) != 0 {
		t.Fatalf("a pending core waited %s and sent %d heartbeats; want %s and none", wait,
			len(built.fake.heartbeats), pendingEvery)
	}
	built.fake.claim()
	built.service.step(ctx) // learns it is claimed
	built.service.step(ctx) // sends its addresses
	status := built.service.Status()
	if status.State != discoveryproto.StateClaimed || status.Zone != "orpheum-core.example" ||
		status.ReachableFrom != discoveryproto.ReachableIPv6OrWebRTC || len(status.Probe) != 1 {
		t.Errorf("once claimed the status is %+v; want claimed, its zone and what the probe found", status)
	}
	built.service.step(ctx) // nothing changed, nothing due
	built.candidates = append(built.candidates, discoveryproto.Candidate{
		Address: "203.0.113.7", Port: 51234, Source: "mapped", Method: "pcp",
	})
	built.service.step(ctx) // an address appeared: sent at once
	built.clock.Advance(heartbeatEvery)
	built.service.step(ctx) // five minutes on: sent again
	if count := len(built.fake.heartbeats); count != 3 {
		t.Fatalf("%d heartbeats; want one when claimed, one when an address appeared, one five minutes on", count)
	}
	sent := built.fake.heartbeats[1]
	if sent.APIMajor != 1 || sent.OrpheumVersion != "v0.1.0" || len(sent.Candidates) != 2 {
		t.Errorf("the heartbeat was %+v; want the version, the API's major and both candidates", sent)
	}
}

func TestStep_OpensANewSessionWhenTheDiscoveryRefusesTheOld(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.baseURL); err != nil {
		t.Fatal(err)
	}
	built.service.step(ctx)
	built.fake.mutex.Lock()
	built.fake.tokens = map[string]bool{} // the discovery restarted and forgot its sessions
	built.fake.mutex.Unlock()
	if wait := built.service.step(ctx); wait != retryAfter || built.service.Status().Problem == "" {
		t.Errorf("a refused session waited %s with problem %q; want %s and the refusal said", wait,
			built.service.Status().Problem, retryAfter)
	}
	built.service.step(ctx)
	if count := built.fake.sessionCount; count != 2 || built.service.Status().Problem != "" {
		t.Errorf("%d sessions opened, problem %q; want a second one opened and the problem cleared", count,
			built.service.Status().Problem)
	}
}

func TestCall_KeepsTheDeprecationTheDiscoveryNames(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	built.fake.deprecated = "2027-04-09"
	if _, err := built.service.Connect(t.Context(), built.baseURL); err != nil {
		t.Fatal(err)
	}
	if until := built.service.Status().DeprecatedUntil; until != "2027-04-09" {
		t.Errorf("the deprecation kept is %q; want the discovery's date (D273 §7)", until)
	}
}

func TestLeave_TellsTheDiscoveryAndForgetsIt(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	if _, err := built.service.Connect(t.Context(), built.baseURL); err != nil {
		t.Fatal(err)
	}
	if err := built.service.Leave(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !built.fake.isLeft || built.service.Status().BaseURL != "" {
		t.Error("leaving did not tell the discovery, or the core still names it")
	}
	if _, err := built.files.ReadPersistentFile(StateFileName); !errors.Is(err, instancedir.ErrNotFound) {
		t.Errorf("%s is still there after leaving: %v", StateFileName, err)
	}
}

func TestCheckBaseURL_RefusesAnythingButHTTPSOrThisMachine(t *testing.T) {
	t.Parallel()
	for url, isGood := range map[string]bool{
		ServiceURL: true, "https://discovery.example.org/core/v1": true, "http://127.0.0.1:8080": true,
		"http://localhost:9": true, "http://discovery.example.org": false, "ftp://x": false, "": false,
	} {
		if err := checkBaseURL(url); (err == nil) != isGood {
			t.Errorf("checkBaseURL(%q) = %v; want accepted: %t", url, err, isGood)
		}
	}
}

// serveSignal is the signalling connection: the offers the test sends go to the core, and its
// answers come back to the test.
func (fake *fakeDiscovery) serveSignal(writer http.ResponseWriter, request *http.Request) {
	fake.mutex.Lock()
	isSignedIn := fake.tokens[strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")]
	fake.mutex.Unlock()
	if !isSignedIn {
		http.Error(writer, "no session", http.StatusUnauthorized)
		return
	}
	connection, err := websocket.Accept(writer, request, nil)
	if err != nil {
		return
	}
	defer func() { _ = connection.CloseNow() }() //nolint:errcheck // the test is over
	ctx := request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case offer := <-fake.signals:
			if err := wsjson.Write(ctx, connection, offer); err != nil {
				return
			}
			var reply discoveryproto.SignalMessage
			if err := wsjson.Read(ctx, connection, &reply); err != nil {
				return
			}
			fake.replies <- reply
		}
	}
}

// answering answers the first offer it is given and refuses the rest, as a core at its limit does.
type answering struct {
	mutex   sync.Mutex
	offered []string
}

func (answerer *answering) Answer(_ context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
	answerer.mutex.Lock()
	defer answerer.mutex.Unlock()
	answerer.offered = append(answerer.offered, offer.SDP)
	if len(answerer.offered) > 1 {
		return rtcapi.Description{}, rtcapi.ErrTooManyConnections
	}
	return rtcapi.Description{Type: "answer", SDP: "answer to " + offer.SDP}, nil
}

func TestSignal_AClaimedCoreAnswersTheOffersItsDiscoveryPassesOn(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	answerer := &answering{}
	built.service.parts.Answerer = answerer
	if _, err := built.service.Connect(t.Context(), built.baseURL); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(t.Context())
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		built.service.signal(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		testutil.Receive(t, done, "the signalling loop to end")
	})
	for index, offer := range []string{"offer one", "offer two"} {
		built.fake.signals <- discoveryproto.SignalMessage{
			Type:    discoveryproto.SignalOffer,
			OfferID: fmt.Sprint(index), SDP: offer,
		}
	}
	answered := testutil.Receive(t, built.fake.replies, "the core's answer")
	refused := testutil.Receive(t, built.fake.replies, "the core's refusal")
	if answered.Type != discoveryproto.SignalAnswer || answered.OfferID != "0" ||
		answered.SDP != "answer to offer one" {
		t.Errorf("the first offer was answered %+v; want its answer, under its id", answered)
	}
	if refused.Type != discoveryproto.SignalRefusal || refused.OfferID != "1" ||
		!strings.Contains(refused.Reason, "as many connections") {
		t.Errorf("the offer past the limit was answered %+v; want a refusal saying why", refused)
	}
}

// claimedWorld is a world whose core the discovery has claimed, its first heartbeat sent.
func claimedWorld(t *testing.T) *world {
	t.Helper()
	built := newWorld(t)
	if _, err := built.service.Connect(t.Context(), built.baseURL); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(t.Context())
	built.service.step(t.Context())
	return built
}

// The core learns its zone when the discovery claims it, says so once, and keeps it, so its
// certificate's name is known from the start after a restart.
func TestStep_SaysOnceWhenTheCoreIsClaimedAndKeepsItsZone(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.baseURL); err != nil {
		t.Fatal(err)
	}
	built.service.step(ctx)
	if count := built.claimedCount.Load(); count != 0 {
		t.Fatalf("a pending core was said claimed %d times; want none", count)
	}
	built.fake.claim()
	for range 3 {
		built.clock.Advance(heartbeatEvery)
		built.service.step(ctx)
	}
	want := "*." + fakeCoreID + "." + fakeZone
	if count, name := built.claimedCount.Load(), built.service.Status().CertificateName(); count != 1 || name != want {
		t.Errorf("once claimed: said %d times, certificate name %q; want once, %q", count, name, want)
	}
	if servers := built.service.Status().ICEServers; len(servers) != 1 || servers[0].URLs[0] != fakeSTUN {
		t.Errorf("the STUN servers kept are %+v; want the discovery's, %s", servers, fakeSTUN)
	}
	if name := built.restart(t).Status().CertificateName(); name != want {
		t.Errorf("after a restart the certificate name is %q; want %q, kept", name, want)
	}
}

// A claimed core asks how it stands with each heartbeat, and stops sending its addresses once its
// subscription has ended.
func TestStep_AClaimedCoreLearnsWhenItsSubscriptionEnds(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	sent := len(built.fake.heartbeats)
	built.fake.become(discoveryproto.StateFrozen)
	built.clock.Advance(heartbeatEvery)
	if wait := built.service.step(t.Context()); wait != endedEvery || len(built.fake.heartbeats) != sent {
		t.Errorf("a frozen core waited %s, %d heartbeats after %d; want %s and none more", wait,
			len(built.fake.heartbeats), sent, endedEvery)
	}
	if state := built.service.Status().State; state != discoveryproto.StateFrozen {
		t.Errorf("the core's state is %q; want frozen", state)
	}
}

// The certificate of the core's names is obtained through its discovery, which holds the DNS-01
// record for it: a wildcard for every name under the core's, set and cleared again (the spec's §3.7).
func TestSetTXT_ACertificateForTheCoresNamesIsObtainedThroughTheDiscovery(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	now := built.clock.Now()
	authority := certificatetest.NewCA(t, built.fake, now, 90*24*time.Hour)
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	name := built.service.Status().CertificateName()
	certificates, err := certificate.New(certificate.Parts{
		Domain: name, DirectoryURL: authority.Directory(), HTTP: httpClient, DNS: built.service,
		Files: certificateFiles{}, Clock: built.clock, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = certificates.Obtain(t.Context()); err != nil {
		t.Fatal(err)
	}
	served, err := certificates.GetCertificate(nil)
	if err != nil || served.Leaf == nil || !slices.Equal(served.Leaf.DNSNames, []string{name}) {
		t.Fatalf("GetCertificate = %+v, %v; want a certificate for %s", served, err, name)
	}
	if left := built.fake.TXT("_acme-challenge." + fakeCoreID + "." + fakeZone); left != "" {
		t.Errorf("the challenge's record is still %q; want it cleared", left)
	}
}

// certificateFiles keeps a certificate's files in memory.
type certificateFiles map[string][]byte

func (files certificateFiles) ReadPersistentFile(name string) ([]byte, error) {
	contents, found := files[name]
	if !found {
		return nil, instancedir.ErrNotFound
	}
	return contents, nil
}

func (files certificateFiles) WritePersistentFile(name string, contents []byte) error {
	files[name] = contents
	return nil
}
