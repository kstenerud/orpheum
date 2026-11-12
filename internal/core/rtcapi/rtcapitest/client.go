// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapitest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/pkg/strictjson"
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
	answerSDP  string
	// verdicts carries what the core says on the negotiation channel (rtcapi.Verdict).
	verdicts chan string
	// tracks counts the media tracks the core sent: none, ever.
	tracks *atomic.Int32
}

// Tracks is how many media tracks the core has sent the client.
func (client *Client) Tracks() int {
	return int(client.tracks.Load())
}

// Answerer takes the client's offer to the core and returns its answer: rtcapi.Server.Answer, or the
// offer endpoint.
type Answerer func(ctx context.Context, offer rtcapi.Description) (rtcapi.Description, error)

// Option changes how Connect's client gathers its candidates.
type Option func(*webrtc.SettingEngine)

// OverIPv4 holds the client to IPv4 candidates, for a test of which path a connection took: a
// machine with other addresses (a container network's IPv6, say) otherwise gave it an IPv6 pair,
// and the path depended on the machine, not the code.
func OverIPv4() Option {
	return func(settings *webrtc.SettingEngine) {
		settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	}
}

// OnLoopback holds the client to the loopback's addresses, for a test whose clients must be on the
// loopback and in one network: on a machine with container networks, each bridge gave the client
// more candidates (past the core's limit on a relayed offer's) and another network to connect from.
func OnLoopback() Option {
	return func(settings *webrtc.SettingEngine) {
		settings.SetIPFilter(func(ip net.IP) bool { return ip.IsLoopback() })
	}
}

// Connect makes a client on the loopback and connects it through answer.
func Connect(tb testing.TB, answer Answerer, options ...Option) *Client {
	tb.Helper()
	client, isConnected := Dial(tb, answer, waitLimit, options...)
	if !isConnected {
		tb.Fatalf("waited %s for the connection", waitLimit)
	}
	return client
}

// Dial makes a client on the loopback, passes its offer to answer, and returns it with whether it
// connected within wait: for a test of a connection the core should not let connect.
func Dial(tb testing.TB, answer Answerer, wait time.Duration, options ...Option) (*Client, bool) {
	tb.Helper()
	settings := webrtc.SettingEngine{}
	settings.SetIncludeLoopbackCandidate(true)
	for _, option := range options {
		option(&settings)
	}
	media := &webrtc.MediaEngine{}
	if err := media.RegisterDefaultCodecs(); err != nil {
		tb.Fatal(err)
	}
	connection, err := webrtc.NewAPI(webrtc.WithSettingEngine(settings), webrtc.WithMediaEngine(media)).
		NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = connection.Close() }) //nolint:errcheck // the test is over
	// An offer needs something to negotiate: a channel the client opens and sends nothing on, on which
	// the core tells its verdict; and, as the web client offers (offerWith), a video m-line that only
	// receives, for Chrome's socket buffer.
	hello, err := connection.CreateDataChannel(rtcapi.NegotiationLabel, nil)
	if err != nil {
		tb.Fatal(err)
	}
	if _, err = connection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		tb.Fatal(err)
	}
	var tracks atomic.Int32
	connection.OnTrack(func(*webrtc.TrackRemote, *webrtc.RTPReceiver) { tracks.Add(1) })
	verdicts := make(chan string, 1)
	hello.OnMessage(func(message webrtc.DataChannelMessage) {
		select {
		case verdicts <- string(message.Data):
		default:
		}
	})
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
	client := &Client{tb: tb, connection: connection, answerSDP: answered.SDP, verdicts: verdicts, tracks: &tracks}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-connected:
		return client, true
	case <-timer.C:
		return client, false
	}
}

// Verdict is what the core told the client on its negotiation channel, once it did.
func (client *Client) Verdict() rtcapi.Verdict {
	client.tb.Helper()
	ctx, cancel := context.WithTimeout(client.tb.Context(), waitLimit)
	defer cancel()
	var verdict rtcapi.Verdict
	select {
	case text := <-client.verdicts:
		if err := strictjson.Unmarshal([]byte(text), &verdict); err != nil {
			client.tb.Fatalf("the core's verdict %q is not one: %v", text, err)
		}
	case <-ctx.Done():
		client.tb.Fatalf("waited %s for the core's verdict", waitLimit)
	}
	return verdict
}

// Close closes the client's connection as Chrome closing its peer connection does: DTLS's
// close_notify, with no SCTP ABORT before it (headless Chromium against the core, 2026-11-01, D397).
func (client *Client) Close() {
	_ = client.connection.SCTP().Transport().Stop() //nolint:errcheck // closed either way
	_ = client.connection.Close()                   //nolint:errcheck // closed either way
}

// AnswerSDP is the core's answer, as the client took it.
func (client *Client) AnswerSDP() string {
	return client.answerSDP
}

// IsClosedWithin is whether the core closed the connection within wait.
func (client *Client) IsClosedWithin(wait time.Duration) bool {
	ended := make(chan struct{})
	var once sync.Once
	isEnded := func(state webrtc.PeerConnectionState) bool {
		return state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateDisconnected
	}
	client.connection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if isEnded(state) {
			once.Do(func() { close(ended) })
		}
	})
	if isEnded(client.connection.ConnectionState()) {
		return true
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ended:
		return true
	case <-timer.C:
		return false
	}
}

// Begin sends a request's head on a channel of its own, and its body and end when isWhole, and
// returns the channel without waiting for an answer: for a test of a request the core should cut off.
func (client *Client) Begin(head rtcapi.RequestHead, isWhole bool) *Channel {
	client.tb.Helper()
	channel, err := client.connection.CreateDataChannel("request", nil)
	if err != nil {
		client.tb.Fatal(err)
	}
	begun := &Channel{
		channel: channel, closed: make(chan struct{}), answered: make(chan struct{}), ended: make(chan struct{}),
	}
	var closeOnce, answerOnce, endOnce sync.Once
	channel.OnClose(func() { closeOnce.Do(func() { close(begun.closed) }) })
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		answerOnce.Do(func() { close(begun.answered) })
		if !message.IsString && len(message.Data) == 0 {
			endOnce.Do(func() { close(begun.ended) })
		}
	})
	channel.OnOpen(func() {
		var sendErr error
		if isWhole {
			sendErr = sendRequest(channel, head, nil)
		} else {
			encoded, encodeErr := json.Marshal(head)
			if sendErr = encodeErr; sendErr == nil {
				sendErr = channel.SendText(string(encoded))
			}
		}
		// A channel past the core's limit is closed as it opens, and a send may meet it closed: the
		// test reads that by IsClosedWithin, so it is no failure of the client's.
		if sendErr != nil && !errors.Is(sendErr, io.ErrClosedPipe) {
			client.tb.Error(sendErr)
		}
	})
	client.tb.Cleanup(func() { _ = channel.Close() }) //nolint:errcheck // the test is over
	return begun
}

// Channel is a request Begin sent.
type Channel struct {
	channel                 *webrtc.DataChannel
	closed, answered, ended chan struct{}
}

// Stream is the channel's SCTP stream id, as the core's log names it, or -1 before it has one.
func (channel *Channel) Stream() int {
	if id := channel.channel.ID(); id != nil {
		return int(*id)
	}
	return -1
}

// IsEndedWithin is whether the core's whole answer, its empty end message too, came within wait.
func (channel *Channel) IsEndedWithin(wait time.Duration) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-channel.ended:
		return true
	case <-timer.C:
		return false
	}
}

// Close closes the channel from the client's end, as a browser does with a request its caller
// aborted.
func (channel *Channel) Close() {
	_ = channel.channel.Close() //nolint:errcheck // a channel already closed is as good
}

// IsAnsweredWithin is whether the core began its answer within wait.
func (channel *Channel) IsAnsweredWithin(wait time.Duration) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-channel.answered:
		return true
	case <-timer.C:
		return false
	}
}

// IsClosedWithin is whether the core closed the channel within wait, without a word of answer.
func (channel *Channel) IsClosedWithin(wait time.Duration) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-channel.closed:
		select {
		case <-channel.answered:
			return false
		default:
			return true
		}
	case <-timer.C:
		return false
	}
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
			if err := strictjson.Unmarshal(message.Data, &got.Head); err != nil {
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

// DoAsChrome90 sends one request on a channel of its own as Chrome before 92 can, and reads its
// response whole: no empty message ends it, its head declaring its body's length and asking for its
// response to end in rtcapi.EndMessage, and an empty message the core sends fails the test, as that
// Chrome never receives one (idiosyncrasies.md).
func (client *Client) DoAsChrome90(head rtcapi.RequestHead, body []byte) Reply {
	client.tb.Helper()
	channel, err := client.connection.CreateDataChannel("request", nil)
	if err != nil {
		client.tb.Fatal(err)
	}
	asked := rtcapi.RequestHead{Method: head.Method, Path: head.Path, Headers: head.Headers.Clone()}
	if asked.Headers == nil {
		asked.Headers = http.Header{}
	}
	asked.Headers["content-length"] = []string{strconv.Itoa(len(body))}
	asked.Headers[strings.ToLower(rtcapi.EndHeader)] = []string{rtcapi.EndInText}
	var got Reply
	read := 0
	isHeadRead := false
	done := make(chan struct{})
	var once sync.Once
	channel.OnMessage(func(message webrtc.DataChannelMessage) {
		switch {
		case message.IsString && isHeadRead && string(message.Data) == rtcapi.EndMessage:
			once.Do(func() { close(done) })
		case message.IsString:
			isHeadRead = true
			if err := strictjson.Unmarshal(message.Data, &got.Head); err != nil {
				client.tb.Error(err)
			}
		case len(message.Data) == 0:
			client.tb.Error("the core sent an empty message, which Chrome before 92 never receives")
		default:
			got.Body = append(got.Body, message.Data...)
			read++
			if read%grantEvery == 0 {
				grant(client.tb, channel, grantEvery)
			}
		}
	})
	channel.OnOpen(func() {
		encoded, err := json.Marshal(asked)
		if err == nil {
			err = channel.SendText(string(encoded))
		}
		for start := 0; err == nil && start < len(body); start += rtcapi.MaxMessageBytes {
			err = channel.Send(body[start:min(len(body), start+rtcapi.MaxMessageBytes)])
		}
		if err != nil {
			client.tb.Error(err)
		}
	})
	receive(client.tb, done, "the response to "+head.Path)
	_ = channel.Close() //nolint:errcheck // the response is read
	return got
}

// send sends a request's head, its body and the empty message that ends it.
func send(tb testing.TB, channel *webrtc.DataChannel, head rtcapi.RequestHead, body []byte) {
	tb.Helper()
	if err := sendRequest(channel, head, body); err != nil {
		tb.Error(err)
	}
}

// sendRequest sends head, then body in messages no larger than rtcapi.MaxMessageBytes, then the
// empty message that ends the request.
func sendRequest(channel *webrtc.DataChannel, head rtcapi.RequestHead, body []byte) error {
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
		return fmt.Errorf("send a request: %w", err)
	}
	return nil
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
			if err := strictjson.Unmarshal(message.Data, &head); err != nil {
				client.tb.Error(err)
			}
			messages.head <- head
		case message.IsString:
			messages.received <- string(message.Data)
		default:
			var closing rtcapi.CloseMessage
			if err := strictjson.Unmarshal(message.Data, &closing); err != nil {
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
