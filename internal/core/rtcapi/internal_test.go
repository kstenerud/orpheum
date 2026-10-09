// SPDX-License-Identifier: GPL-3.0-or-later

package rtcapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/testutil"
)

func TestAnswer_RefusesWhatIsNotAnOfferAndPastTheLimit(t *testing.T) {
	t.Parallel()
	server := New(t.Context(), http.NotFoundHandler(), slog.New(slog.DiscardHandler))
	if _, err := server.Answer(t.Context(), Description{Type: "answer", SDP: "v=0"}); err == nil {
		t.Error("an answer was taken as an offer")
	}
	server.mutex.Lock()
	for range MaxConnections {
		server.connections[&webrtc.PeerConnection{}] = true
	}
	server.mutex.Unlock()
	if _, err := server.Answer(t.Context(), Description{Type: "offer", SDP: "v=0"}); !errors.Is(err,
		ErrTooManyConnections) {
		t.Errorf("past %d connections, Answer = %v; want ErrTooManyConnections", MaxConnections, err)
	}
	server.mutex.Lock()
	server.connections = map[*webrtc.PeerConnection]bool{}
	server.mutex.Unlock()
}

func TestChannel_AHeadThatIsNotOneIsRefused(t *testing.T) {
	t.Parallel()
	reader := &requestReader{done: make(chan struct{})}
	reader.take(webrtc.DataChannelMessage{IsString: false, Data: []byte(`{"method":"GET","path":"/"}`)})
	if _, err := reader.request(t.Context(), ""); err == nil {
		t.Error("a binary head was taken")
	}
	reader = &requestReader{done: make(chan struct{})}
	reader.take(webrtc.DataChannelMessage{IsString: true, Data: []byte(`{"method":"GET","path":"http://x/"}`)})
	reader.take(webrtc.DataChannelMessage{})
	if _, err := reader.request(t.Context(), ""); err == nil {
		t.Error("a head naming another host was taken")
	}
	reader = &requestReader{done: make(chan struct{})}
	reader.take(webrtc.DataChannelMessage{IsString: true, Data: []byte(`{"method":"POST","path":"/"}`)})
	reader.take(webrtc.DataChannelMessage{Data: make([]byte, MaxRequestBodyBytes+1)})
	if _, err := reader.request(t.Context(), ""); err == nil {
		t.Errorf("a body over %d bytes was taken", MaxRequestBodyBytes)
	}
}

// A response sends as many messages as its client granted, then waits for the next grant: a film
// asked for from its first byte never sits whole in the browser (InitialCreditMessages).
func TestCredit_AResponseWaitsForTheClientsNextGrant(t *testing.T) {
	t.Parallel()
	granted := newCredit(2)
	for range 2 {
		if err := granted.take(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	taken := make(chan error, 1)
	go func() { taken <- granted.take(t.Context()) }()
	select {
	case err := <-taken:
		t.Fatalf("a third message was sent on a credit of two: %v", err)
	default:
	}
	granted.grant(1)
	if err := testutil.Receive(t, taken, "the message the grant allowed"); err != nil {
		t.Error(err)
	}
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	if err := granted.take(ended); !errors.Is(err, context.Canceled) {
		t.Errorf("with no credit and the response's context ended, take = %v; want it to stop waiting", err)
	}
}
