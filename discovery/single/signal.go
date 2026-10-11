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

// signalling is the core's signalling connection, the newest it opened, and the offers waiting on it
// for an answer, by their ids.
type signalling struct {
	logger  *slog.Logger
	mutex   sync.Mutex
	current *websocket.Conn
	lastID  uint64
	waiting map[string]chan discoveryproto.SignalMessage
}

func newSignalling(logger *slog.Logger) *signalling {
	return &signalling{logger: logger, waiting: map[string]chan discoveryproto.SignalMessage{}}
}

// accept takes the core's signalling connection in place of any it had, and reads its answers until
// it closes.
func (signal *signalling) accept(writer http.ResponseWriter, request *http.Request) {
	connection, err := websocket.Accept(writer, request, nil)
	if err != nil {
		return
	}
	connection.SetReadLimit(signalMessageMost)
	signal.mutex.Lock()
	previous := signal.current
	signal.current = connection
	signal.mutex.Unlock()
	if previous != nil {
		_ = previous.Close(websocket.StatusGoingAway, "the core connected again") //nolint:errcheck // it is replaced
	}
	signal.logger.InfoContext(request.Context(), "discovery: the core's signalling connected")
	signal.read(request.Context(), connection)
	signal.mutex.Lock()
	if signal.current == connection {
		signal.current = nil
	}
	signal.mutex.Unlock()
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
