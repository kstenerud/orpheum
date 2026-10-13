// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

// signalling is the core's signalling connection, the newest its live instance opened, that
// instance, and the offers waiting on it for an answer, by their ids.
type signalling struct {
	logger   *slog.Logger
	mutex    sync.Mutex
	current  *websocket.Conn
	instance string
	lastID   uint64
	waiting  map[string]chan discoveryproto.SignalMessage
}

func newSignalling(logger *slog.Logger) *signalling {
	return &signalling{logger: logger, waiting: map[string]chan discoveryproto.SignalMessage{}}
}

// accept takes the signalling connection of the core's instance in place of any it had, and reads its
// answers until it closes. Only the live instance holds a session to call it with (D293 §1).
func (signal *signalling) accept(writer http.ResponseWriter, request *http.Request, instanceID string) {
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
		signal.current, signal.instance = nil, ""
	}
	signal.mutex.Unlock()
}

// answers is whether instanceID holds the signalling connection and answers a ping on it within wait.
func (signal *signalling) answers(ctx context.Context, instanceID string, wait time.Duration) bool {
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

// dropSilent drops the signalling connection when instanceID holds it: it did not answer a ping, so
// it is not asked to close, which would wait for an answer that never comes.
func (signal *signalling) dropSilent(instanceID string) {
	if connection := signal.take(instanceID); connection != nil {
		_ = connection.CloseNow() //nolint:errcheck // it is dead
	}
}

// closeAll closes the signalling connection, whichever instance holds it, saying why; the closing
// handshake runs apart, so the call that ended it does not wait on the core.
func (signal *signalling) closeAll(reason string) {
	signal.mutex.Lock()
	instanceID := signal.instance
	signal.mutex.Unlock()
	if connection := signal.take(instanceID); connection != nil {
		go func() { _ = connection.Close(websocket.StatusPolicyViolation, reason) }() //nolint:errcheck // it is ended
	}
}

// take removes the signalling connection when instanceID holds it, and returns it.
func (signal *signalling) take(instanceID string) *websocket.Conn {
	signal.mutex.Lock()
	defer signal.mutex.Unlock()
	connection := signal.current
	if connection == nil || signal.instance != instanceID {
		return nil
	}
	signal.current, signal.instance = nil, ""
	return connection
}

// read hands each answer or refusal the core sends to the offer waiting on its id.
func (signal *signalling) read(ctx context.Context, connection *websocket.Conn) {
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

// Offer passes a client's WebRTC offer to the core and returns its answer: at once ErrCoreNotConnected
// when the core has no signalling connection, ErrRefused when it refuses, and the context's error
// when it does not answer in time (the spec's §3.5).
func (discovery *Discovery) Offer(ctx context.Context, sdp string) (string, error) {
	signal := discovery.signal
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
		return "", ErrCoreNotConnected
	}
	defer func() {
		signal.mutex.Lock()
		delete(signal.waiting, offerID)
		signal.mutex.Unlock()
	}()
	offer := discoveryproto.SignalMessage{Type: discoveryproto.SignalOffer, OfferID: offerID, SDP: sdp}
	if err := wsjson.Write(ctx, connection, offer); err != nil {
		return "", fmt.Errorf("pass the offer to the core: %w", err)
	}
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("wait for the core's answer: %w", ctx.Err())
	case reply := <-replied:
		if reply.Type != discoveryproto.SignalAnswer {
			return "", fmt.Errorf("%w: %s", ErrRefused, reply.Reason)
		}
		return reply.SDP, nil
	}
}
