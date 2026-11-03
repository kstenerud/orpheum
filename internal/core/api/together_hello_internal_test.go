// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/coder/websocket"
)

// silentSocket is a device's socket that never says anything; it notes how it was closed.
type silentSocket struct {
	closedWith websocket.StatusCode
}

func (socket *silentSocket) readJSON(ctx context.Context, _ any) error {
	<-ctx.Done()
	return fmt.Errorf("read: %w", ctx.Err())
}

func (*silentSocket) writeJSON(context.Context, any) error { return nil }

func (socket *silentSocket) closeWith(code websocket.StatusCode, _ string) error {
	socket.closedWith = code
	return nil
}

func (*silentSocket) closeNow() error { return nil }

// A device whose hello does not come in time is told to try again, not refused: a refusal (1008)
// ends its live connection, or its place in a session, until a reload (principles §28).
func TestSessionHello_NoHelloInTimeIsCloseToTryAgainNotARefusal(t *testing.T) {
	t.Parallel()
	server := NewServer(slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // the wait for a hello is over at once
	socket := &silentSocket{}
	if _, isHello := server.sessionHello(ctx, socket, nil, nil); isHello {
		t.Fatal("a socket that said nothing was taken as a hello")
	}
	if socket.closedWith != websocket.StatusTryAgainLater {
		t.Fatalf("closed with %d, want %d (try again later)", socket.closedWith, websocket.StatusTryAgainLater)
	}
}
