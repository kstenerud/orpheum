// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// The realm the discovery gave the claimed core is in its status, as each status call says it: the
// host's account at the discovery holds it, not the core (D297 §1).
func TestStatus_NamesTheRealmTheDiscoveryGaveTheCore(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer); err != nil {
		t.Fatal(err)
	}
	var told []string
	built.service.parts.OnRealm = func(realm string) { told = append(told, realm) }
	built.fake.claim()
	built.fake.realm = "karlnas.live"
	built.service.step(ctx)
	if got := built.service.Status().Realm; got != "karlnas.live" {
		t.Errorf("the status names the realm %q; want karlnas.live, the discovery's", got)
	}
	built.service.askStatus(ctx, built.service.Status())
	if !slices.Equal(told, []string{"karlnas.live"}) {
		t.Errorf("the core was told the realms %q; want karlnas.live, once, as it came", told)
	}
}

// A realm sign-in the discovery passes on is answered by the core's identity key and its tag over
// it under the person's key, which a page checks, and refused when the person's proof does not hold
// (D297 §3).
func TestSignal_ARealmSignInIsAnsweredByTheCoresTagUnderThePersonsKey(t *testing.T) {
	t.Parallel()
	key := []byte("karl's realm key, 32 bytes long!")
	built, _ := signallingWorld(t, nil)
	built.service.parts.RealmSignIn = func(_ context.Context, person, ask string) ([]byte, error) {
		if person == "karl" && ask == "his proof" {
			return key, nil
		}
		return nil, errors.New("the ask does not hold")
	}
	built.fake.signals <- discoveryproto.SignalMessage{
		Type: discoveryproto.SignalRealm, OfferID: "1", Person: "karl", Ask: "his proof",
	}
	reply := testutil.Receive(t, built.fake.replies, "the core's answer")
	if reply.Type != discoveryproto.SignalRealm || string(reply.PublicKey) != string(built.key.Public()) ||
		!proof.VerifyRealmTag(key, fakeCoreID, built.key.Public(), reply.Tag) {
		t.Errorf("karl's sign-in was answered %+v; want the core's key and its tag under his key", reply)
	}
	built.fake.signals <- discoveryproto.SignalMessage{
		Type: discoveryproto.SignalRealm, OfferID: "2", Person: "karl", Ask: "a guess",
	}
	reply = testutil.Receive(t, built.fake.replies, "the core's refusal")
	if reply.Type != discoveryproto.SignalRefusal || reply.Tag != "" {
		t.Errorf("a guess was answered %+v; want it refused, with no tag", reply)
	}
}
