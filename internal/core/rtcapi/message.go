// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/pion/webrtc/v4"
)

// ErrMessageConnClosed is a message connection whose channel closed.
var ErrMessageConnClosed = errors.New("the data channel closed")

// inboxSize is how many of a client's messages wait for the handler to read them: a WebSocket's
// worth of a session's chatter; a client this far ahead is not being read, and its connection closes.
const inboxSize = 64

// CloseMessage is the core's last message on an upgraded channel, sent as a binary message so no text
// message can be taken for it: the WebSocket close code and reason.
type CloseMessage struct {
	Code   int    `json:"close"`
	Reason string `json:"reason,omitempty"`
}

// MessageConn is a request that asked to become a WebSocket, carried by its data channel: after its
// Accept, each text message either way is one message, as a WebSocket's would be (D273 §8). A handler
// that serves WebSockets finds it in the request's context (MessageConnFrom).
type MessageConn struct {
	channel *webrtc.DataChannel
	inbox   chan []byte
	closed  chan struct{}

	mutex      sync.Mutex
	isAccepted bool
	isClosed   bool
}

type messageConnKey struct{}

// MessageConnFrom is the message connection a request over a data channel asked to become, when it
// asked: the WebSocket a handler would otherwise accept.
func MessageConnFrom(ctx context.Context) (*MessageConn, bool) {
	conn, isConn := ctx.Value(messageConnKey{}).(*MessageConn)
	return conn, isConn
}

// asksForUpgrade reports whether a request head asks to become a WebSocket.
func asksForUpgrade(headers http.Header) bool {
	for name, values := range headers {
		if strings.EqualFold(name, "Upgrade") {
			for _, value := range values {
				if strings.EqualFold(value, "websocket") {
					return true
				}
			}
		}
	}
	return false
}

func newMessageConn(channel *webrtc.DataChannel) *MessageConn {
	return &MessageConn{channel: channel, inbox: make(chan []byte, inboxSize), closed: make(chan struct{})}
}

// Accept answers the request as accepted, after which messages flow both ways.
func (conn *MessageConn) Accept() error {
	conn.mutex.Lock()
	conn.isAccepted = true
	conn.mutex.Unlock()
	head, err := json.Marshal(ResponseHead{Status: http.StatusSwitchingProtocols})
	if err != nil {
		return fmt.Errorf("encode the upgrade's head: %w", err)
	}
	if err = conn.channel.SendText(string(head)); err != nil {
		return fmt.Errorf("accept the upgrade: %w", err)
	}
	return nil
}

// accepted reports whether the handler accepted the upgrade.
func (conn *MessageConn) accepted() bool {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()
	return conn.isAccepted
}

// Read is the client's next message, waiting until one comes, the channel closes, or ctx ends.
func (conn *MessageConn) Read(ctx context.Context) ([]byte, error) {
	select {
	case message := <-conn.inbox:
		return message, nil
	case <-conn.closed:
		return nil, ErrMessageConnClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Write sends one message to the client.
func (conn *MessageConn) Write(ctx context.Context, message []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := conn.channel.SendText(string(message)); err != nil {
		return fmt.Errorf("send a message: %w", err)
	}
	return nil
}

// Close says why the connection ends, as a WebSocket's close code and reason, then closes it.
func (conn *MessageConn) Close(code int, reason string) error {
	encoded, err := json.Marshal(CloseMessage{Code: code, Reason: reason})
	if err == nil {
		err = conn.channel.Send(encoded)
	}
	return errors.Join(err, conn.CloseNow())
}

// CloseNow closes the connection without saying why.
func (conn *MessageConn) CloseNow() error {
	conn.ended()
	if err := conn.channel.Close(); err != nil {
		return fmt.Errorf("close the data channel: %w", err)
	}
	return nil
}

// take takes one of the client's messages after its request; one that finds the inbox full closes
// the connection, as a client that far ahead is not being read.
func (conn *MessageConn) take(message []byte) {
	select {
	case conn.inbox <- message:
	default:
		_ = conn.CloseNow() //nolint:errcheck // a client not being read: closed, whatever the error
	}
}

// ended marks the connection closed, once, which ends every Read.
func (conn *MessageConn) ended() {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()
	if !conn.isClosed {
		conn.isClosed = true
		close(conn.closed)
	}
}
