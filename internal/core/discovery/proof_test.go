// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// An offer the discovery passes on reaches rtcapi only when its proof holds under the key the core
// keeps for the hash it came under: a discovery cannot make one up, nor swap its own certificate into
// a client's (the protocol's §2.5 item 3; A9).
func TestSignal_AnOfferWhoseProofDoesNotHoldIsRefusedBeforeRTCSeesIt(t *testing.T) {
	t.Parallel()
	clientSDP := "a client's offer\r\na=fingerprint:sha-256 AB:CD"
	swappedSDP := "the discovery's own offer\r\na=fingerprint:sha-256 99:99"
	for name, offer := range map[string]discoveryproto.SignalMessage{
		"no proof": {LocatorHash: aLocatorHash, SDP: clientSDP},
		"a proof under another key": {
			LocatorHash: aLocatorHash, SDP: clientSDP,
			Proof: proof.Offer(proof.SessionOfferKey("another session"), rtcapi.FingerprintOf(clientSDP)),
		},
		"a hash the core keeps no key for": {
			LocatorHash: "a hash nobody registered", SDP: clientSDP,
			Proof: proof.Offer(aSessionKey, rtcapi.FingerprintOf(clientSDP)),
		},
		"the client's proof on the discovery's own certificate": {
			LocatorHash: aLocatorHash, SDP: swappedSDP,
			Proof: proof.Offer(aSessionKey, rtcapi.FingerprintOf(clientSDP)),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			built, answerer := signallingWorld(t, offerKeys)
			offer.Type, offer.OfferID = discoveryproto.SignalOffer, "1"
			built.fake.signals <- offer
			reply := testutil.Receive(t, built.fake.replies, "the core's reply")
			if reply.Type != discoveryproto.SignalRefusal || !strings.Contains(reply.Reason, "proof") {
				t.Errorf("an offer with %s was answered %+v; want it refused for its proof", name, reply)
			}
			if offered := answerer.taken(); len(offered) != 0 {
				t.Errorf("rtcapi was given %q; want nothing", offered)
			}
		})
	}
	t.Run("a core that can look up no key", func(t *testing.T) {
		t.Parallel()
		built, answerer := signallingWorld(t, nil)
		built.fake.signals <- provenOffer("1", clientSDP)
		reply := testutil.Receive(t, built.fake.replies, "the core's reply")
		if reply.Type != discoveryproto.SignalRefusal || len(answerer.taken()) != 0 {
			t.Errorf("with no keys to check by the offer was answered %+v; want it refused", reply)
		}
	})
}

// signallingWorld is a claimed core holding its signalling connection, its offers checked by keys.
func signallingWorld(
	t *testing.T, keys func(context.Context, string) ([]byte, error),
) (*world, *answering) {
	t.Helper()
	built := newWorld(t)
	answerer := &answering{}
	built.service.parts.Answerer, built.service.parts.OfferKey = answerer, keys
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
	return built, answerer
}
