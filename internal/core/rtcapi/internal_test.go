// SPDX-License-Identifier: GPL-3.0-or-later

package rtcapi

import (
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/pion/webrtc/v4"
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
