// SPDX-License-Identifier: GPL-3.0-or-later

package rtcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"
)

// MaxMessageBytes is the most a message on a channel holds, which every browser carries.
const MaxMessageBytes = 64 << 10

// MaxConnections bounds the peer connections the core holds at once: a client opens a few
// (ClientConnections), and a household has a handful of people and devices.
const MaxConnections = 64

// ClientConnections is how many peer connections a client opens and spreads its requests across:
// one is bound by its congestion window, and four carried three times as much at a 50 ms round
// trip (reachability-approaches-2026-10-09.md §9.4).
const ClientConnections = 4

// cwndStepBytes is how fast pion grows a connection's congestion window in congestion avoidance:
// 64 KiB took a 50 ms round trip from 15 to 20 Mbit/s on one connection (§9.4).
const cwndStepBytes = 64 << 10

// gatherTimeout bounds how long an answer waits for the core's candidates.
const gatherTimeout = 5 * time.Second

// ErrTooManyConnections is an offer refused because the core holds MaxConnections already.
var ErrTooManyConnections = errors.New("the core holds as many WebRTC connections as it allows")

// Description is a session description as the offer endpoint carries it: "offer" or "answer", and
// the SDP.
type Description struct {
	Type string
	SDP  string
}

// Server answers WebRTC offers, and hands each request a client sends over a data channel to the
// core's own handler (webrtc-transport.md).
type Server struct {
	// ctx is the server's lifetime, which every request's context derives from.
	ctx     context.Context //nolint:containedctx // the lifetime of the connections it holds
	handler http.Handler
	api     *webrtc.API
	logger  *slog.Logger

	// iceServers are the STUN and TURN servers a connection gathers its candidates through: its
	// discovery's (D274); none gathers only the core's own addresses.
	iceServers atomic.Pointer[func() []webrtc.ICEServer]

	mutex sync.Mutex
	// Guarded by mutex: connections are the peer connections held now, and paths how many
	// connections took each path.
	connections map[*webrtc.PeerConnection]bool
	paths       map[Path]int
}

// New returns a server whose channels' requests handler serves, until ctx ends.
func New(ctx context.Context, handler http.Handler, logger *slog.Logger) *Server {
	settings := webrtc.SettingEngine{}
	settings.SetSCTPCwndCAStep(cwndStepBytes)
	return &Server{
		ctx: ctx, handler: handler, api: webrtc.NewAPI(webrtc.WithSettingEngine(settings)), logger: logger,
		connections: map[*webrtc.PeerConnection]bool{}, paths: map[Path]int{},
	}
}

// UseICEServers has each new connection gather its candidates through the servers source names
// then: the discovery's STUN, so an answer carries the public address the core's NAT gives it.
func (server *Server) UseICEServers(source func() []webrtc.ICEServer) {
	server.iceServers.Store(&source)
}

// Answer takes a client's offer and returns the core's answer, with every candidate gathered. The
// connection lives until either side closes it or it fails.
func (server *Server) Answer(ctx context.Context, offer Description) (Description, error) {
	if offer.Type != webrtc.SDPTypeOffer.String() {
		return Description{}, fmt.Errorf("want an offer, not %q", offer.Type)
	}
	connection, err := server.open()
	if err != nil {
		return Description{}, err
	}
	answer, err := server.answer(ctx, connection, offer.SDP)
	if err != nil {
		server.forget(connection)
		return Description{}, errors.Join(err, connection.Close())
	}
	return answer, nil
}

// open makes a peer connection, unless the core holds MaxConnections.
func (server *Server) open() (*webrtc.PeerConnection, error) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if len(server.connections) >= MaxConnections {
		return nil, ErrTooManyConnections
	}
	configuration := webrtc.Configuration{}
	if source := server.iceServers.Load(); source != nil {
		configuration.ICEServers = (*source)()
	}
	connection, err := server.api.NewPeerConnection(configuration)
	if err != nil {
		return nil, fmt.Errorf("make a peer connection: %w", err)
	}
	server.connections[connection] = true
	var isConnected atomic.Bool
	connection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected && isConnected.CompareAndSwap(false, true) {
			server.recordPath(connection, selectedPath(connection))
		}
		if state == webrtc.PeerConnectionStateFailed && !isConnected.Load() {
			server.recordPath(connection, PathFailed)
		}
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			server.forget(connection)
			_ = connection.Close() //nolint:errcheck // a failed or closed connection, gone either way
		}
	})
	connection.OnDataChannel(func(channel *webrtc.DataChannel) {
		remote := func() string { return remoteAddress(connection) }
		serveChannel(server.ctx, server.handler, channel, remote, server.logger)
	})
	return connection, nil
}

// answer sets the offer and returns the answer once the core's candidates are gathered.
func (server *Server) answer(
	ctx context.Context, connection *webrtc.PeerConnection, offerSDP string,
) (Description, error) {
	err := connection.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP})
	if err != nil {
		return Description{}, fmt.Errorf("take the offer: %w", err)
	}
	answer, err := connection.CreateAnswer(nil)
	if err != nil {
		return Description{}, fmt.Errorf("make the answer: %w", err)
	}
	gathered := webrtc.GatheringCompletePromise(connection)
	if err = connection.SetLocalDescription(answer); err != nil {
		return Description{}, fmt.Errorf("set the answer: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, gatherTimeout)
	defer cancel()
	select {
	case <-gathered:
	case <-waitCtx.Done():
		return Description{}, fmt.Errorf("gather the core's candidates: %w", waitCtx.Err())
	}
	local := connection.LocalDescription()
	return Description{Type: local.Type.String(), SDP: local.SDP}, nil
}

// forget drops a connection from those held.
func (server *Server) forget(connection *webrtc.PeerConnection) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	delete(server.connections, connection)
}

// Close closes every connection held.
func (server *Server) Close() error {
	server.mutex.Lock()
	held := make([]*webrtc.PeerConnection, 0, len(server.connections))
	for connection := range server.connections {
		held = append(held, connection)
	}
	server.connections = map[*webrtc.PeerConnection]bool{}
	server.mutex.Unlock()
	var errs []error
	for _, connection := range held {
		errs = append(errs, connection.Close())
	}
	return errors.Join(errs...)
}

// remoteAddress is the address of the client's end of the candidate pair the connection selected, as
// a request's remote address says it: what rate limits and link speeds are kept by (D257).
func remoteAddress(connection *webrtc.PeerConnection) string {
	sctp := connection.SCTP()
	if sctp == nil {
		return ""
	}
	pair, err := sctp.Transport().ICETransport().GetSelectedCandidatePair()
	if err != nil || pair == nil || pair.Remote == nil {
		return ""
	}
	return net.JoinHostPort(pair.Remote.Address, strconv.Itoa(int(pair.Remote.Port)))
}
