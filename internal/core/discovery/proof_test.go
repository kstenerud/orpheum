// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"strings"
	"sync"
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

// The log says which check an offer failed, so a refusal can be told from a race or a client's
// mistake: three offers refused just after a sign-in on staging could not be (2026-10-23).
func TestWhyUnproven_NamesTheCheckAnOfferFailed(t *testing.T) {
	t.Parallel()
	clientSDP := "a client's offer\r\na=fingerprint:sha-256 AB:CD"
	proven := proof.Offer(aSessionKey, rtcapi.FingerprintOf(clientSDP))
	service := &Service{parts: Parts{OfferKey: offerKeys}}
	for _, each := range []struct {
		offer discoveryproto.SignalMessage
		want  string
	}{
		{discoveryproto.SignalMessage{LocatorHash: aLocatorHash, SDP: clientSDP, Proof: proven}, ""},
		{discoveryproto.SignalMessage{SDP: clientSDP, Proof: proven}, "names no hash"},
		{
			discoveryproto.SignalMessage{LocatorHash: aLocatorHash, SDP: "no fingerprint", Proof: proven},
			"no DTLS fingerprint",
		},
		{
			discoveryproto.SignalMessage{LocatorHash: "a hash nobody registered", SDP: clientSDP, Proof: proven},
			"no key for its hash",
		},
		{discoveryproto.SignalMessage{
			LocatorHash: aLocatorHash, SDP: clientSDP,
			Proof: proof.Offer(proof.SessionOfferKey("another session"), rtcapi.FingerprintOf(clientSDP)),
		}, "not made under the key its hash has"},
	} {
		why := service.whyUnproven(t.Context(), each.offer)
		if (each.want == "") != (why == "") || !strings.Contains(why, each.want) {
			t.Errorf("an offer under %q with proof %q: why %q; want one saying %q", each.offer.LocatorHash,
				each.offer.Proof, why, each.want)
		}
	}
}

// A page's offers are answered at once, not in turn: each answer waits on gathering the core's
// candidates, and answered one after another a page's four connections took 2, 4, 6 and 8 s
// (staging, 2026-10-23).
func TestSignal_OffersAreAnsweredAtOnce(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	answerer := &slowAnswering{bothIn: make(chan struct{})}
	built.service.parts.Answerer, built.service.parts.OfferKey = answerer, offerKeys
	built.fake.mutex.Lock()
	built.fake.passesAtOnce = true
	built.fake.mutex.Unlock()
	if _, err := built.service.Connect(t.Context(), built.issuer); err != nil {
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
	t.Cleanup(func() { cancel(); testutil.Receive(t, done, "the signalling to stop") })
	for _, id := range []string{"1", "2"} {
		built.fake.signals <- provenOffer(id, "an offer\r\na=fingerprint:sha-256 "+id)
	}
	// Each answer is held until both offers are being answered, which a core answering in turn
	// never reaches.
	for range 2 {
		if reply := testutil.Receive(t, built.fake.replies, "an answer"); reply.Type != discoveryproto.SignalAnswer {
			t.Errorf("an offer was answered %+v; want an answer", reply)
		}
	}
}

// slowAnswering answers each offer only once two are being answered at the same time.
type slowAnswering struct {
	mutex  sync.Mutex
	count  int
	bothIn chan struct{}
}

func (answerer *slowAnswering) AnswerRelayed(
	ctx context.Context, offer rtcapi.Description, _ func(rtcapi.ProvenAtHome),
) (rtcapi.Description, error) {
	answerer.mutex.Lock()
	answerer.count++
	if answerer.count == 2 {
		close(answerer.bothIn)
	}
	answerer.mutex.Unlock()
	select {
	case <-answerer.bothIn:
	case <-ctx.Done():
		return rtcapi.Description{}, ctx.Err()
	}
	return rtcapi.Description{Type: "answer", SDP: "answer to " + offer.SDP}, nil
}

// signallingWorld is a claimed core holding its signalling connection, its offers checked by keys.
func signallingWorld(
	t *testing.T, keys func(context.Context, string) ([]byte, error),
) (*world, *answering) {
	t.Helper()
	built := newWorld(t)
	answerer := &answering{}
	built.service.parts.Answerer, built.service.parts.OfferKey = answerer, keys
	if _, err := built.service.Connect(t.Context(), built.issuer); err != nil {
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

// An offer passed on under a person a realm sign-in names holds under a live hand-off's key, as the
// client origin holding only the code proves it, and under no stale one (D308 §4).
func TestSignal_AnOfferProvenByALiveHandOffsKeyIsAnswered(t *testing.T) {
	t.Parallel()
	const person, sdp = "a-realm-person", "a client origin's offer\r\na=fingerprint:sha-256 AB:CD"
	live := proof.HandOffOfferKey("a live code")
	built, answerer := signallingWorld(t, offerKeys)
	built.service.parts.HandOffOfferKeys = func(hash string) [][]byte {
		if hash != person {
			return nil
		}
		return [][]byte{live}
	}
	for offerID, key := range map[string][]byte{"1": live, "2": proof.HandOffOfferKey("a stale code")} {
		built.fake.signals <- discoveryproto.SignalMessage{
			Type: discoveryproto.SignalOffer, OfferID: offerID, SDP: sdp, LocatorHash: person,
			Proof: proof.Offer(key, rtcapi.FingerprintOf(sdp)),
		}
		reply := testutil.Receive(t, built.fake.replies, "the core's reply")
		if isAnswered := reply.Type == discoveryproto.SignalAnswer; isAnswered != (offerID == "1") {
			t.Errorf("offer %s, proven by %s, was answered %+v", offerID,
				map[string]string{"1": "the live hand-off's key", "2": "a stale one"}[offerID], reply)
		}
	}
	if offered := answerer.taken(); len(offered) != 1 {
		t.Errorf("rtcapi was given %d offers; want the one proven by the live key", len(offered))
	}
}
