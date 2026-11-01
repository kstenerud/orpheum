// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// MaxRequestBodyBytes bounds a request's body over a channel: the API's bodies are small JSON.
const MaxRequestBodyBytes = 1 << 20

// The buffered bytes a response may queue on its channel before it waits, and the level it waits
// down to: pion counts the bytes in flight as buffered, so these bound a channel's bytes in flight.
// At 1 MiB and 256 KiB they held a paced channel to 42-59 Mb/s at 50 ms against headless Chromium;
// at 8 and 4 MiB it reached 141-165 (webrtc-throughput-review-2026-10-28.md §4.1, D397). They hold
// no film in memory: the client's credit (InitialCreditMessages, 4 MiB) binds first.
const (
	bufferedHighBytes = 8 << 20
	bufferedLowBytes  = 4 << 20
)

// RequestHead is a request's first message on its channel.
type RequestHead struct {
	Method string `json:"method"`
	// Path is the request's path and query, as an HTTP request line names them.
	Path    string      `json:"path"`
	Headers http.Header `json:"headers,omitempty"`
}

// InitialCreditMessages is how many body messages the core sends before the client grants more: 4 MiB
// in flight, enough to fill a fast link's window while the client reads, little enough that a film
// asked for from its first byte never sits whole in the browser's memory.
const InitialCreditMessages = 64

// Credit is a client's grant of more body messages, sent on the channel after its request ends, as
// its reader takes those it has: {"credit": n}.
type Credit struct {
	Messages int `json:"credit"`
}

// ResponseHead is a response's first message on its channel.
type ResponseHead struct {
	Status  int         `json:"status"`
	Headers http.Header `json:"headers,omitempty"`
}

// servedChannel is how a channel is served: the time its request has to arrive whole, and what is
// told when its request is done with, answered or its channel closed, whichever comes first.
type servedChannel struct {
	deadline time.Duration
	ended    func()
	// path is the path the channel's connection selected, which the request's context carries
	// (PathOf); nil carries none.
	path func() Path
	// ownNetworks are the core's own public IPv6 networks on the channel's connection, which the
	// request's context carries (OwnIPv6NetworksOf); nil carries none.
	ownNetworks func() []netip.Prefix
	// held is the channel's connection's record, which counts its requests and learns whose it is
	// (NoteClient); nil keeps none.
	held *heldConnection
	// now reads the clock a response's lingering channel is timed by, and answerLingers is how long
	// after the core's last byte for it its channel may stay open before its close is logged
	// (answerWatch); a nil now times none.
	now           func() time.Time
	answerLingers time.Duration
}

// serveChannel serves the one request a channel carries: its head, its body, an empty message ending
// it; then the response's head, its body in messages of at most MaxMessageBytes, and an empty message
// ending it, after which the client closes the channel. The client's address is what remote says.
// The request's context ends with the channel, or with base, the server's lifetime. A request that
// has not arrived whole by served.deadline closes its channel, so no one holds a request open by
// sending it slowly.
func serveChannel(
	base context.Context, handler http.Handler, channel *webrtc.DataChannel, remote func() string,
	logger *slog.Logger, served servedChannel,
) {
	ctx, cancel := context.WithCancel(base)
	conn := newMessageConn(channel)
	watch := newAnswerWatch(channel, served, logger)
	var endOnce sync.Once
	channel.OnClose(func() {
		// Before the context ends, which ends a wait for SCTP's buffer the watch may still report.
		watch.closed()
		conn.ended()
		cancel()
		endOnce.Do(served.ended)
	})
	credit := newCredit(InitialCreditMessages)
	reader := &requestReader{done: make(chan struct{}), credit: credit, conn: conn}
	channel.OnMessage(reader.take)
	go func() {
		defer cancel()
		defer endOnce.Do(served.ended)
		late := time.NewTimer(served.deadline)
		defer late.Stop()
		select {
		case <-reader.done:
		case <-ctx.Done():
			return
		case <-late.C:
			logger.WarnContext(ctx, "a request over WebRTC did not arrive in time", "deadline", served.deadline)
			_ = channel.Close() //nolint:errcheck // nothing more to say on it
			return
		}
		overWebRTC := context.WithValue(ctx, overWebRTCKey{}, true)
		if served.path != nil {
			overWebRTC = context.WithValue(overWebRTC, pathKey{}, served.path())
		}
		if served.ownNetworks != nil {
			overWebRTC = context.WithValue(overWebRTC, ownNetworksKey{}, served.ownNetworks())
		}
		overWebRTC = withHeld(overWebRTC, served.held)
		request, err := reader.request(overWebRTC, remote())
		if err != nil {
			logger.WarnContext(ctx, "a request over WebRTC was not one", "err", err)
			_ = channel.Close() //nolint:errcheck // nothing more to say on it
			return
		}
		watch.asked(request.URL.Path)
		if reader.isUpgrade {
			request = request.WithContext(context.WithValue(overWebRTC, messageConnKey{}, conn))
		}
		writer := newChannelWriter(ctx, channel, credit, watch)
		if !serveGuarded(ctx, handler, writer, request, logger) {
			_ = channel.Close() //nolint:errcheck // the response is cut short, and the client knows it so
			return
		}
		if conn.accepted() {
			_ = conn.CloseNow() //nolint:errcheck // the handler is done with it, closed or not
			return
		}
		if err := writer.finish(); err != nil && ctx.Err() == nil {
			logger.WarnContext(ctx, "a response over WebRTC was cut short", "path", request.URL.Path, "err", err)
		}
	}()
}

// requestReader gathers a request's messages until the empty one that ends it.
type requestReader struct {
	// credit is what the client has granted the response, which its messages after the request add to.
	credit *credit
	// conn takes the client's messages after a request that asked to become a WebSocket.
	conn      *MessageConn
	isUpgrade bool
	mutex     sync.Mutex
	head      *RequestHead
	body      bytes.Buffer
	failed    error
	isDone    bool
	done      chan struct{}
}

// take takes one message of the request.
func (reader *requestReader) take(message webrtc.DataChannelMessage) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	if reader.isDone && reader.isUpgrade {
		if message.IsString {
			reader.conn.take(message.Data)
		}
		return
	}
	if reader.isDone {
		var granted Credit
		if message.IsString && json.Unmarshal(message.Data, &granted) == nil && granted.Messages > 0 {
			reader.credit.grant(granted.Messages)
		}
		return
	}
	switch {
	case reader.head == nil:
		var head RequestHead
		if err := json.Unmarshal(message.Data, &head); err != nil || !message.IsString {
			reader.end(fmt.Errorf("the first message is not a request head: %w", errors.Join(err, errNotText(message))))
			return
		}
		reader.head = &head
		reader.isUpgrade = asksForUpgrade(head.Headers)
	case len(message.Data) == 0:
		reader.end(nil)
	case reader.body.Len()+len(message.Data) > MaxRequestBodyBytes:
		reader.end(fmt.Errorf("the request's body is over %d bytes", MaxRequestBodyBytes))
	default:
		_, _ = reader.body.Write(message.Data) // a bytes.Buffer's Write never returns an error
	}
}

// end ends the request, failed or whole. The caller holds the mutex.
func (reader *requestReader) end(failed error) {
	reader.failed, reader.isDone = failed, true
	close(reader.done)
}

// pathKey is the context key of the path a request's WebRTC connection selected.
type pathKey struct{}

// PathOf is the path the WebRTC connection a request came over selected, as the core's own end of
// it says: what decides whether the request is from home (D300 §3); PathFailed for any other request.
func PathOf(ctx context.Context) Path {
	path, isMarked := ctx.Value(pathKey{}).(Path)
	if !isMarked {
		return PathFailed
	}
	return path
}

// overWebRTCKey marks a request's context as one a data channel carried.
type overWebRTCKey struct{}

// IsOverWebRTC reports whether a request came over a data channel: its head is what the page wrote,
// so it says nothing of the page's origin (the security audit's A1).
func IsOverWebRTC(ctx context.Context) bool {
	isOver, isMarked := ctx.Value(overWebRTCKey{}).(bool)
	return isMarked && isOver
}

// isWhereFrom is whether a header names where a request came from: its origin, its host, or a
// proxy's word on its client, none of which a page's head is believed in.
func isWhereFrom(name string) bool {
	canonical := http.CanonicalHeaderKey(name)
	return canonical == "Origin" || canonical == "Host" || canonical == "Forwarded" ||
		strings.HasPrefix(canonical, "X-Forwarded-")
}

// errNotText is why a head that is not text is refused, or nil for one that is.
func errNotText(message webrtc.DataChannelMessage) error {
	if message.IsString {
		return nil
	}
	return errors.New("a head is a text message")
}

// request is the gathered request as the core's handler takes it, from remoteAddress.
func (reader *requestReader) request(ctx context.Context, remoteAddress string) (*http.Request, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	if reader.failed != nil {
		return nil, reader.failed
	}
	if reader.head.Method == "" || !strings.HasPrefix(reader.head.Path, "/") {
		return nil, fmt.Errorf("a request head names a method and a path from /, not %q %q",
			reader.head.Method, reader.head.Path)
	}
	target, err := url.ParseRequestURI(reader.head.Path)
	if err != nil {
		return nil, fmt.Errorf("the request's path: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, reader.head.Method, target.String(),
		bytes.NewReader(reader.body.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("make the request: %w", err)
	}
	// A browser names headers in lower case; the handler reads them by their canonical names. The
	// head is the page's, so none that names where the request came from is believed: a page could
	// claim the core's own origin (the security audit's A1).
	for name, values := range reader.head.Headers {
		if isWhereFrom(name) {
			continue
		}
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	request.RemoteAddr, request.RequestURI = remoteAddress, target.RequestURI()
	return request, nil
}

// serveGuarded runs handler on request, as net/http's server does, a panic ending this request alone:
// logged with its stack, answered 500 when nothing of the response was sent yet; it reports whether
// the response may be finished. One run on a WebRTC channel without it ended the core: huma panics
// when it cannot write a response, as when the client closed the channel first (staging,
// 2026-10-18, a page's GET /titles closed as it moved to another path).
func serveGuarded(
	ctx context.Context, handler http.Handler, writer *channelWriter, request *http.Request, logger *slog.Logger,
) (isWhole bool) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		if recovered != http.ErrAbortHandler { //nolint:errorlint,err113 // net/http's own sentinel, compared as it does
			logger.ErrorContext(ctx, "a request over WebRTC panicked", "path", request.URL.Path,
				"panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
		}
		isWhole = !writer.isHeadSent
		if isWhole {
			writer.headers = http.Header{}
			writer.status = http.StatusInternalServerError
		}
	}()
	handler.ServeHTTP(writer, request)
	return true
}
