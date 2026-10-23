// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/platform/backoff"
)

// signalCheckEvery is how often the signalling loop looks whether the core is claimed and so should
// hold the connection, when it holds none; a core newly claimed wakes it at once (claimedNow).
const signalCheckEvery = 10 * time.Second

// pingEvery is how often the core pings its signalling connection, jittered by pingJitter either
// way, so a proxy on the way never finds it idle (Cloudflare closes a WebSocket idle for 100 s) and a
// dead connection is found: a ping not answered by the next one's time is missed, and two missed in
// a row end the connection, which is dialled again (the protocol's §2.5; C11, D292 §9).
const (
	pingEvery     = 27500 * time.Millisecond
	pingJitter    = 0.09
	pingsMissedAt = 2
)

// maxSignalBytes bounds one signalling message: an SDP with its candidates is a few kilobytes.
const maxSignalBytes = 64 << 10

// signal holds the signalling connection while the core is claimed (discovery-core.md C3): the
// offers the discovery passes on are answered by the core's WebRTC server, which checks each itself;
// every request over a connection still carries its own sign-in. A dropped connection is dialled
// again after discovery_retry_first_seconds, jittered, and each failure in a row doubles the wait
// (the protocol's §2.5; D292 §9), so a restarted discovery has its cores back within seconds.
func (service *Service) signal(ctx context.Context) {
	var failures int64
	for ctx.Err() == nil {
		wait, isClaimed := signalCheckEvery, false
		if status := service.Status(); service.parts.Answerer != nil &&
			(status.State == discoveryproto.StateClaimed || status.State == discoveryproto.StateGrace) {
			wasOpen, err := service.holdSignalling(ctx, status.BaseURL)
			if ctx.Err() != nil {
				return
			}
			if wasOpen {
				failures = 0
			}
			service.parts.Logger.WarnContext(ctx, "discovery: the signalling connection ended; dialling again",
				"err", err)
			wait, isClaimed = service.retryWait(ctx, failures), true
			failures++
		}
		if !service.waitToLookAgain(ctx, wait, isClaimed) {
			return
		}
	}
}

// waitToLookAgain waits before the loop's next look, saying false when ctx ends. A claimed core's
// wait is a dial's backoff; an unclaimed core's ends early when it is newly claimed.
func (service *Service) waitToLookAgain(ctx context.Context, wait time.Duration, isClaimed bool) bool {
	if isClaimed {
		return service.parts.Clock.Sleep(ctx, wait) == nil
	}
	timer := service.parts.Clock.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-service.claimedNow:
		return true
	case <-timer.C():
		return true
	}
}

// wakeSignallingIfClaimed has the signalling loop look again now when the core, in state was, is
// newly claimed: it dials at once rather than up to signalCheckEvery later (the owner, 2026-10-18).
func (service *Service) wakeSignallingIfClaimed(was, now discoveryproto.State) {
	if !(Status{State: now}).IsPublished() || (Status{State: was}).IsPublished() {
		return
	}
	select {
	case service.claimedNow <- struct{}{}:
	default:
	}
}

// holdSignalling dials the signalling connection and answers its offers until it ends, saying
// whether it opened. A token the discovery refuses is renewed at once, and dialled with once more.
func (service *Service) holdSignalling(ctx context.Context, baseURL string) (bool, error) {
	token, err := service.session(ctx)
	if err != nil {
		return false, err
	}
	connection, err := service.dialSignalling(ctx, baseURL, token)
	if errors.Is(err, errUnauthorised) {
		service.dropToken(token)
		if token, err = service.session(ctx); err != nil {
			return false, err
		}
		connection, err = service.dialSignalling(ctx, baseURL, token)
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = connection.CloseNow() }() //nolint:errcheck // closing what may be closed
	connection.SetReadLimit(maxSignalBytes)
	service.parts.Logger.InfoContext(ctx, "discovery: the signalling connection is open")
	pinging, stopPinging := context.WithCancel(ctx)
	defer stopPinging()
	go service.keepAlive(pinging, connection)
	var answering sync.WaitGroup
	defer answering.Wait()
	slots := make(chan struct{}, answersAtOnce)
	for {
		var offer discoveryproto.SignalMessage
		if err := wsjson.Read(ctx, connection, &offer); err != nil {
			return true, fmt.Errorf("read an offer: %w", err)
		}
		if offer.Type != discoveryproto.SignalOffer && offer.Type != discoveryproto.SignalRealm {
			continue
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return true, fmt.Errorf("wait to answer an offer: %w", ctx.Err())
		}
		answering.Go(func() {
			defer func() { <-slots }()
			service.answerSignal(ctx, connection, offer)
		})
	}
}

// answersAtOnce is how many offers and realm sign-ins the core answers at once on its signalling
// connection. Answering one waits on gathering the core's candidates, up to stunGatherTimeout, so
// answered in turn a page's four connections took 2, 4, 6 and 8 s (staging, 2026-10-23).
const answersAtOnce = 8

// answerSignal answers an offer or a realm sign-in on the signalling connection. A reply that cannot
// be sent closes the connection, which ends the reading and so the signalling.
func (service *Service) answerSignal(
	ctx context.Context, connection *websocket.Conn, offer discoveryproto.SignalMessage,
) {
	reply := service.answerRealm
	if offer.Type == discoveryproto.SignalOffer {
		reply = service.answerOffer
	}
	if err := wsjson.Write(ctx, connection, reply(ctx, offer)); err != nil && ctx.Err() == nil {
		service.parts.Logger.WarnContext(ctx, "discovery: an answer could not be sent; closing the signalling",
			"offerId", offer.OfferID, "err", err)
		_ = connection.CloseNow() //nolint:errcheck // closing what may be closed
	}
}

// keepAlive pings connection every pingEvery, jittered, until ctx ends, and closes it once
// pingsMissedAt pings in a row went unanswered by the next one's time.
func (service *Service) keepAlive(ctx context.Context, connection *websocket.Conn) {
	answered := make(chan error, 1)
	isWaiting, missed := false, 0
	for {
		every, err := backoff.Jittered(service.parts.Random, pingEvery, pingEvery*2, pingJitter)
		if err != nil {
			every = pingEvery
		}
		timer := service.parts.Clock.NewTimer(every)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C():
		}
		if isWaiting {
			select {
			case err := <-answered:
				isWaiting, missed = false, 0
				if err != nil {
					missed = 1
				}
			default:
				missed++
			}
		}
		if missed >= pingsMissedAt {
			service.parts.Logger.WarnContext(ctx, "discovery: the signalling connection answered no ping; closing it",
				"missed", missed)
			_ = connection.CloseNow() //nolint:errcheck // dead, and the reader ends with it
			return
		}
		if !isWaiting {
			isWaiting = true
			service.pingsUnanswered.Add(1)
			go func() {
				err := connection.Ping(ctx)
				service.pingsUnanswered.Add(-1)
				answered <- err
			}()
		}
	}
}

// dialSignalling opens the signalling connection with token; errUnauthorised when the discovery
// refuses the token.
func (service *Service) dialSignalling(ctx context.Context, baseURL, token string) (*websocket.Conn, error) {
	address := "ws" + strings.TrimPrefix(baseURL, "http") + discoveryproto.PathSignal
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set(discoveryproto.ProtocolHeader, discoveryproto.ProtocolVersion)
	options := &websocket.DialOptions{HTTPClient: service.parts.HTTP, HTTPHeader: header}
	connection, response, err := websocket.Dial(ctx, address, options) //nolint:bodyclose // coder/websocket closes it
	switch {
	case err == nil:
		return connection, nil
	case response != nil && response.StatusCode == http.StatusUnauthorized:
		return nil, errUnauthorised
	}
	return nil, fmt.Errorf("dial the signalling connection: %w", err)
}

// answerOffer is the core's answer to an offer, or its refusal and why.
func (service *Service) answerOffer(
	ctx context.Context, offer discoveryproto.SignalMessage,
) discoveryproto.SignalMessage {
	if why := service.whyUnproven(ctx, offer); why != "" {
		service.parts.Logger.WarnContext(ctx, "discovery: an offer's proof does not hold; refused",
			"offerId", offer.OfferID, "why", why)
		return discoveryproto.SignalMessage{
			Type: discoveryproto.SignalRefusal, OfferID: offer.OfferID, Reason: "the offer's proof does not hold",
		}
	}
	answer, err := service.parts.Answerer.AnswerRelayed(ctx, rtcapi.Description{Type: "offer", SDP: offer.SDP})
	if err != nil {
		reason := "the core could not answer"
		switch {
		case errors.Is(err, rtcapi.ErrTooManyConnections):
			reason = "the core holds as many connections as it allows"
		case errors.Is(err, rtcapi.ErrOfferRefused):
			reason = "the core refused the offer"
		}
		service.parts.Logger.WarnContext(ctx, "discovery: an offer was refused", "offerId", offer.OfferID, "err", err)
		return discoveryproto.SignalMessage{Type: discoveryproto.SignalRefusal, OfferID: offer.OfferID, Reason: reason}
	}
	signed := proof.Answer{
		CoreID: service.Status().CoreID, OfferID: offer.OfferID, OfferFingerprint: rtcapi.FingerprintOf(offer.SDP),
		SDP: answer.SDP,
	}
	return discoveryproto.SignalMessage{
		Type: discoveryproto.SignalAnswer, OfferID: offer.OfferID, SDP: answer.SDP,
		Signature: service.parts.Key.SignAnswer(signed), PublicKey: service.parts.Key.Public(),
	}
}

// whyUnproven says why the offer's proof does not hold under the key the core keeps for the hash the
// discovery passed it on with, or is empty when it holds: an HMAC over the offer's DTLS fingerprint,
// which the discovery cannot make, so an offer it made up, or a client's with its own certificate
// swapped in, is refused before rtcapi sees it (the protocol's §2.5 item 3; A9). Under a person a
// realm sign-in names, a live hand-off's key holds too, which a client origin holding only the
// hand-off's code proves by (D308 §4). Which check failed goes to the log alone: three offers were
// refused just after a sign-in on staging, and the log could not say whether their hash had no key
// or their proof was made under another (2026-10-23).
func (service *Service) whyUnproven(ctx context.Context, offer discoveryproto.SignalMessage) string {
	fingerprint := rtcapi.FingerprintOf(offer.SDP)
	switch {
	case service.parts.OfferKey == nil:
		return "the core looks up no keys"
	case offer.LocatorHash == "":
		return "the offer names no hash"
	case fingerprint == "":
		return "the offer carries no DTLS fingerprint"
	}
	key, err := service.parts.OfferKey(ctx, offer.LocatorHash)
	if err == nil && proof.VerifyOffer(key, fingerprint, offer.Proof) {
		return ""
	}
	if service.parts.HandOffOfferKeys != nil {
		for _, handedOff := range service.parts.HandOffOfferKeys(offer.LocatorHash) {
			if proof.VerifyOffer(handedOff, fingerprint, offer.Proof) {
				return ""
			}
		}
	}
	if err != nil {
		return "no key for its hash: " + err.Error()
	}
	return "its proof was not made under the key its hash has"
}
