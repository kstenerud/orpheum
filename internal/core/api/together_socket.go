// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/internal/core/together"
)

// togetherSocketPath is the session WebSocket's path (cut-a §5.1): one per device.
const togetherSocketPath = Prefix + "/together/ws"

// helloWait is how long a device has to say hello once its socket is open: a second's work on any
// network, with room for a slow phone.
const helloWait = 10 * time.Second

// maxTogetherMessageBytes bounds one message a device sends: the largest is voice signalling's SDP,
// a few kilobytes.
const maxTogetherMessageBytes = 64 << 10

// outboxSize is how many messages a device may fall behind by before it is closed: a group change
// and a roster each tick at most, so a device this far behind is not reading at all.
const outboxSize = 64

// serveTogetherSocket is the session WebSocket (cut-a §5.1): the device says hello with its
// membership's credential, and a signed-in person's session token when it has one (D243 §2), never
// in the URL (D154); then its messages go to the hub and the hub's to it, until either end closes.
func (server *Server) serveTogetherSocket(writer http.ResponseWriter, request *http.Request) {
	dependencies, err := server.readyDependencies()
	if err != nil {
		http.Error(writer, "the server is not ready", http.StatusServiceUnavailable)
		return
	}
	connection, err := websocket.Accept(writer, request, nil)
	if err != nil {
		return // Accept has answered
	}
	defer func() { _ = connection.CloseNow() }() //nolint:errcheck // closing what may be closed
	connection.SetReadLimit(maxTogetherMessageBytes)
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	hello, err := readHello(ctx, connection)
	if err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "say hello first") //nolint:errcheck // closing
		return
	}
	var person *together.Person
	if hello.SessionToken != "" {
		if principal, authErr := dependencies.Auth.Authenticate(ctx, hello.SessionToken); authErr == nil {
			person = &together.Person{UserID: principal.UserID, Name: principal.Name}
		}
	}
	outbox := make(chan together.Outbound, outboxSize)
	var closing sync.Once
	send := func(message together.Outbound) {
		select {
		case outbox <- message:
		default:
			closing.Do(cancel) // too far behind: it reads nothing
		}
	}
	attachment, err := dependencies.Together.Attach(hello.Credential, hello.Role, hello.CanSetRate, person, send)
	if err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "no such session") //nolint:errcheck // closing
		return
	}
	written := make(chan struct{})
	go func() {
		defer close(written)
		writeOutbox(ctx, connection, outbox, cancel)
	}()
	readInto(ctx, connection, dependencies.Together, attachment)
	dependencies.Together.Detach(attachment) // nothing is sent to it after this
	close(outbox)
	<-written
}

// readHello reads a device's first message, which must be a hello, within helloWait.
func readHello(ctx context.Context, connection *websocket.Conn) (together.Inbound, error) {
	helloContext, cancel := context.WithTimeout(ctx, helloWait)
	defer cancel()
	var hello together.Inbound
	if err := wsjson.Read(helloContext, connection, &hello); err != nil {
		return hello, err //nolint:wrapcheck // the socket's, to close on
	}
	if hello.Type != together.MessageHello || hello.Credential == "" {
		return hello, errors.New("the first message is not a hello with a credential")
	}
	return hello, nil
}

// readInto hands each message the device sends to the hub, until the socket or ctx ends.
func readInto(ctx context.Context, connection *websocket.Conn, hub *together.Hub, attachment together.Attachment) {
	for {
		var message together.Inbound
		if err := wsjson.Read(ctx, connection, &message); err != nil {
			return
		}
		hub.Handle(ctx, attachment, message)
	}
}

// writeOutbox writes the hub's messages to the device until the outbox closes; an ended session's
// last message closes the socket after it.
func writeOutbox(
	ctx context.Context, connection *websocket.Conn, outbox <-chan together.Outbound, cancel context.CancelFunc,
) {
	for message := range outbox {
		if err := wsjson.Write(ctx, connection, message); err != nil {
			cancel()
			continue // drain, so the hub's sends never block
		}
		if message.Type == together.MessageEnded {
			_ = connection.Close(websocket.StatusNormalClosure, message.Reason) //nolint:errcheck // closing
			cancel()
		}
	}
}
