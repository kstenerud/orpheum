// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
)

// realmKey is the key karl's password makes in the realm karlnas, made once: it costs RealmRounds.
var realmKey = proof.RealmKey("karlnas", "karl", "a password")

// slowRefusal is how long a slow core takes to refuse a realm sign-in: past the second the answer once
// waited, inside the time it waits now.
const slowRefusal = 3 * time.Second

// answerRealms is a core holding the signalling connection that takes a realm sign-in whose ask holds
// under karl's key, answering its tag, and refuses any other, a slow person's after slowRefusal.
func answerRealms(t *testing.T, built clientStage) {
	t.Helper()
	connection := built.dial(t, built.token)
	go func() {
		for {
			var asked discoveryproto.SignalMessage
			if wsjson.Read(t.Context(), connection, &asked) != nil {
				return
			}
			reply := discoveryproto.SignalMessage{
				Type: discoveryproto.SignalRefusal, OfferID: asked.OfferID, Reason: "the ask does not hold",
			}
			if asked.Type == discoveryproto.SignalRealm && asked.Person == proof.RealmPerson("karlnas", "slow") {
				built.advance(slowRefusal) // a core that takes its time to refuse
			}
			if asked.Type == discoveryproto.SignalRealm && asked.Person == proof.RealmPerson("karlnas", "karl") &&
				proof.VerifyRealmAsk(realmKey, "karlnas", asked.Person, asked.Ask) {
				reply = discoveryproto.SignalMessage{
					Type: discoveryproto.SignalRealm, OfferID: asked.OfferID, PublicKey: []byte("the core's key"),
					Tag: "the core's tag",
				}
			}
			if asked.Type == discoveryproto.SignalOffer {
				reply = discoveryproto.SignalMessage{
					Type: discoveryproto.SignalAnswer, OfferID: asked.OfferID,
					SDP: "answered under " + asked.LocatorHash,
				}
			}
			if wsjson.Write(t.Context(), connection, reply) != nil {
				return
			}
		}
	}()
	built.logs.AwaitLine(t, "the core's signalling connected")
}

// karlsSignIn is karl's realm sign-in at karlnas, right in every part.
func karlsSignIn() discoveryproto.ClientRealmRequest {
	person := proof.RealmPerson("karlnas", "karl")
	return discoveryproto.ClientRealmRequest{
		Realm: "KarlNAS", Person: person, Ask: proof.RealmAsk(realmKey, "karlnas", person),
	}
}

// The realm the discovery's operator named is the claimed core's, as its status says, and a person
// who signs in by it right, in any case, gets what a link's locate gives, the core's key and its tag;
// their offers go to the core under who they are, so the core proves them by their key (D297 §1, §3).
func TestRealmSignIn_ARightOneIsTheCoresPathsKeyAndTagAndItsOffersGoUnderThePerson(t *testing.T) {
	t.Parallel()
	built := newClientStageWithRealm(t, "karlnas")
	var status discoveryproto.StatusResponse
	built.call(t, http.MethodGet, discoveryproto.PathStatus, built.token, nil, &status)
	if status.Realm != "karlnas" {
		t.Errorf("the status names the realm %q; want karlnas, the operator's", status.Realm)
	}
	answerRealms(t, built)
	var answered discoveryproto.ClientRealmResponse
	if code, refusal := built.ask(t, discoveryproto.PathRealmSignIn, karlsSignIn(), &answered); code != http.StatusOK {
		t.Fatalf("karl's sign-in answered %d %q", code, refusal)
	}
	if answered.CoreID != built.coreID || answered.BootstrapToken == "" || answered.Tag != "the core's tag" ||
		string(answered.PublicKey) != "the core's key" {
		t.Errorf("karl's sign-in gave %+v; want the core's paths, a bootstrap token, its key and its tag", answered)
	}
	var offered discoveryproto.OfferResponse
	asked := discoveryproto.OfferRequest{
		PathsRequest: discoveryproto.PathsRequest{BootstrapToken: answered.BootstrapToken},
		Offers:       []discoveryproto.ClientOffer{{SDP: "an offer", Proof: "its proof"}},
	}
	if code, _ := built.ask(t, discoveryproto.OfferPath(built.coreID), asked, &offered); code != http.StatusOK ||
		offered.Answers[0].SDP != "answered under "+karlsSignIn().Person {
		t.Errorf("karl's offer was answered %+v; want it passed under karl's person", offered.Answers)
	}
}

// A sign-in wrong in any part, its realm unknown, its ask not karl's, no ask, or one at a discovery
// that has no realms, answers the same not-found after the same wait from its asking: nothing says
// which part, so no realm is learnt by asking (D297 §2).
func TestRealmSignIn_AnyPartWrongIsTheSameNotFoundAfterTheSameWait(t *testing.T) {
	t.Parallel()
	none := newClientStage(t)
	answerRealms(t, none)
	var answered discoveryproto.ClientRealmResponse
	code, refusal := none.ask(t, discoveryproto.PathRealmSignIn, karlsSignIn(), &answered)
	if code != http.StatusNotFound || refusal != discoveryproto.RefusalNotFound {
		t.Errorf("a sign-in at a discovery with no realms answered %d %q; want notFound", code, refusal)
	}
	built := newClientStageWithRealm(t, "karlnas")
	answerRealms(t, built)
	wrongAsk := karlsSignIn()
	wrongAsk.Ask = proof.RealmAsk(proof.RealmKey("karlnas", "karl", "another password"), "karlnas", wrongAsk.Person)
	unknown := karlsSignIn()
	unknown.Realm = "the-smiths"
	for name, asked := range map[string]discoveryproto.ClientRealmRequest{
		"an unknown realm": unknown, "a wrong password": wrongAsk,
		"no ask": {Realm: "karlnas", Person: wrongAsk.Person},
	} {
		code, refusal = built.ask(t, discoveryproto.PathRealmSignIn, asked, &answered)
		if code != http.StatusNotFound || refusal != discoveryproto.RefusalNotFound {
			t.Errorf("%s answered %d %q; want notFound", name, code, refusal)
		}
	}
	for _, stage := range []clientStage{none, built} {
		stage.mutex.Lock()
		waited := slices.Clone(stage.waited)
		stage.mutex.Unlock()
		isAnother := func(wait time.Duration) bool { return wait != realmAnswerWait }
		if len(waited) == 0 || slices.ContainsFunc(waited, isAnother) {
			t.Errorf("the refusals waited %v; want each the whole %s, the stage's clock still", waited, realmAnswerWait)
		}
	}
}

// A core slow to refuse is answered at the moment an unknown realm is, the wait for the core being no
// longer than a failure's answer, so no realm is told by a later refusal (D312 §17).
func TestRealmSignIn_ACoreSlowToRefuseIsAnsweredWhenAnUnknownRealmIs(t *testing.T) {
	t.Parallel()
	if realmAskWait > realmAnswerWait {
		t.Errorf("a realm sign-in waits %s for its core, past the %s a failure is answered at", realmAskWait,
			realmAnswerWait)
	}
	built := newClientStageWithRealm(t, "karlnas")
	answerRealms(t, built)
	slow := karlsSignIn()
	slow.Person = proof.RealmPerson("karlnas", "slow")
	before := built.clock()
	var answered discoveryproto.ClientRealmResponse
	if code, refusal := built.ask(t, discoveryproto.PathRealmSignIn, slow, &answered); code != http.StatusNotFound ||
		refusal != discoveryproto.RefusalNotFound {
		t.Errorf("a slow core's refusal answered %d %q; want notFound", code, refusal)
	}
	built.mutex.Lock()
	took := built.now.Sub(before)
	for _, wait := range built.waited {
		took += wait
	}
	built.mutex.Unlock()
	if took != realmAnswerWait {
		t.Errorf("a refusal that took the core %s was answered after %s; want %s", slowRefusal, took, realmAnswerWait)
	}
}

// saltOf is the salt the client API answers for realm.
func saltOf(t *testing.T, built clientStage, realm string) string {
	t.Helper()
	var answered discoveryproto.RealmSaltResponse
	if code, refusal := built.ask(t, discoveryproto.PathRealmSalt, discoveryproto.RealmSaltRequest{Realm: realm},
		&answered); code != http.StatusOK {
		t.Fatalf("the salt of %q answered %d %q", realm, code, refusal)
	}
	if salt, err := base64.RawURLEncoding.DecodeString(answered.Salt); err != nil || len(salt) != proof.KeySaltBytes {
		t.Fatalf("the salt of %q is %q, %v; want %d bytes, base64url", realm, answered.Salt, err, proof.KeySaltBytes)
	}
	return answered.Salt
}

// The realm the operator named answers the key salt its claimed core's last heartbeat gave; any other
// realm, and that one before a heartbeat gave a salt, a salt derived from its name under a secret the
// discovery keeps, the same at each ask and after a restart, and another for another realm, so no
// answer tells a realm with a core from one without (D527 §2.2).
func TestRealmSalt_IsTheCoresFromItsHeartbeatAndForAnyOtherRealmTheSameDerivedOne(t *testing.T) {
	t.Parallel()
	built := newClientStageWithRealm(t, "karlnas")
	beforeHeartbeat := saltOf(t, built, "KarlNAS")
	nobody := saltOf(t, built, "nobody")
	if nobody == beforeHeartbeat || nobody == saltOf(t, built, "someone") {
		t.Error("two realms answer one salt; want each its own")
	}
	if again := saltOf(t, built, "Nobody"); again != nobody {
		t.Errorf("a realm asked again, in another case, answers %q; want %q, the first answer", again, nobody)
	}
	coresSalt := base64.RawURLEncoding.EncodeToString([]byte("a core's own 16b"))
	if status := built.call(t, http.MethodPut, discoveryproto.PathAddresses, built.token,
		discoveryproto.AddressesRequest{KeySalt: coresSalt}, nil); status != http.StatusOK {
		t.Fatalf("the heartbeat answered %d", status)
	}
	if salt := saltOf(t, built, "KarlNAS"); salt != coresSalt {
		t.Errorf("after the core's heartbeat, its realm's salt is %q; want the core's, %q", salt, coresSalt)
	}
	restarted, err := New(built.discovery.parts)
	if err != nil {
		t.Fatal(err)
	}
	client := httptest.NewServer(restarted.ClientHandler())
	t.Cleanup(client.Close)
	afterRestart := built
	afterRestart.client = client
	if salt := saltOf(t, afterRestart, "nobody"); salt != nobody {
		t.Errorf("after a restart, a realm with no core answers %q; want %q, as before", salt, nobody)
	}
}
