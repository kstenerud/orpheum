// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/live"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/platform/strictjson"
)

// The presence socket (D337 §2; plans/cast-to-device.md §3) is a second connection to the session
// socket's address, opened with a hello of its own (live.MessageHello): every signed-in page holds
// one, so the core knows which of a person's devices are open now and can hand one a cast. Sharing
// the address, it travels wherever the session socket does, directly or over a WebRTC data channel
// (D273 §8), with nothing new in the transports.

// The keys log lines name a person, their device and its browser by: a live connection's, a cast's
// and a page's.
const (
	logKeyUserID   = "userId"
	logKeyDeviceID = "deviceId"
	// The browser's own words for itself (ClientFacts).
	logKeyUserAgent = "userAgent"
)

// isLiveHello is whether a device's first message opens a live connection rather than a session's.
func isLiveHello(first json.RawMessage) bool {
	var hello struct {
		Type    live.MessageType `json:"type"`
		Channel string           `json:"channel"`
	}
	if strictjson.Unmarshal(first, &hello) != nil {
		return false
	}
	return hello.Type == live.MessageHello && hello.Channel == live.ChannelLive || hello.Type == live.MessageOldHello
}

// serveLiveSocket is a device's live connection, after its hello: registered while it lasts, in front
// or behind as its tab says (D338 §2), its person's connected devices and any cast handed to it
// written to it, its pings answered, until either end closes or it stays silent past the silence
// limit. A sign-in of no device is refused, as one that cannot be cast to (§2).
func (server *Server) serveLiveSocket(
	ctx context.Context, connection togetherSocket, dependencies *Dependencies, first json.RawMessage,
	request *http.Request,
) {
	var hello live.Inbound
	if err := strictjson.Unmarshal(first, &hello); err != nil {
		_ = connection.closeWith(websocket.StatusPolicyViolation, "say hello first") //nolint:errcheck // closing
		return
	}
	principal, err := dependencies.Auth.Authenticate(ctx, hello.SessionToken)
	if err != nil {
		code, why := websocket.StatusPolicyViolation, "sign in first"
		if !errors.Is(err, auth.ErrNotAuthenticated) {
			server.logger.ErrorContext(ctx, "live: a device's sign-in could not be checked", "err", err)
			code, why = websocket.StatusInternalError, "try again"
		}
		_ = connection.closeWith(code, why) //nolint:errcheck // closing
		return
	}
	if principal.DeviceID == 0 {
		const why = "this sign-in has no device"
		_ = connection.closeWith(websocket.StatusPolicyViolation, why) //nolint:errcheck // closing
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	outbox := make(chan live.Outbound, outboxSize)
	var closing sync.Once
	send := func(message live.Outbound) {
		select {
		case outbox <- message:
		default:
			closing.Do(cancel) // too far behind: it reads nothing
		}
	}
	registered := dependencies.Live.Connect(principal.UserID, principal.DeviceID, !hello.IsBehind, send)
	device := []any{
		logKeyUserID, principal.UserID, logKeyDeviceID, principal.DeviceID, "client", clientScriptOf(hello.Client),
	}
	server.logger.DebugContext(ctx, "live: a device connected", append(device, slogx.UserAgent(request))...)
	written := make(chan struct{})
	go func() {
		defer close(written)
		writeLive(ctx, connection, outbox, cancel)
	}()
	showFront := func(isInFront bool) { dependencies.Live.ShowFront(registered, isInFront) }
	ended := readLive(ctx, connection, send, showFront, time.Duration(server.togetherSilence.Load()))
	dependencies.Live.Disconnect(registered) // nothing is sent to it after this
	server.logger.DebugContext(ctx, "live: a device disconnected", append(device, "why", ended.Error())...)
	close(outbox)
	<-written
}

// readLive answers each ping the device sends and passes each move of its tab, to the front or
// behind, to showFront (D338 §2), until the socket or ctx ends or the device sends nothing for
// silence, and says why it ended.
func readLive(
	ctx context.Context, connection togetherSocket, send live.Sender, showFront func(isInFront bool),
	silence time.Duration,
) error {
	for {
		var message live.Inbound
		readContext, cancel := context.WithTimeoutCause(ctx, silence, errSilent)
		err := connection.readJSON(readContext, &message)
		isSilent := errors.Is(context.Cause(readContext), errSilent)
		cancel()
		if isSilent {
			return fmt.Errorf("sent nothing for %s: %w", silence, errSilent)
		}
		if err != nil {
			return err
		}
		switch message.Type {
		case live.MessagePing:
			send(live.Outbound{Type: live.MessagePong, TMono: message.TMono})
		case live.MessageFront:
			showFront(!message.IsBehind)
		// The server's own messages, never a page's: a cast's sending (D364; VQ90) and a removed
		// device's signing out (VQ87) among them.
		case live.MessageHello, live.MessageOldHello, live.MessageDevices, live.MessageReceive,
			live.MessageSending, live.MessagePong,
			live.MessageSignedOut:
		}
	}
}

// writeLive writes the registry's messages to the device until the outbox closes.
func writeLive(ctx context.Context, connection togetherSocket, outbox <-chan live.Outbound, cancel context.CancelFunc) {
	for message := range outbox {
		if err := connection.writeJSON(ctx, message); err != nil {
			cancel()
		}
	}
}
