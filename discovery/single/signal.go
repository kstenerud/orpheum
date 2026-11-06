// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// Errors Offer returns.
var (
	ErrCoreNotConnected = errors.New("the core is not connected to its discovery")
	// ErrRefused is the core's refusal of an offer, its reason wrapped with it.
	ErrRefused = errors.New("the core refused the offer")
)

// signalMessageMost is the largest signalling message read: an SDP of a few kilobytes (the spec's
// §3.5).
const signalMessageMost = 64 << 10

// Signal is one core's signalling connection, the newest its live instance opened, that instance,
// and the offers waiting on it for an answer, by their ids: a building block a discovery of many cores
// holds one of for each (D275 §1), as this one holds one for its core.
type Signal struct {
	logger   *slog.Logger
	now      func() time.Time
	mutex    sync.Mutex
	current  *websocket.Conn
	instance string
	lastID   uint64
	waiting  map[string]chan discoveryproto.SignalMessage
	// lastClosed is when the core last held no connection, from its closing: zero before the
	// first. A core is online while it holds one (the protocol's §2.4; C16).
	lastClosed time.Time
}

// NewSignal is a core's signalling, holding no connection yet.
func NewSignal(logger *slog.Logger, now func() time.Time) *Signal {
	return &Signal{logger: logger, now: now, waiting: map[string]chan discoveryproto.SignalMessage{}}
}

// Standing is whether the core holds a signalling connection, and when it last held none.
func (signal *Signal) Standing() (isOnline bool, offlineSince time.Time) {
	signal.mutex.Lock()
	defer signal.mutex.Unlock()
	return signal.current != nil, signal.lastClosed
}

// Accept takes the signalling connection of the core's instance in place of any it had, and reads its
// answers until it closes. Only the live instance holds a session to call it with (D293 §1).
func (signal *Signal) Accept(writer http.ResponseWriter, request *http.Request, instanceID string) {
	connection, err := websocket.Accept(writer, request, nil)
	if err != nil {
		return
	}
	connection.SetReadLimit(signalMessageMost)
	signal.mutex.Lock()
	previous := signal.current
	signal.current, signal.instance = connection, instanceID
	signal.mutex.Unlock()
	if previous != nil {
		_ = previous.Close(websocket.StatusGoingAway, "the core connected again") //nolint:errcheck // it is replaced
	}
	signal.logger.InfoContext(request.Context(), "discovery: the core's signalling connected")
	signal.read(request.Context(), connection)
	signal.mutex.Lock()
	if signal.current == connection {
		signal.current, signal.instance, signal.lastClosed = nil, "", signal.now()
	}
	signal.mutex.Unlock()
}

// Answers is whether instanceID holds the signalling connection and answers a ping on it within wait.
func (signal *Signal) Answers(ctx context.Context, instanceID string, wait time.Duration) bool {
	signal.mutex.Lock()
	connection := signal.current
	isItsOwn := connection != nil && signal.instance == instanceID
	signal.mutex.Unlock()
	if !isItsOwn {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	return connection.Ping(ctx) == nil
}

// DropSilent drops the signalling connection when instanceID holds it: it did not answer a ping, so
// it is not asked to close, which would wait for an answer that never comes.
func (signal *Signal) DropSilent(instanceID string) {
	if connection := signal.take(instanceID); connection != nil {
		_ = connection.CloseNow() //nolint:errcheck // it is dead
	}
}

// CloseAll closes the signalling connection, whichever instance holds it, saying why; the closing
// handshake runs apart, so the call that ended it does not wait on the core.
func (signal *Signal) CloseAll(reason string) {
	signal.mutex.Lock()
	instanceID := signal.instance
	signal.mutex.Unlock()
	if connection := signal.take(instanceID); connection != nil {
		go func() { _ = connection.Close(websocket.StatusPolicyViolation, reason) }() //nolint:errcheck // it is ended
	}
}

// take removes the signalling connection when instanceID holds it, and returns it.
func (signal *Signal) take(instanceID string) *websocket.Conn {
	signal.mutex.Lock()
	defer signal.mutex.Unlock()
	connection := signal.current
	if connection == nil || signal.instance != instanceID {
		return nil
	}
	signal.current, signal.instance, signal.lastClosed = nil, "", signal.now()
	return connection
}

// CopyRefused tells the instance holding the signalling connection that a copy of the core asked for
// a session from address, an IP address with no port, when, and was refused because this one
// answers (discoveryproto.SignalCopyRefused; D437 §4). The core sends no reply; with no connection,
// it is ErrCoreNotConnected.
func (signal *Signal) CopyRefused(ctx context.Context, address string, when time.Time) error {
	signal.mutex.Lock()
	connection := signal.current
	signal.mutex.Unlock()
	if connection == nil {
		return ErrCoreNotConnected
	}
	message := discoveryproto.SignalMessage{
		Type: discoveryproto.SignalCopyRefused, Address: address, At: when.UnixMilli(),
	}
	if err := wsjson.Write(ctx, connection, message); err != nil {
		return fmt.Errorf("tell the core a copy was refused: %w", err)
	}
	return nil
}

// read hands each answer or refusal the core sends to the offer waiting on its id.
func (signal *Signal) read(ctx context.Context, connection *websocket.Conn) {
	for {
		var reply discoveryproto.SignalMessage
		if err := wsjson.Read(ctx, connection, &reply); err != nil {
			return
		}
		signal.mutex.Lock()
		waiting := signal.waiting[reply.OfferID]
		delete(signal.waiting, reply.OfferID)
		signal.mutex.Unlock()
		if waiting != nil {
			waiting <- reply
		}
	}
}

// Answer is the core's answer to an offer: the id it was passed under, its SDP, and the core's
// signature over it, which the client checks with proof.VerifyAnswer before it takes the answer (the
// protocol's §2.5 item 4).
type Answer struct {
	OfferID   string
	SDP       string
	Signature []byte
	PublicKey []byte
}

// IsCoreOnline is whether the core holds its signalling connection, so offers and realm sign-ins
// reach it.
func (discovery *Discovery) IsCoreOnline() bool {
	isOnline, _ := discovery.signal.Standing()
	return isOnline
}

// Offer passes a client's WebRTC offer to the core, with the hash the client found it by, the
// offer's proof, which the core checks itself, and the network the offer came from
// (discoveryproto.HouseNetworkOf its source; none over IPv4), which the core learns as the house's
// once the offer's connection proves itself from home (D356 §2); and returns its answer: at once
// ErrCoreNotConnected when the core has no signalling connection, ErrRefused when it refuses, and the
// context's error when it does not answer in time (the protocol's §2.5).
func (discovery *Discovery) Offer(
	ctx context.Context, offer discoveryproto.ClientOffer, locatorHash string, clientNetwork netip.Prefix,
) (Answer, error) {
	return discovery.signal.Offer(ctx, offer, locatorHash, clientNetwork)
}

// Offer passes a client's offer to the core over this connection, as Discovery.Offer says; a
// clientNetwork that is no house network (discoveryproto.IsHouseNetwork) passes none.
func (signal *Signal) Offer(
	ctx context.Context, offer discoveryproto.ClientOffer, locatorHash string, clientNetwork netip.Prefix,
) (Answer, error) {
	message := discoveryproto.SignalMessage{
		Type: discoveryproto.SignalOffer, SDP: offer.SDP, LocatorHash: locatorHash, Proof: offer.Proof,
	}
	if discoveryproto.IsHouseNetwork(clientNetwork) {
		message.ClientNetwork = clientNetwork.String()
	}
	reply, offerID, err := signal.pass(ctx, message, discoveryproto.SignalAnswer)
	if err != nil {
		return Answer{OfferID: offerID}, err
	}
	return Answer{OfferID: offerID, SDP: reply.SDP, Signature: reply.Signature, PublicKey: reply.PublicKey}, nil
}

// RealmAnswer is the core's answer to a realm sign-in it took: its identity key, and its tag over it
// under the person's key (proof.RealmTag), which the page checks.
type RealmAnswer struct {
	PublicKey []byte
	Tag       string
}

// Realm passes a realm sign-in to the core, who it is for and its proof, which the core checks
// under that person's key, and returns its answer: ErrCoreNotConnected, ErrRefused and the
// context's error as Offer has them (D297 §3).
func (signal *Signal) Realm(ctx context.Context, person, ask string) (RealmAnswer, error) {
	reply, _, err := signal.pass(ctx, discoveryproto.SignalMessage{
		Type: discoveryproto.SignalRealm, Person: person, Ask: ask,
	}, discoveryproto.SignalRealm)
	if err != nil {
		return RealmAnswer{}, err
	}
	return RealmAnswer{PublicKey: reply.PublicKey, Tag: reply.Tag}, nil
}

// pass sends message to the core under a new id and waits for its reply of type taken: at once
// ErrCoreNotConnected when the core has no signalling connection, ErrRefused when it replies another
// type, and the context's error when it does not reply in time.
func (signal *Signal) pass(
	ctx context.Context, message discoveryproto.SignalMessage, taken string,
) (discoveryproto.SignalMessage, string, error) {
	signal.mutex.Lock()
	connection := signal.current
	signal.lastID++
	offerID := strconv.FormatUint(signal.lastID, 10)
	replied := make(chan discoveryproto.SignalMessage, 1)
	if connection != nil {
		signal.waiting[offerID] = replied
	}
	signal.mutex.Unlock()
	if connection == nil {
		return discoveryproto.SignalMessage{}, offerID, ErrCoreNotConnected
	}
	defer func() {
		signal.mutex.Lock()
		delete(signal.waiting, offerID)
		signal.mutex.Unlock()
	}()
	message.OfferID = offerID
	if err := wsjson.Write(ctx, connection, message); err != nil {
		return discoveryproto.SignalMessage{}, offerID, fmt.Errorf("pass the %s to the core: %w", message.Type, err)
	}
	select {
	case <-ctx.Done():
		return discoveryproto.SignalMessage{}, offerID, fmt.Errorf("wait for the core's answer: %w", ctx.Err())
	case reply := <-replied:
		if reply.Type != taken {
			return discoveryproto.SignalMessage{}, offerID, fmt.Errorf("%w: %s", ErrRefused, reply.Reason)
		}
		return reply, offerID, nil
	}
}
