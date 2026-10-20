// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
)

// fakeDiscovery speaks the discovery's side of the protocol, checking every signature, and keeps
// what the core sent.
type fakeDiscovery struct {
	t *testing.T
	// baseURL is where the core calls it, which the core's signatures are bound to; clientOrigin is
	// where it says it serves its client.
	baseURL, clientOrigin, loaderOrigin string
	mutex                               sync.Mutex
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
	refusal     *discoveryproto.Refusal
	enrolCount  int
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
	onSignal        func()
	// locateHashes and locators are each set of links and locators the core sent, in turn.
	locateHashes []discoveryproto.LocateHashesRequest
	locators     []discoveryproto.LocatorsRequest
	// deviceCodes are the devices waiting with a code the discovery issued, by code, and
	// deviceOutcomes how the core said each code it claimed ended (its sealed outcome, or "" refused).
	deviceCodes    map[string]discoveryproto.DeviceCodeClaimResponse
	deviceOutcomes map[string]string
	// claimsRefused refuses every claim as one too many.
	claimsRefused bool
	// realm is the realm the discovery gave the core, which its status names.
	realm string
	// account is the account a key claimed the core for, which its status names; removedAt when the
	// core was removed from an account, which a pending core's status says.
	account   string
	removedAt int64
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
		signals: make(chan discoveryproto.SignalMessage), replies: make(chan discoveryproto.SignalMessage, 4),
		sessionLasts: int64((24 * time.Hour) / time.Second), failing: map[string]bool{},
	}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	fake.baseURL = server.URL
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
		if fake.refusal != nil {
			writer.WriteHeader(http.StatusConflict)
			fake.answer(writer, fake.refusal)
			return
		}
		fake.sessionCount++
		token := rand.Text()
		fake.tokens[token] = true
		fake.answer(writer, discoveryproto.SessionResponse{Token: token, ExpiresIn: fake.sessionLasts})
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
			ClientOrigin: fake.clientOrigin, LoaderOrigin: fake.loaderOrigin, Realm: fake.realm,
			Account: fake.account, RemovedAt: fake.removedAt,
		})
	case "POST /claim":
		fake.claimByKey(writer, request)
	case "PUT /addresses":
		var heartbeat discoveryproto.AddressesRequest
		fake.read(request, &heartbeat)
		fake.heartbeats = append(fake.heartbeats, heartbeat)
		fake.answer(writer, discoveryproto.AddressesResponse{
			Published: []string{"2a02-8070-0-0-0-0-0-1." + fakeCoreID + ".orpheum-core.example"},
			Probe: []discoveryproto.ProbeResult{{
				Address: "2a0f:1000::1", Port: 443,
				Kind: discoveryproto.ProbeHTTPS, Result: discoveryproto.ProbeReached,
			}},
			ReachableFrom: discoveryproto.ReachableIPv6, DeadEndsLastWeek: 4,
		})
	case "DELETE /core":
		fake.isLeft = true
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
	return proof.Verify(publicKey, purpose, fake.baseURL, core, nonce, signature)
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

// serveSignal is the signalling connection: the offers the test sends go to the core, and its
// answers come back to the test.
func (fake *fakeDiscovery) serveSignal(writer http.ResponseWriter, request *http.Request) {
	fake.mutex.Lock()
	isSignedIn := fake.tokens[strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")]
	drops, onSignal, answersPings := fake.dropsSignalling, fake.onSignal, fake.answersPings
	fake.mutex.Unlock()
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
	if drops {
		return
	}
	ctx := request.Context()
	if answersPings {
		// Reads only control frames, answering each ping, until the connection or the request ends.
		reading := connection.CloseRead(ctx)
		select {
		case <-reading.Done():
		case <-ctx.Done():
		}
		return
	}
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
