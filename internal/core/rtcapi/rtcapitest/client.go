// SPDX-License-Identifier: GPL-3.0-or-later

package rtcapitest

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

// waitLimit bounds every wait on the connection: loopback answers in milliseconds, so one that does
// not is a failure, not a slow machine.
const waitLimit = 30 * time.Second

// receive waits for ready, failing the test after waitLimit. The core's packages never reach os/exec
// (TestArch_CoreNeverExecs), so testutil.Receive, whose package does, is not open to it.
func receive[Value any](tb testing.TB, ready <-chan Value, waitingFor string) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(tb.Context(), waitLimit)
	defer cancel()
	select {
	case <-ready:
	case <-ctx.Done():
		tb.Fatalf("waited %s for %s", waitLimit, waitingFor)
	}
}

// Client is one peer connection to a core.
type Client struct {
	tb         testing.TB
	connection *webrtc.PeerConnection
}

// Answerer takes the client's offer to the core and returns its answer: rtcapi.Server.Answer, or the
// offer endpoint.
type Answerer func(ctx context.Context, offer rtcapi.Description) (rtcapi.Description, error)

// Connect makes a client on the loopback and connects it through answer.
func Connect(tb testing.TB, answer Answerer) *Client {
	tb.Helper()
	settings := webrtc.SettingEngine{}
	settings.SetIncludeLoopbackCandidate(true)
	connection, err := webrtc.NewAPI(webrtc.WithSettingEngine(settings)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = connection.Close() }) //nolint:errcheck // the test is over
	// An offer needs something to negotiate: a channel the client opens and never uses.
	if _, err = connection.CreateDataChannel("hello", nil); err != nil {
		tb.Fatal(err)
	}
	offer, err := connection.CreateOffer(nil)
	if err != nil {
		tb.Fatal(err)
	}
	gathered := webrtc.GatheringCompletePromise(connection)
	if err = connection.SetLocalDescription(offer); err != nil {
		tb.Fatal(err)
	}
	receive(tb, gathered, "the client's candidates")
	local := connection.LocalDescription()
	answered, err := answer(tb.Context(), rtcapi.Description{Type: local.Type.String(), SDP: local.SDP})
	if err != nil {
		tb.Fatal(err)
	}
	connected := make(chan struct{})
	var once sync.Once
	connection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			once.Do(func() { close(connected) })
		}
	})
	err = connection.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answered.SDP})
	if err != nil {
		tb.Fatal(err)
	}
	receive(tb, connected, "the connection")
	return &Client{tb: tb, connection: connection}
}

// Address is the client's own end of the candidate pair its connection selected, as the core sees
// it: host and port.
func (client *Client) Address() string {
	client.tb.Helper()
	pair, err := client.connection.SCTP().Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil || pair == nil || pair.Local == nil {
		client.tb.Fatalf("the connection has selected no candidate pair: %v", err)
	}
	return net.JoinHostPort(pair.Local.Address, strconv.Itoa(int(pair.Local.Port)))
}

// grantEvery is how many body messages the client reads before granting as many more, as the
// browser's reader does (rtcapi.Credit).
const grantEvery = 16

// grant grants the core messages more of a response's body.
func grant(tb testing.TB, channel *webrtc.DataChannel, messages int) {
	tb.Helper()
	encoded, err := json.Marshal(rtcapi.Credit{Messages: messages})
	if err == nil {
		err = channel.SendText(string(encoded))
	}
	if err != nil {
		tb.Error(err)
	}
}

// Reply is a response as the client read it.
type Reply struct {
	Head rtcapi.ResponseHead
	Body []byte
	// LargestMessageBytes is the largest body message it came in.
	LargestMessageBytes int
}

// Do sends one request on a channel of its own and reads its response whole.
func (client *Client) Do(head rtcapi.RequestHead, body []byte) Reply {
	client.tb.Helper()
	channel, err := client.connection.CreateDataChannel("request", nil)
	if err != nil {
		client.tb.Fatal(err)
	}
	var got Reply
	read := 0
	done := make(chan struct{})
	var once sync.Once
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		switch {
		case message.IsString:
			if err := json.Unmarshal(message.Data, &got.Head); err != nil {
				client.tb.Error(err)
			}
		case len(message.Data) == 0:
			once.Do(func() { close(done) })
		default:
			got.Body = append(got.Body, message.Data...)
			got.LargestMessageBytes = max(got.LargestMessageBytes, len(message.Data))
			read++
			if read%grantEvery == 0 {
				grant(client.tb, channel, grantEvery)
			}
		}
	})
	channel.OnOpen(func() { send(client.tb, channel, head, body) })
	receive(client.tb, done, "the response to "+head.Path)
	_ = channel.Close() //nolint:errcheck // the response is read
	return got
}

// send sends a request's head, its body and the empty message that ends it.
func send(tb testing.TB, channel *webrtc.DataChannel, head rtcapi.RequestHead, body []byte) {
	tb.Helper()
	encoded, err := json.Marshal(head)
	if err == nil {
		err = channel.SendText(string(encoded))
	}
	for start := 0; err == nil && start < len(body); start += rtcapi.MaxMessageBytes {
		err = channel.Send(body[start:min(len(body), start+rtcapi.MaxMessageBytes)])
	}
	if err == nil {
		err = channel.Send([]byte{})
	}
	if err != nil {
		tb.Error(err)
	}
}

// Messages is a channel a request asked to become a WebSocket on: once accepted, a text message each
// way is one message, and the core's binary message is its close (rtcapi.CloseMessage).
type Messages struct {
	tb       testing.TB
	channel  *webrtc.DataChannel
	head     chan rtcapi.ResponseHead
	received chan string
	closed   chan rtcapi.CloseMessage
}

// Upgrade asks for path to become a WebSocket over a channel of its own, and returns once the core
// has answered the request's head.
func (client *Client) Upgrade(path string) (*Messages, rtcapi.ResponseHead) {
	client.tb.Helper()
	channel, err := client.connection.CreateDataChannel("upgrade", nil)
	if err != nil {
		client.tb.Fatal(err)
	}
	messages := &Messages{
		tb: client.tb, channel: channel, head: make(chan rtcapi.ResponseHead, 1),
		received: make(chan string, 64), closed: make(chan rtcapi.CloseMessage, 1),
	}
	isHeadRead := false
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		switch {
		case message.IsString && !isHeadRead:
			isHeadRead = true
			var head rtcapi.ResponseHead
			if err := json.Unmarshal(message.Data, &head); err != nil {
				client.tb.Error(err)
			}
			messages.head <- head
		case message.IsString:
			messages.received <- string(message.Data)
		default:
			var closing rtcapi.CloseMessage
			if err := json.Unmarshal(message.Data, &closing); err != nil {
				client.tb.Error(err)
			}
			messages.closed <- closing
		}
	})
	head := rtcapi.RequestHead{
		Method: "GET", Path: path, Headers: map[string][]string{"upgrade": {"websocket"}},
	}
	channel.OnOpen(func() { send(client.tb, channel, head, nil) })
	var answered rtcapi.ResponseHead
	ready := make(chan struct{})
	go func() {
		answered = <-messages.head
		close(ready)
	}()
	receive(client.tb, ready, "the answer to the upgrade of "+path)
	client.tb.Cleanup(func() { _ = channel.Close() }) //nolint:errcheck // the test is over
	return messages, answered
}

// Send sends one message.
func (messages *Messages) Send(text string) {
	messages.tb.Helper()
	if err := messages.channel.SendText(text); err != nil {
		messages.tb.Fatal(err)
	}
}

// Next is the core's next message.
func (messages *Messages) Next() string {
	messages.tb.Helper()
	var next string
	got := make(chan struct{})
	go func() {
		next = <-messages.received
		close(got)
	}()
	receive(messages.tb, got, "the core's next message")
	return next
}

// Closed is the close the core sent.
func (messages *Messages) Closed() rtcapi.CloseMessage {
	messages.tb.Helper()
	var closing rtcapi.CloseMessage
	got := make(chan struct{})
	go func() {
		closing = <-messages.closed
		close(got)
	}()
	receive(messages.tb, got, "the core's close")
	return closing
}
