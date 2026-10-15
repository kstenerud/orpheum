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
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// MaxRequestBodyBytes bounds a request's body over a channel: the API's bodies are small JSON.
const MaxRequestBodyBytes = 1 << 20

// The buffered bytes a response may queue on its channel before it waits, and the level it waits
// down to: enough in flight to fill a fast link's window, little enough to hold no film in memory.
const (
	bufferedHighBytes = 1 << 20
	bufferedLowBytes  = 256 << 10
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
	var endOnce sync.Once
	channel.OnClose(func() {
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
		request, err := reader.request(overWebRTC, remote())
		if err != nil {
			logger.WarnContext(ctx, "a request over WebRTC was not one", "err", err)
			_ = channel.Close() //nolint:errcheck // nothing more to say on it
			return
		}
		if reader.isUpgrade {
			request = request.WithContext(context.WithValue(overWebRTC, messageConnKey{}, conn))
		}
		writer := newChannelWriter(ctx, channel, credit)
		handler.ServeHTTP(writer, request)
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
		reader.body.Write(message.Data)
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
