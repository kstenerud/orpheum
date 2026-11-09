// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/discovery/release"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

// fakeDiscovery speaks the discovery's side of the protocol, checking every signature, and keeps
// what the core sent.
type fakeDiscovery struct {
	t *testing.T
	// issuer is its issuer, the server's own URL, where the core calls it too unless a test moves its
	// core API to another; chain is its root chain, e1 its everyday key and r1 its recovery key, and
	// document the address document it serves, signed by e1. files, where a path's is set, are served
	// in place of its own as they are.
	issuer   string
	chain    *trusttest.Chain
	e1, r1   trusttest.Key
	mutex    sync.Mutex
	document addressdoc.Document
	files    map[string]trust.Signed
	// documentFetches is how many times the core asked for its address document.
	documentFetches int
	// publisher is the publisher's root chain, signed by its everyday key publisherKey.
	publisher    *trusttest.Chain
	publisherKey trusttest.Key
	// Guarded by mutex:
	nonces       map[string]bool
	publicKey    ed25519.PublicKey
	state        discoveryproto.State
	tokens       map[string]bool
	heartbeats   []discoveryproto.AddressesRequest
	deprecated   string
	isLeft       bool
	sessionCount int
	// sessionLasts is how many seconds a session lasts, as the discovery says: a day unless set.
	sessionLasts int64
	// instanceIDs are the instances that opened sessions, in turn; refusal, when set, refuses every
	// session with its code, until the core enrols again; enrolCount is how many times it enrolled.
	instanceIDs []string
	// installationIDs are the installations that asked for sessions, in turn (D437 §5).
	installationIDs []string
	refusal         *discoveryproto.Refusal
	enrolCount      int
	// pauses are the routes whose next call is held until the test releases it (pause).
	pauses map[string]paused
	// txt are the certificate challenges' records the core had the discovery set, by name, and
	// accounts the ACME accounts it reported, in turn.
	txt      map[string]string
	accounts []string
	// signals are the offers the test has the discovery pass on, and replies the core's answers.
	signals chan discoveryproto.SignalMessage
	replies chan discoveryproto.SignalMessage
	// failing are the routes ("GET /status") it answers with a 500; dropsSignalling closes each
	// signalling connection as soon as it opens; onSignal is told of each dial of it. Unless
	// answersPings, a signalling connection reads nothing while no offer is passed on, so a ping on
	// it goes unanswered, as on a connection a proxy dropped.
	failing         map[string]bool
	dropsSignalling bool
	answersPings    bool
	// passesAtOnce, offers are passed on as they come, each reply taken as it comes, as the real
	// discovery passes a page's offers (discovery/single); without it, each waits for the last's reply.
	passesAtOnce bool
	onSignal     func()
	// silentDials is how many of the next signalling connections are held open reading nothing,
	// answering neither ping nor offer: a discovery restarted behind its tunnel, which left the core's
	// link open (staging, 2026-10-29). Those after them are served as the fields above say.
	silentDials int
	// locateHashes and locators are each set of links and locators the core sent, in turn.
	locateHashes []discoveryproto.LocateHashesRequest
	locators     []discoveryproto.LocatorsRequest
	// deviceCodes are the devices waiting with a code the discovery issued, by code, and
	// deviceOutcomes how the core said each code it claimed ended (its sealed outcome, or "" refused).
	deviceCodes map[string]discoveryproto.DeviceCodeClaimResponse
	// coreNonces are the nonces the core claimed each code with, by code (D321).
	coreNonces     map[string]string
	deviceOutcomes map[string]string
	// claimsRefused refuses every claim as one too many.
	claimsRefused bool
	// realm is the realm the discovery gave the core, which its status names.
	realm string
	// account is the account a key claimed the core for, which its status names; removedAt when the
	// core was removed from an account, which a pending core's status says.
	account   string
	removedAt int64
	// release is the release record it serves, signed by recordSigner, the publisher's key unless a
	// test names another; nil serves none. releaseFetches is how many times the core read it.
	release        *release.Record
	recordSigner   *trusttest.Key
	releaseFetches int
}

// fakeKey is the one key the fake discovery takes.
const fakeKey = "abcd-efgh-jkmn-pqrs"

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
		deviceCodes: map[string]discoveryproto.DeviceCodeClaimResponse{}, deviceOutcomes: map[string]string{},
		coreNonces: map[string]string{},
		signals:    make(chan discoveryproto.SignalMessage), replies: make(chan discoveryproto.SignalMessage, 4),
		sessionLasts: int64((24 * time.Hour) / time.Second), failing: map[string]bool{},
	}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	fake.issuer, fake.files = server.URL, map[string]trust.Signed{}
	fake.chain, fake.e1, fake.r1 = trusttest.NewDiscovery(t)
	fake.publisherKey = trusttest.HardwareKey(t, "p1")
	fake.publisher = trusttest.NewChain(t, trust.ChainPublisher, []trusttest.Key{fake.publisherKey},
		[]trusttest.Key{trusttest.HardwareKey(t, "pr1")})
	fake.document = addressdoc.Document{
		ID: fake.chain.ID(), RootVersion: 1, Sequence: 1, Issued: trusttest.Issued(),
		Expires: trusttest.Issued().Add(365 * 24 * time.Hour), Name: "Fake", Issuer: server.URL,
		Locations: []string{server.URL}, CoreAPI: server.URL, ClientAPI: server.URL + "/client",
		LoaderOrigin: fakeLoaderOrigin, ClientOrigins: fakeClientOrigins,
	}
	return fake, server
}

// The origins the fake discovery's document names: a loader, and each core's own client origin.
const (
	fakeLoaderOrigin  = "http://localhost:8443"
	fakeClientOrigins = "http://" + addressdoc.CorePlace + ".localhost:8443"
)

// servedWellKnown is whether request was for a signed file or its signature, which it served: at
// its issuer, and at any location under it, as a mirror serves it.
func (fake *fakeDiscovery) servedWellKnown(writer http.ResponseWriter, request *http.Request) bool {
	index := strings.Index(request.URL.Path, "/.well-known/")
	if request.Method != http.MethodGet || index < 0 {
		return false
	}
	name, isSignature := strings.CutSuffix(request.URL.Path[index:], addressdoc.SignatureEnd)
	if name == release.RecordPath && fake.failing["GET "+release.RecordPath] {
		http.Error(writer, "the discovery failed", http.StatusInternalServerError)
		return true
	}
	signed, isFound := fake.signedFile(name, !isSignature)
	if !isFound {
		http.NotFound(writer, request)
		return true
	}
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	body := signed.Payload
	if isSignature {
		body = signed.Signature
	}
	_, _ = writer.Write([]byte(body)) //nolint:errcheck // the core reads what arrived
	return true
}

// signedFile is the signed file at name, and whether there is one; isCounted counts a fetch of the
// document's or the record's payload.
func (fake *fakeDiscovery) signedFile(name string, isCounted bool) (trust.Signed, bool) {
	if signed, isSet := fake.files[name]; isSet {
		if isCounted && name == addressdoc.DocumentPath {
			fake.documentFetches++
		}
		return signed, true
	}
	switch {
	case name == addressdoc.DocumentPath:
		if isCounted {
			fake.documentFetches++
		}
		return fake.signedDocument(fake.document), true
	case name == release.RecordPath:
		if fake.release == nil {
			return trust.Signed{}, false
		}
		if isCounted {
			fake.releaseFetches++
		}
		signer := fake.publisherKey
		if fake.recordSigner != nil {
			signer = *fake.recordSigner
		}
		return signRecord(fake.t, *fake.release, fake.publisher.ID(), signer), true
	case strings.HasPrefix(name, addressdoc.RootPathPrefix):
		return statementAt(fake.chain, strings.TrimPrefix(name, addressdoc.RootPathPrefix))
	case strings.HasPrefix(name, release.PublisherRootPathPrefix):
		return statementAt(fake.publisher, strings.TrimPrefix(name, release.PublisherRootPathPrefix))
	}
	return trust.Signed{}, false
}

// statementAt is chain's statement of version, "latest" the last.
func statementAt(chain *trusttest.Chain, version string) (trust.Signed, bool) {
	if version == "latest" {
		return chain.Statements[len(chain.Statements)-1], true
	}
	for _, statement := range chain.Statements {
		if root, err := trust.ParseRoot([]byte(statement.Payload)); err == nil && fmt.Sprint(root.Version) == version {
			return statement, true
		}
	}
	return trust.Signed{}, false
}

// signedDocument is document signed by the fake's everyday key.
func (fake *fakeDiscovery) signedDocument(document addressdoc.Document) trust.Signed {
	return signDocument(fake.t, document, fake.e1)
}

// signDocument is document signed by key in the address namespace.
func signDocument(t *testing.T, document addressdoc.Document, key trusttest.Key) trust.Signed {
	t.Helper()
	payload, err := document.Payload()
	if err != nil {
		t.Fatal(err)
	}
	return trusttest.Sign(t, trust.NamespaceDiscoveryAddress, payload, key)
}

// signRecord is record, under the publisher id, signed by key in the release namespace.
func signRecord(t *testing.T, record release.Record, id string, key trusttest.Key) trust.Signed {
	t.Helper()
	record.ID = id
	payload, err := record.Payload()
	if err != nil {
		t.Fatal(err)
	}
	return trusttest.Sign(t, trust.NamespaceRelease, payload, key)
}

// paused holds the next call to a route until released, before the discovery takes it: a call in
// flight while the test does something else.
type paused struct {
	arrived  chan struct{}
	released chan struct{}
	// release lets the call go on; the test's end does too, so a test that fails first does not
	// leave the call holding the fake's server open.
	release func()
}

// pause holds the next call to route ("POST /enrol") until the returned pause is released.
func (fake *fakeDiscovery) pause(t *testing.T, route string) paused {
	t.Helper()
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	released := make(chan struct{})
	held := paused{
		arrived: make(chan struct{}), released: released, release: sync.OnceFunc(func() { close(released) }),
	}
	t.Cleanup(held.release)
	if fake.pauses == nil {
		fake.pauses = map[string]paused{}
	}
	fake.pauses[route] = held
	return held
}

func (fake *fakeDiscovery) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	fake.mutex.Lock()
	held, isPaused := fake.pauses[request.Method+" "+request.URL.Path]
	delete(fake.pauses, request.Method+" "+request.URL.Path)
	fake.mutex.Unlock()
	if isPaused {
		close(held.arrived)
		select {
		case <-held.released:
		case <-request.Context().Done():
			return
		}
	}
	fake.serveHTTP(writer, request)
}

func (fake *fakeDiscovery) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == discoveryproto.PathSignal {
		fake.serveSignal(writer, request)
		return
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if fake.servedWellKnown(writer, request) {
		return
	}
	if request.Header.Get(discoveryproto.ProtocolHeader) != discoveryproto.ProtocolVersion {
		http.Error(writer, "say the protocol", http.StatusBadRequest)
		return
	}
	if fake.deprecated != "" {
		writer.Header().Set(discoveryproto.DeprecatedHeader, fake.deprecated)
	}
	route := request.Method + " " + request.URL.Path
	if fake.failing[route] {
		http.Error(writer, "the discovery failed", http.StatusInternalServerError)
		return
	}
	switch route {
	case "POST /challenge":
		nonce := rand.Text()
		fake.nonces[nonce] = true
		fake.answer(writer, discoveryproto.ChallengeResponse{Nonce: nonce})
	case "POST /enrol":
		var enrol discoveryproto.EnrolRequest
		fake.read(request, &enrol)
		if !fake.signedFresh(enrol.PublicKey, discoveryproto.PurposeEnrol, proof.KeyLabel(enrol.PublicKey),
			enrol.Nonce, enrol.Signature) {
			http.Error(writer, "bad signature", http.StatusForbidden)
			return
		}
		fake.publicKey, fake.state, fake.refusal = enrol.PublicKey, discoveryproto.StatePending, nil
		fake.enrolCount++
		fake.answer(writer, discoveryproto.EnrolResponse{CoreID: fakeCoreID, ClaimCode: "abcd-efgh", ClaimExpiresAt: 1})
	case "POST /session":
		var opened discoveryproto.SessionRequest
		fake.read(request, &opened)
		if opened.CoreID != fakeCoreID ||
			!fake.signedFresh(fake.publicKey, discoveryproto.PurposeSession, fakeCoreID, opened.Nonce,
				opened.Signature) {
			http.Error(writer, "bad signature", http.StatusForbidden)
			return
		}
		fake.instanceIDs = append(fake.instanceIDs, opened.InstanceID)
		fake.installationIDs = append(fake.installationIDs, opened.InstallationID)
		if fake.refusal != nil {
			writer.WriteHeader(http.StatusConflict)
			fake.answer(writer, fake.refusal)
			return
		}
		fake.sessionCount++
		token := rand.Text()
		fake.tokens[token] = true
		fake.answer(writer, discoveryproto.SessionResponse{Token: token, ExpiresInSeconds: fake.sessionLasts})
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
			Realm:   fake.realm,
			Account: fake.account, RemovedAt: fake.removedAt,
		})
	case "POST /claim":
		fake.claimByKey(writer, request)
	case "PUT /addresses":
		var heartbeat discoveryproto.AddressesRequest
		fake.read(request, &heartbeat)
		fake.heartbeats = append(fake.heartbeats, heartbeat)
		fake.answer(writer, discoveryproto.AddressesResponse{
			Published: []string{"2a0f-5000-0-0-0-0-0-1." + fakeCoreID + ".orpheum-core.example"},
			Probe: []discoveryproto.ProbeResult{{
				Address: "2a0f:5000::1", Port: 443,
				Kind: discoveryproto.ProbeHTTPS, Result: discoveryproto.ProbeReached,
			}},
			ReachableFrom: discoveryproto.ReachableIPv6, DeadEndsLastWeek: 4,
			DeadEndCausesLastWeek: map[string]int{discoveryproto.DeadEndCauseNoIPv6: 3, "none": 1},
		})
	case "DELETE /core":
		// The discovery no longer holds the core: a session it asks for is refused as one to enrol again.
		fake.isLeft = true
		fake.refusal = &discoveryproto.Refusal{Code: discoveryproto.RefusalEnrolAgain, Message: "enrol again"}
		fake.tokens = map[string]bool{}
	case "PUT /locate-hashes":
		var sent discoveryproto.LocateHashesRequest
		fake.read(request, &sent)
		fake.locateHashes = append(fake.locateHashes, sent)
	case "PUT /locators":
		var sent discoveryproto.LocatorsRequest
		fake.read(request, &sent)
		fake.locators = append(fake.locators, sent)
	default:
		fake.serveMore(writer, request, route)
	}
}

// serveMore answers the rest of serveSigned's calls: the device codes and the certificates'.
func (fake *fakeDiscovery) serveMore(writer http.ResponseWriter, request *http.Request, route string) {
	switch route {
	case "POST /device-codes/claim":
		var claim discoveryproto.DeviceCodeClaimRequest
		fake.read(request, &claim)
		fake.coreNonces[claim.Code] = claim.CoreNonce
		waiting, isWaiting := fake.deviceCodes[claim.Code]
		if fake.claimsRefused {
			writer.WriteHeader(http.StatusTooManyRequests)
			fake.answer(writer, discoveryproto.Refusal{Code: discoveryproto.RefusalTooManyClaims, Message: "slow"})
			return
		}
		if !isWaiting {
			writer.WriteHeader(http.StatusNotFound)
			fake.answer(writer, discoveryproto.Refusal{Code: discoveryproto.RefusalNotFound, Message: "no such code"})
			return
		}
		fake.answer(writer, waiting)
	case "POST /device-codes/outcome":
		var outcome discoveryproto.DeviceCodeOutcomeRequest
		fake.read(request, &outcome)
		fake.deviceOutcomes[outcome.Code] = outcome.Sealed
	case "PUT /acme-account":
		var reported discoveryproto.ACMEAccountRequest
		fake.read(request, &reported)
		fake.accounts = append(fake.accounts, reported.AccountURI)
	case "PUT /acme-challenge":
		var challenge discoveryproto.ACMEChallengeRequest
		fake.read(request, &challenge)
		if challenge.Name != "_acme-challenge."+fakeCoreID+"."+fakeZone {
			http.Error(writer, "not this core's name", http.StatusForbidden)
			return
		}
		fake.txt[challenge.Name] = challenge.Value
	default:
		// The record is cleared by its name in the path, with no body (the protocol's §2.7).
		cleared, isClearing := strings.CutPrefix(request.URL.Path, discoveryproto.PathACMEChallenge+"/")
		if request.Method != http.MethodDelete || !isClearing || request.ContentLength > 0 {
			http.NotFound(writer, request)
			return
		}
		delete(fake.txt, cleared)
	}
}

// signedFresh reports whether signature is publicKey's over a nonce the discovery issued and no call
// has used, for purpose, bound to this discovery and core.
func (fake *fakeDiscovery) signedFresh(
	publicKey ed25519.PublicKey, purpose, core, nonce string, signature []byte,
) bool {
	if !fake.nonces[nonce] || len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	delete(fake.nonces, nonce)
	return proof.Verify(publicKey, purpose, fake.chain.ID(), core, nonce, signature)
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

// signalMode is how the fake serves one signalling connection, as its fields said at the dial.
type signalMode struct {
	drops, answersPings, atOnce, isSilent bool
}

// takeSignalMode is how the fake serves the connection being dialled, a silent dial counted off.
func (fake *fakeDiscovery) takeSignalMode() signalMode {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	mode := signalMode{
		drops: fake.dropsSignalling, answersPings: fake.answersPings, atOnce: fake.passesAtOnce,
		isSilent: fake.silentDials > 0,
	}
	if mode.isSilent {
		fake.silentDials--
	}
	return mode
}

// serveSignal is the signalling connection: the offers the test sends go to the core, and its
// answers come back to the test.
func (fake *fakeDiscovery) serveSignal(writer http.ResponseWriter, request *http.Request) {
	fake.mutex.Lock()
	isSignedIn := fake.tokens[strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")]
	onSignal := fake.onSignal
	fake.mutex.Unlock()
	mode := fake.takeSignalMode()
	if onSignal != nil {
		onSignal()
	}
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
	switch {
	case mode.drops:
		return
	case mode.isSilent:
		<-ctx.Done()
		return
	case mode.answersPings:
		// Reads only control frames, answering each ping, until the connection or the request ends.
		reading := connection.CloseRead(ctx)
		select {
		case <-reading.Done():
		case <-ctx.Done():
		}
		return
	}
	fake.passOffers(ctx, connection, mode.atOnce)
}

// passOffers passes on each offer the test sends until the connection ends: as they come when
// atOnce, else each after the last's reply.
func (fake *fakeDiscovery) passOffers(ctx context.Context, connection *websocket.Conn, atOnce bool) {
	if atOnce {
		go fake.takeReplies(ctx, connection)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case offer := <-fake.signals:
			if err := wsjson.Write(ctx, connection, offer); err != nil {
				return
			}
			if atOnce {
				continue
			}
			var reply discoveryproto.SignalMessage
			if err := wsjson.Read(ctx, connection, &reply); err != nil {
				return
			}
			fake.replies <- reply
		}
	}
}

// takeReplies hands on each reply the core sends, as it comes, until the connection ends.
func (fake *fakeDiscovery) takeReplies(ctx context.Context, connection *websocket.Conn) {
	for {
		var reply discoveryproto.SignalMessage
		if err := wsjson.Read(ctx, connection, &reply); err != nil {
			return
		}
		fake.replies <- reply
	}
}

// claimByKey takes fakeKey, claiming the core for karl's account, and refuses any other key.
func (fake *fakeDiscovery) claimByKey(writer http.ResponseWriter, request *http.Request) {
	var asked discoveryproto.ClaimRequest
	fake.read(request, &asked)
	if asked.Key != fakeKey {
		writer.WriteHeader(http.StatusForbidden)
		fake.answer(writer, discoveryproto.Refusal{Code: discoveryproto.RefusalWrongKey, Message: "no such key"})
		return
	}
	fake.state, fake.account, fake.removedAt = discoveryproto.StateClaimed, "karl@example.com", 0
	fake.answer(writer, discoveryproto.ClaimResponse{Account: fake.account})
}
