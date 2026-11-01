// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// heldConnection is what the log says of one connection the core holds: a number to know it by
// across lines, since when, how many requests it carried, and whose it is once a request of it
// was signed in (NoteClient). The owner's network was turned away past its share of 16 with four
// browsers open (staging, 2026-10-25), and nothing said whose the 16 were.
type heldConnection struct {
	number   int64
	openedAt time.Time
	requests atomic.Int64
	client   atomic.Pointer[string]
	// streamClosedAt is when the last channel of each SCTP stream closed, for a channel that reuses
	// the stream to say how soon (answerWatch).
	streamsMutex   sync.Mutex
	streamClosedAt map[uint16]time.Time
	// end is what its end line needs, and whether it was said (logEnded).
	end connectionEnd
}

// newHeldConnection is the record of the connection numbered number, opened at at.
func newHeldConnection(number int64, at time.Time) *heldConnection {
	return &heldConnection{number: number, openedAt: at}
}

// name is how the log names the connection: its number, and whose it is where known.
func (held *heldConnection) name() string {
	if client := held.client.Load(); client != nil {
		return fmt.Sprintf("c%d (%s)", held.number, *client)
	}
	return fmt.Sprintf("c%d (no sign-in yet)", held.number)
}

// streamClosed notes that the channel of stream id closed at at.
func (held *heldConnection) streamClosed(id uint16, at time.Time) {
	held.streamsMutex.Lock()
	defer held.streamsMutex.Unlock()
	if held.streamClosedAt == nil {
		held.streamClosedAt = map[uint16]time.Time{}
	}
	held.streamClosedAt[id] = at
}

// sinceStreamClosed is how long before at the last channel of stream id closed, and whether one did.
func (held *heldConnection) sinceStreamClosed(id uint16, at time.Time) (time.Duration, bool) {
	held.streamsMutex.Lock()
	defer held.streamsMutex.Unlock()
	closedAt, wasClosed := held.streamClosedAt[id]
	return at.Sub(closedAt), wasClosed
}

// heldKey carries a request's connection record in its context.
type heldKey struct{}

// withHeld marks ctx as a request on held's connection, and counts the request.
func withHeld(ctx context.Context, held *heldConnection) context.Context {
	if held == nil {
		return ctx
	}
	held.requests.Add(1)
	return context.WithValue(ctx, heldKey{}, held)
}

// NoteClient names whose connection a request over WebRTC came on, the first time a request of it
// is signed in, as "user 1 device 18": the core's handler calls it as it authenticates. A request
// over plain HTTP, or on a connection already named, changes nothing.
func NoteClient(ctx context.Context, who string) {
	held, isHeld := ctx.Value(heldKey{}).(*heldConnection)
	if !isHeld || held == nil {
		return
	}
	held.client.CompareAndSwap(nil, &who)
}

// namesOf is the log's line of connections: each name, sorted by number.
func namesOf(held []*heldConnection) string {
	sorted := slices.Clone(held)
	slices.SortFunc(sorted, func(first, second *heldConnection) int {
		return int(first.number - second.number)
	})
	names := make([]string, 0, len(sorted))
	for _, each := range sorted {
		names = append(names, each.name())
	}
	return strings.Join(names, ", ")
}
