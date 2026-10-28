// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/core/together"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// togetherSocketPath is the session WebSocket's path (cut-a §5.1): one per device.
const togetherSocketPath = Prefix + "/together/ws"

// helloWait is how long a device has to say hello once its socket is open: a second's work on any
// network, with room for a slow phone.
const helloWait = 10 * time.Second

// maxTogetherMessageBytes bounds one message a device sends: the largest is voice signalling's SDP,
// a few kilobytes.
const maxTogetherMessageBytes = 64 << 10

// clientPingEvery is how often a device pings while it is attached: the web client's pingEveryMs
// (web/packages/sdk/src/together-connection.ts), which TestTogether_TheSilenceLimitIsThreeOfTheClientsPings
// holds equal.
const clientPingEvery = 15 * time.Second

// silenceLimit is how long a device may send nothing before it is taken for gone: three missed pings.
// A connection cut without a close, as a phone's on an airport's network, otherwise stays attached
// until the operating system gives up on it, many minutes later (staging, 2026-10-06).
const silenceLimit = 3 * clientPingEvery

// errSilent is the cause a read's context ends with when the device stayed silent.
var errSilent = errors.New("the device stayed silent")

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
	connection, err := acceptTogetherSocket(writer, request, server.clientOriginOf())
	if err != nil {
		return // the refusal is answered
	}
	defer func() { _ = connection.closeNow() }() //nolint:errcheck // closing what may be closed
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	hello, isSession := server.sessionHello(ctx, connection, dependencies, request)
	if !isSession {
		return
	}
	person, err := signedInPerson(ctx, dependencies, hello.SessionToken)
	if err != nil {
		// Could not be checked: the server's failure, and the device comes again (the M4 closing audit).
		server.logger.ErrorContext(ctx, "watch together: a device's sign-in could not be checked", "err", err)
		_ = connection.closeWith(websocket.StatusInternalError, "try again") //nolint:errcheck // closing
		return
	}
	outbox := make(chan together.Outbound, outboxSize)
	var closing sync.Once
	send := func(message together.Outbound) {
		select {
		case outbox <- message:
		default:
			closing.Do(func() {
				server.logger.WarnContext(ctx, "watch together: a device fell too far behind; closing it")
				cancel()
			}) // too far behind: it reads nothing
		}
	}
	attachment, isAttached := server.attachDevice(ctx, connection, dependencies, hello, person, send)
	if !isAttached {
		return
	}
	device := []any{
		"sessionId", attachment.SessionID, "memberId", attachment.MemberID, "deviceId", attachment.DeviceID,
		"role", string(hello.Role), "client", clientScriptOf(hello.Client),
	}
	server.logger.InfoContext(ctx, "watch together: a device connected",
		append(device, slogx.UserAgent(request))...)
	written := make(chan struct{})
	go func() {
		defer close(written)
		writeOutbox(ctx, connection, outbox, cancel)
	}()
	ended := readInto(ctx, connection, dependencies.Together, attachment, time.Duration(server.togetherSilence.Load()))
	gone := together.GoneClosed
	if errors.Is(ended, errSilent) {
		gone = together.GoneSilent
	}
	dependencies.Together.Detach(attachment, gone) // nothing is sent to it after this
	server.logger.InfoContext(ctx, "watch together: a device disconnected", append(device, "why", ended.Error())...)
	close(outbox)
	<-written
}

// attachDevice attaches the device that said hello, welcomed with how to follow its group as the
// settings say now; when it cannot, it closes the socket saying why and reports false.
func (server *Server) attachDevice(
	ctx context.Context, connection togetherSocket, dependencies *Dependencies, hello together.Inbound,
	person *together.Person, send together.Sender,
) (together.Attachment, bool) {
	tuning, err := server.togetherTuning(ctx, dependencies)
	if err != nil {
		const why = "the settings could not be read"
		_ = connection.closeWith(websocket.StatusInternalError, why) //nolint:errcheck // closing
		return together.Attachment{}, false
	}
	attachment, err := dependencies.Together.Attach(
		hello.Credential, hello.Role, hello.CanSetRate, person, tuning, send)
	if err != nil {
		server.logger.InfoContext(ctx, "watch together: a device's hello named no session or member",
			"role", string(hello.Role), "err", err)
		_ = connection.closeWith(websocket.StatusPolicyViolation, "no such session") //nolint:errcheck // closing
		return together.Attachment{}, false
	}
	return attachment, true
}

// togetherTuning is how a device follows its group, as the settings say now (D250 §1).
func (server *Server) togetherTuning(ctx context.Context, dependencies *Dependencies) (together.Tuning, error) {
	var tuning together.Tuning
	for _, read := range []struct {
		key  settings.Key
		into *int64
	}{
		{settings.TogetherDriftLeftAloneMs, &tuning.LeaveDriftUpToMs},
		{settings.TogetherNudgeMostPercent, &tuning.NudgeMostPercent},
		{settings.TogetherSeekDriftBeyondMs, &tuning.SeekDriftBeyondMs},
		{settings.TogetherSeekOnlyDriftLeftAloneMs, &tuning.SeekOnlyLeaveDriftUpToMs},
		{settings.TogetherCatchUpBufferPercent, &tuning.CatchUpBufferPercent},
		{settings.TogetherSettleAfterChangeMs, &tuning.SettleAfterChangeMs},
	} {
		value, err := dependencies.Settings.Int(ctx, read.key)
		if err != nil {
			return together.Tuning{}, server.settingsError(ctx, "read "+string(read.key), err)
		}
		*read.into = value
	}
	snapKey := settings.TogetherSnapSeeksBeyondSeconds
	snapBeyond, err := dependencies.Settings.Duration(ctx, snapKey)
	if err != nil {
		return together.Tuning{}, server.settingsError(ctx, "read "+string(snapKey), err)
	}
	tuning.SnapSeeksBeyondMs = snapBeyond.Milliseconds()
	for _, read := range []struct {
		key  settings.Key
		into *bool
	}{
		{settings.ShouldTogetherNudgeKeepPitch, &tuning.ShouldNudgeKeepPitch},
		{settings.ShouldTogetherVoiceCancelEcho, &tuning.ShouldCancelMicrophoneEcho},
		{settings.ShouldTogetherVoiceBrowserLevelMicrophone, &tuning.ShouldBrowserLevelMicrophone},
	} {
		value, err := dependencies.Settings.Bool(ctx, read.key)
		if err != nil {
			return together.Tuning{}, server.settingsError(ctx, "read "+string(read.key), err)
		}
		*read.into = value
	}
	return tuning, nil
}

// sessionHello reads a device's first message: a session's hello, which it returns; else, having
// served the live connection a live hello opens, or closed the socket for want of a hello, false.
func (server *Server) sessionHello(
	ctx context.Context, connection togetherSocket, dependencies *Dependencies, request *http.Request,
) (together.Inbound, bool) {
	first, err := readFirst(ctx, connection)
	if err == nil && isLiveHello(first) {
		server.serveLiveSocket(ctx, connection, dependencies, first, request)
		return together.Inbound{}, false
	}
	hello, err := togetherHelloOf(first, err)
	if err != nil {
		_ = connection.closeWith(websocket.StatusPolicyViolation, "say hello first") //nolint:errcheck // closing
		return hello, false
	}
	return hello, true
}

// readFirst reads a device's first message, within helloWait: a session's hello, or a live one.
func readFirst(ctx context.Context, connection togetherSocket) (json.RawMessage, error) {
	helloContext, cancel := context.WithTimeout(ctx, helloWait)
	defer cancel()
	var first json.RawMessage
	if err := connection.readJSON(helloContext, &first); err != nil {
		return nil, err
	}
	return first, nil
}

// togetherHelloOf is a device's first message, read with err, as a session's hello, which must
// carry a credential.
func togetherHelloOf(first json.RawMessage, err error) (together.Inbound, error) {
	var hello together.Inbound
	if err != nil {
		return hello, err
	}
	if err := json.Unmarshal(first, &hello); err != nil {
		return hello, fmt.Errorf("the first message is not JSON: %w", err)
	}
	if hello.Type != together.MessageHello || hello.Credential == "" {
		return hello, errors.New("the first message is not a hello with a credential")
	}
	return hello, nil
}

// readInto hands each message the device sends to the hub, until the socket or ctx ends or the
// device sends nothing for silence, and says why it ended: the close the device sent, the
// connection lost, its silence, or the server closing it.
func readInto(
	ctx context.Context, connection togetherSocket, hub *together.Hub, attachment together.Attachment,
	silence time.Duration,
) error {
	for {
		var message together.Inbound
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
		hub.Handle(ctx, attachment, message)
	}
}

// writeOutbox writes the hub's messages to the device until the outbox closes; an ended session's
// last message closes the socket after it.
func writeOutbox(
	ctx context.Context, connection togetherSocket, outbox <-chan together.Outbound, cancel context.CancelFunc,
) {
	for message := range outbox {
		if err := connection.writeJSON(ctx, message); err != nil {
			cancel()
			continue // drain, so the hub's sends never block
		}
		if message.Type == together.MessageEnded {
			_ = connection.closeWith(websocket.StatusNormalClosure, message.Reason) //nolint:errcheck // closing
			cancel()
		}
	}
}

// togetherSocket is a device's connection to its session: a WebSocket, or a request over a WebRTC
// data channel that asked to become one (D273 §8), each message one JSON object.
type togetherSocket interface {
	readJSON(ctx context.Context, into any) error
	writeJSON(ctx context.Context, value any) error
	closeWith(code websocket.StatusCode, reason string) error
	closeNow() error
}

// acceptTogetherSocket accepts a device's connection: the message connection a request over a data
// channel asked to become, else a WebSocket from the core's own page or from the client the core's
// discovery serves, clientOrigin, never another (the discovery protocol's §3.4; discovery-core.md
// K10). A refusal is answered by the time it returns.
func acceptTogetherSocket(
	writer http.ResponseWriter, request *http.Request, clientOrigin string,
) (togetherSocket, error) {
	if conn, isChannel := rtcapi.MessageConnFrom(request.Context()); isChannel {
		if err := conn.Accept(); err != nil {
			return nil, err //nolint:wrapcheck // the channel's own words; nothing reads them
		}
		return channelSocket{conn: conn}, nil
	}
	options := &websocket.AcceptOptions{}
	if origin, err := url.Parse(clientOrigin); clientOrigin != "" && err == nil && origin.Host != "" {
		options.OriginPatterns = []string{origin.Host}
	}
	connection, err := websocket.Accept(writer, request, options)
	if err != nil {
		return nil, err //nolint:wrapcheck // Accept has answered
	}
	connection.SetReadLimit(maxTogetherMessageBytes)
	return webSocket{conn: connection}, nil
}

// webSocket is a device's WebSocket.
type webSocket struct{ conn *websocket.Conn }

func (socket webSocket) readJSON(ctx context.Context, into any) error {
	return wsjson.Read(ctx, socket.conn, into) //nolint:wrapcheck // the socket's own words are the why
}

func (socket webSocket) writeJSON(ctx context.Context, value any) error {
	return wsjson.Write(ctx, socket.conn, value) //nolint:wrapcheck // the socket's own words
}

func (socket webSocket) closeWith(code websocket.StatusCode, reason string) error {
	return socket.conn.Close(code, reason) //nolint:wrapcheck // closing
}

func (socket webSocket) closeNow() error {
	return socket.conn.CloseNow() //nolint:wrapcheck // closing
}

// channelSocket is a device's connection over a data channel: a message is at most
// rtcapi.MaxMessageBytes, as maxTogetherMessageBytes holds a WebSocket's to.
type channelSocket struct{ conn *rtcapi.MessageConn }

func (socket channelSocket) readJSON(ctx context.Context, into any) error {
	message, err := socket.conn.Read(ctx)
	if err != nil {
		return err //nolint:wrapcheck // the channel's own words are the why
	}
	return json.Unmarshal(message, into) //nolint:wrapcheck // not JSON: the device's fault, said as it is
}

func (socket channelSocket) writeJSON(ctx context.Context, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode a message: %w", err)
	}
	return socket.conn.Write(ctx, encoded) //nolint:wrapcheck // the channel's own words
}

func (socket channelSocket) closeWith(code websocket.StatusCode, reason string) error {
	return socket.conn.Close(int(code), reason) //nolint:wrapcheck // closing
}

func (socket channelSocket) closeNow() error {
	return socket.conn.CloseNow() //nolint:wrapcheck // closing
}

// clientScriptMost is the most of a hello's client script logged: a built one's name is some 20
// characters, and anything longer is not one.
const clientScriptMost = 64

// clientScriptOf is the client script a hello named, as one line no longer than clientScriptMost.
func clientScriptOf(said string) string {
	line := textindex.Line(said)
	if len(line) > clientScriptMost {
		return line[:clientScriptMost]
	}
	return line
}
