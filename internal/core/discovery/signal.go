// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/proof"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/platform/backoff"
)

// signalCheckEvery is how often the signalling loop looks whether the core is claimed and so should
// hold the connection, when it holds none.
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
// again after RetryFirst, jittered, and each failure in a row doubles the wait (the protocol's §2.5;
// D292 §9), so a restarted discovery has its cores back within seconds.
func (service *Service) signal(ctx context.Context) {
	var failures int64
	for ctx.Err() == nil {
		wait := signalCheckEvery
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
			wait = service.retryWait(ctx, failures)
			failures++
		}
		if service.parts.Clock.Sleep(ctx, wait) != nil {
			return
		}
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
	for {
		var offer discoveryproto.SignalMessage
		if err := wsjson.Read(ctx, connection, &offer); err != nil {
			return true, fmt.Errorf("read an offer: %w", err)
		}
		if offer.Type != discoveryproto.SignalOffer {
			continue
		}
		reply := service.answerOffer(ctx, offer)
		if err := wsjson.Write(ctx, connection, reply); err != nil {
			return true, fmt.Errorf("send an answer: %w", err)
		}
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
			go func() { answered <- connection.Ping(ctx) }()
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
	if !service.isProven(ctx, offer) {
		service.parts.Logger.WarnContext(ctx, "discovery: an offer's proof does not hold; refused",
			"offerId", offer.OfferID)
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

// isProven is whether the offer's proof holds under the key the core keeps for the hash the discovery
// passed it on with: an HMAC over the offer's DTLS fingerprint, which the discovery cannot make, so an
// offer it made up, or a client's with its own certificate swapped in, is refused before rtcapi sees
// it (the protocol's §2.5 item 3; A9).
func (service *Service) isProven(ctx context.Context, offer discoveryproto.SignalMessage) bool {
	fingerprint := rtcapi.FingerprintOf(offer.SDP)
	if service.parts.OfferKey == nil || offer.LocatorHash == "" || fingerprint == "" {
		return false
	}
	key, err := service.parts.OfferKey(ctx, offer.LocatorHash)
	if err != nil {
		return false
	}
	return proof.VerifyOffer(key, fingerprint, offer.Proof)
}
