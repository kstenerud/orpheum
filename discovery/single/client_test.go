// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/link"
)

// clientStage is a stage whose claimed core registered a link of unlimited uses, one of one use, and
// a device's locator; client is the client API's server.
type clientStage struct {
	*stage
	client *httptest.Server
	coreID string
	token  string
	signer key
}

const (
	everyUseSecret = "a-registration-links-secret"
	oneUseSecret   = "a-watch-together-links-secret"
	aLocator       = "a-devices-locator"
)

func newClientStage(t *testing.T) clientStage {
	t.Helper()
	built := newStage(t)
	signer, coreID := built.claimed(t)
	token := built.session(t, signer, coreID)
	if status := built.call(t, http.MethodPut, discoveryproto.PathLocateHashes, token,
		discoveryproto.LocateHashesRequest{Hashes: []discoveryproto.LocateHash{
			{Hash: link.LocateHash(everyUseSecret), ExpiresIn: 3600},
			{Hash: link.LocateHash(oneUseSecret), ExpiresIn: 3600, Uses: 1},
		}}, nil); status != http.StatusNoContent {
		t.Fatalf("registering the links answered %d", status)
	}
	locators := discoveryproto.LocatorsRequest{Hashes: []string{link.LocatorHash(aLocator)}}
	status := built.call(t, http.MethodPut, discoveryproto.PathLocators, token, locators, nil)
	if status != http.StatusNoContent {
		t.Fatalf("registering the locator answered %d", status)
	}
	client := httptest.NewServer(built.discovery.ClientHandler())
	t.Cleanup(client.Close)
	return clientStage{stage: built, client: client, coreID: coreID, token: token, signer: signer}
}

// ask posts body to the client API's path and decodes a 200's answer into into; it returns the status
// and a refusal's code.
func (built clientStage) ask(t *testing.T, path string, body, into any) (int, string) {
	t.Helper()
	var sent bytes.Buffer
	if err := json.NewEncoder(&sent).Encode(body); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, built.client.URL+path, &sent)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(discoveryproto.ProtocolHeader, discoveryproto.ProtocolVersion)
	response, err := built.client.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // read in full below
	if response.StatusCode == http.StatusOK {
		if err = json.NewDecoder(response.Body).Decode(into); err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, ""
	}
	var refusal discoveryproto.Refusal
	_ = json.NewDecoder(response.Body).Decode(&refusal) //nolint:errcheck // a refusal's body, or none
	return response.StatusCode, refusal.Code
}

func TestLocate_ALinksHashFindsTheCoresPathsAndABootstrapTokenAndNothingElseDoes(t *testing.T) {
	t.Parallel()
	built := newClientStage(t)
	var located discoveryproto.LocateResponse
	if status, code := built.ask(t, discoveryproto.PathLocate,
		discoveryproto.LocateRequest{Hash: link.LocateHash(everyUseSecret)}, &located); status != http.StatusOK {
		t.Fatalf("locating by the link's hash answered %d %q", status, code)
	}
	want := []discoveryproto.Path{{Kind: discoveryproto.PathKindWebRTC, Offer: discoveryproto.OfferPath(built.coreID)}}
	if located.CoreID != built.coreID || located.BootstrapToken == "" || len(located.Paths) != 1 ||
		located.Paths[0] != want[0] || located.ClientPrefix != "127.0.0.1/32" || len(located.ICEServers) == 0 {
		t.Errorf("located %+v; want the core, a bootstrap token, its WebRTC path, the client's /32 and the "+
			"ICE servers", located)
	}
	for _, hash := range []string{link.LocateHash("a secret it never registered"), everyUseSecret} {
		if status, code := built.ask(t, discoveryproto.PathLocate, discoveryproto.LocateRequest{Hash: hash},
			&located); status != http.StatusNotFound || code != discoveryproto.RefusalNotFound {
			t.Errorf("locating by %q answered %d %q; want not_found", hash, status, code)
		}
	}
}

func TestLocate_ALinkOfOneUseLocatesOnceAndAnyLinkNoLongerOnceItEnds(t *testing.T) {
	t.Parallel()
	built := newClientStage(t)
	var located discoveryproto.LocateResponse
	asked := discoveryproto.LocateRequest{Hash: link.LocateHash(oneUseSecret)}
	if status, _ := built.ask(t, discoveryproto.PathLocate, asked, &located); status != http.StatusOK {
		t.Fatalf("the first use answered %d", status)
	}
	if status, code := built.ask(t, discoveryproto.PathLocate, asked, &located); status != http.StatusNotFound {
		t.Errorf("a second use of a one-use link answered %d %q; want not_found", status, code)
	}
	built.advance(time.Hour)
	every := discoveryproto.LocateRequest{Hash: link.LocateHash(everyUseSecret)}
	if status, code := built.ask(t, discoveryproto.PathLocate, every, &located); status != http.StatusNotFound {
		t.Errorf("a link past its life answered %d %q; want not_found", status, code)
	}
}

func TestPaths_ALocatorOrALiveBootstrapTokenFindsTheCoreAndSaysHowItStands(t *testing.T) {
	t.Parallel()
	built := newClientStage(t)
	var located discoveryproto.LocateResponse
	if status, _ := built.ask(t, discoveryproto.PathLocate,
		discoveryproto.LocateRequest{Hash: link.LocateHash(everyUseSecret)}, &located); status != http.StatusOK {
		t.Fatal("locate failed")
	}
	var paths discoveryproto.PathsResponse
	for name, asked := range map[string]discoveryproto.PathsRequest{
		"the locator": {Locator: aLocator}, "the bootstrap token": {BootstrapToken: located.BootstrapToken},
	} {
		if status, code := built.ask(t, discoveryproto.PathPaths, asked, &paths); status != http.StatusOK ||
			paths.CoreID != built.coreID || paths.State != discoveryproto.CoreOffline {
			t.Errorf("paths by %s = %d %q %+v; want the core's, offline: it holds no signalling connection",
				name, status, code, paths)
		}
	}
	built.dial(t, built.token)
	built.logs.AwaitLine(t, "the core's signalling connected")
	if status, _ := built.ask(t, discoveryproto.PathPaths, discoveryproto.PathsRequest{Locator: aLocator},
		&paths); status != http.StatusOK || paths.State != discoveryproto.CoreOnline || paths.OfflineSince != 0 {
		t.Errorf("with its signalling connected the core is %q since %d; want online", paths.State, paths.OfflineSince)
	}
	built.advance(bootstrapLasts)
	for name, asked := range map[string]discoveryproto.PathsRequest{
		"an ended bootstrap token":   {BootstrapToken: located.BootstrapToken},
		"a locator never registered": {Locator: "another"}, "nothing": {},
	} {
		if status, code := built.ask(t, discoveryproto.PathPaths, asked, &paths); status != http.StatusNotFound {
			t.Errorf("paths by %s answered %d %q; want not_found", name, status, code)
		}
	}
}

func TestPrefixOf_IsAnIPv6Addresses64AndAnIPv4Addresses32(t *testing.T) {
	t.Parallel()
	for remote, want := range map[string]string{
		"[2a0f:3000:1:2:3:4:5:6]:51234": "2a0f:3000:1:2::/64", "203.0.113.7:443": "203.0.113.7/32",
		"[::ffff:203.0.113.7]:443": "203.0.113.7/32", "not an address": "",
	} {
		if got := prefixOf(remote); got != want {
			t.Errorf("prefixOf(%q) = %q, want %q", remote, got, want)
		}
	}
}

// answerAll is a core holding the signalling connection, answering every offer with what it was
// passed: the offer and the hash it came under.
func answerAll(t *testing.T, built clientStage) {
	t.Helper()
	connection := built.dial(t, built.token)
	go func() {
		for {
			var offer discoveryproto.SignalMessage
			if wsjson.Read(t.Context(), connection, &offer) != nil {
				return
			}
			reply := discoveryproto.SignalMessage{
				Type: discoveryproto.SignalAnswer, OfferID: offer.OfferID,
				SDP: offer.SDP + " under " + offer.LocatorHash, Signature: []byte(offer.Proof),
			}
			if wsjson.Write(t.Context(), connection, reply) != nil {
				return
			}
		}
	}()
	built.logs.AwaitLine(t, "the core's signalling connected")
}

func TestOffers_ReachTheCoreWithTheHashTheDeviceFoundItByAndComeBackInOrder(t *testing.T) {
	t.Parallel()
	built := newClientStage(t)
	answerAll(t, built)
	var answered discoveryproto.OfferResponse
	asked := discoveryproto.OfferRequest{
		PathsRequest: discoveryproto.PathsRequest{Locator: aLocator},
		Offers:       []discoveryproto.ClientOffer{{SDP: "first", Proof: "p1"}, {SDP: "second", Proof: "p2"}},
	}
	if status, code := built.ask(t, discoveryproto.OfferPath(built.coreID), asked, &answered); status != http.StatusOK {
		t.Fatalf("the offers answered %d %q", status, code)
	}
	hash := link.LocatorHash(aLocator)
	if len(answered.Answers) != 2 || answered.Answers[0].SDP != "first under "+hash ||
		answered.Answers[1].SDP != "second under "+hash || string(answered.Answers[1].Signature) != "p2" {
		t.Errorf("the answers are %+v; want each offer's, in order, passed under the locator's hash with "+
			"its proof", answered.Answers)
	}
	var located discoveryproto.LocateResponse
	if status, _ := built.ask(t, discoveryproto.PathLocate,
		discoveryproto.LocateRequest{Hash: link.LocateHash(everyUseSecret)}, &located); status != http.StatusOK {
		t.Fatal("locate failed")
	}
	asked.PathsRequest = discoveryproto.PathsRequest{BootstrapToken: located.BootstrapToken}
	asked.Offers = asked.Offers[:1]
	if status, _ := built.ask(t, discoveryproto.OfferPath(built.coreID), asked, &answered); status != http.StatusOK ||
		answered.Answers[0].SDP != "first under "+link.LocateHash(everyUseSecret) {
		t.Errorf("an offer by a bootstrap token was answered %+v; want it passed under its link's hash",
			answered.Answers)
	}
}

func TestOffers_ARefusedRequestNeverReachesTheCore(t *testing.T) {
	t.Parallel()
	built := newClientStage(t)
	answerAll(t, built)
	five := make([]discoveryproto.ClientOffer, discoveryproto.MaxOffersAtOnce+1)
	for name, tc := range map[string]struct {
		path   string
		asked  discoveryproto.OfferRequest
		status int
	}{
		"a locator never registered": {discoveryproto.OfferPath(built.coreID), discoveryproto.OfferRequest{
			PathsRequest: discoveryproto.PathsRequest{Locator: "another"}, Offers: five[:1],
		}, http.StatusNotFound},
		"another core's path": {discoveryproto.OfferPath("another-core"), discoveryproto.OfferRequest{
			PathsRequest: discoveryproto.PathsRequest{Locator: aLocator}, Offers: five[:1],
		}, http.StatusNotFound},
		"more offers than a client makes at once": {discoveryproto.OfferPath(built.coreID), discoveryproto.OfferRequest{
			PathsRequest: discoveryproto.PathsRequest{Locator: aLocator}, Offers: five,
		}, http.StatusBadRequest},
	} {
		var answered discoveryproto.OfferResponse
		if status, _ := built.ask(t, tc.path, tc.asked, &answered); status != tc.status {
			t.Errorf("%s answered %d; want %d", name, status, tc.status)
		}
	}
}

func TestOpenOffers_AHashHoldsAtMostEightOpenAtOnce(t *testing.T) {
	t.Parallel()
	built := newStage(t)
	for range 2 {
		if !built.discovery.openOffers("a-hash", 4) {
			t.Fatal("eight offers open for one hash were refused")
		}
	}
	if built.discovery.openOffers("a-hash", 1) {
		t.Error("a ninth open offer for one hash was let through")
	}
	if !built.discovery.openOffers("another-hash", 1) {
		t.Error("another hash's offer was refused for the first hash's")
	}
	built.discovery.closeOffers("a-hash", 4)
	if !built.discovery.openOffers("a-hash", 1) {
		t.Error("once four closed, an offer for the hash was still refused")
	}
}

func TestDeadEnd_IsCountedForTheCoresWeekAndToldInTheHeartbeatsAnswer(t *testing.T) {
	t.Parallel()
	built := newClientStage(t)
	var nothing struct{}
	for range 2 {
		if status, code := built.ask(t, discoveryproto.PathDeadEnd, discoveryproto.DeadEndRequest{Locator: aLocator},
			&nothing); status != http.StatusNoContent {
			t.Fatalf("a dead end answered %d %q", status, code)
		}
	}
	if status, _ := built.ask(t, discoveryproto.PathDeadEnd, discoveryproto.DeadEndRequest{Locator: "another"},
		&nothing); status != http.StatusNotFound {
		t.Errorf("a dead end by a locator never registered answered %d; want not_found, uncounted", status)
	}
	heartbeat := func() int {
		t.Helper()
		var answered discoveryproto.AddressesResponse
		token := built.session(t, built.signer, built.coreID) // a week on, the first has ended
		if status := built.call(t, http.MethodPut, discoveryproto.PathAddresses, token,
			discoveryproto.AddressesRequest{}, &answered); status != http.StatusOK {
			t.Fatalf("the heartbeat answered %d", status)
		}
		return answered.DeadEndsLastWeek
	}
	if count := heartbeat(); count != 2 {
		t.Errorf("the heartbeat's answer counts %d dead ends; want 2", count)
	}
	built.advance(deadEndWindow)
	if count := heartbeat(); count != 0 {
		t.Errorf("a week on, the heartbeat's answer counts %d dead ends; want none", count)
	}
}
