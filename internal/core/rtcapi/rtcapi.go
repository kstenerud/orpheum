// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/platform/clock"
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

// NegotiationLabel is the channel a client opens only so its offer has something to negotiate
// (web/packages/sdk's rtc-connect.ts, rtcapitest): it carries no request, so it is never served,
// takes no request's place, and is not logged as one that did not arrive.
const NegotiationLabel = "hello"

// gatherTimeout bounds how long an answer waits for the core's candidates.
const gatherTimeout = 5 * time.Second

// stunGatherTimeout bounds how long a STUN server is given to answer while the core gathers, well
// within gatherTimeout: a network with no way out by one family, as a container with an IPv6 route
// and no IPv6 beyond its host, never answers, and pion's own 5 s wait took the whole answer's time
// (staging, 2026-10-14).
const stunGatherTimeout = 2 * time.Second

// gatherAfterReflexive is how long an answer waits for more candidates after the core's first that
// reaches past its own machine, a server-reflexive or a relayed one: a STUN server it cannot reach by
// one family still holds gathering for stunGatherTimeout, and every answer on staging took 2.1 s for
// it (2026-10-23), a sign-in's two in turn. The page's offer waits the same way (web/packages/sdk's
// gatherAfterReflexiveMs).
const gatherAfterReflexive = 300 * time.Millisecond

// The bounds on each connection (the protocol's §2.5; the security audit's A2, code K16):
//   - connectWithin: an answered offer whose ICE has not connected by then is closed, so no one holds
//     the core's slots with offers that never connect;
//   - perSourceLimit: the connections one network holds at once (an IPv4 address, or an IPv6 /56, a
//     household's usual delegation), a few households' clients behind one address;
//   - channelLimit: the requests in flight on one connection, each a channel, its body bounded by
//     MaxRequestBodyBytes;
//   - requestDeadline: how long a request's head and body may take to arrive whole;
//   - sctpReceiveBytes: the most SCTP buffers of what a client sends, which is requests' small bodies.
const (
	connectWithin    = 10 * time.Second
	perSourceLimit   = 16
	channelLimit     = 16
	requestDeadline  = 30 * time.Second
	sctpReceiveBytes = 1 << 20
	sourceIPv6Prefix = 56
)

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
	// api answers the core's own offer endpoint; relayedAPI, the offers a discovery passes on, whose
	// connections check only remote addresses from outside the core's network.
	api        *webrtc.API
	relayedAPI *webrtc.API
	logger     *slog.Logger
	// clock times each connection from its offer to its connecting, for the log.
	clock clock.Clock
	// bounds are the bounds on each connection: the constants above, shorter in the tests.
	bounds bounds

	// iceServers are the STUN and TURN servers a connection gathers its candidates through: its
	// discovery's (D274); none gathers only the core's own addresses.
	iceServers atomic.Pointer[func() []webrtc.ICEServer]
	// anyRelayedAddress lets a relayed connection check any remote address, the core's own networks'
	// too: for a development build alone, whose browser tests put the browser, the discovery and the
	// core on one machine (AcceptAnyRelayedAddress).
	anyRelayedAddress atomic.Bool
	// homeNetworks are the house's networks (SetHomeNetworks); none until named.
	homeNetworks atomic.Pointer[[]netip.Prefix]

	mutex sync.Mutex
	// Guarded by mutex: connections are the peer connections held now, and paths how many
	// connections took each path.
	connections map[*webrtc.PeerConnection]bool
	paths       map[Path]int
	// sources are the networks connected clients are counted under, by their connections.
	sources map[*webrtc.PeerConnection]netip.Prefix
}

// bounds are the bounds on each connection.
type bounds struct {
	connectWithin   time.Duration
	perSource       int
	channels        int
	requestDeadline time.Duration
}

// AcceptAnyRelayedAddress lets the connections a discovery's offers make check any remote address,
// the core's own networks' too, which a release refuses so that no one holding a link can turn the
// core on its own network. Only a development build calls it (cmd/orpheum's build_dev.go), whose
// browser tests run the browser, the discovery and the core on one machine.
func (server *Server) AcceptAnyRelayedAddress() {
	server.anyRelayedAddress.Store(true)
}

// checksRelayed is whether a connection a discovery's offer made checks a remote address: one from
// outside the core's networks, one in its home networks, or any after AcceptAnyRelayedAddress. A
// home address an offer names never reaches here (takesNamedRelayed): one is checked only as the
// address a check reached the core from, sent by a device in the house (D300 §2).
func (server *Server) checksRelayed(ip net.IP) bool {
	return server.takesNamedRelayed(ip) || server.isHomeIP(ip)
}

// takesNamedRelayed is whether an address an offer a discovery passed on names is kept: one from
// outside the core's networks, or any after AcceptAnyRelayedAddress (the security audit's A2).
func (server *Server) takesNamedRelayed(ip net.IP) bool {
	return server.anyRelayedAddress.Load() || isFromOutside(ip, interfacePrefixes)
}

// SetHomeNetworks names the house's networks (server.home_networks): a relayed connection answers a
// check from one, and a request over a connection whose selected pair reaches one directly is from
// home (D300 §2, §3).
func (server *Server) SetHomeNetworks(networks []netip.Prefix) {
	held := slices.Clone(networks)
	server.homeNetworks.Store(&held)
}

// isHomeIP is whether ip is in one of the home networks.
func (server *Server) isHomeIP(ip net.IP) bool {
	address, isAddress := netip.AddrFromSlice(ip)
	networks := server.homeNetworks.Load()
	if !isAddress || networks == nil {
		return false
	}
	address = address.Unmap()
	return slices.ContainsFunc(*networks, func(network netip.Prefix) bool { return network.Contains(address) })
}

// AcceptsAnyRelayedAddress is whether AcceptAnyRelayedAddress was called.
func (server *Server) AcceptsAnyRelayedAddress() bool {
	return server.anyRelayedAddress.Load()
}

// New returns a server whose channels' requests handler serves, until ctx ends.
func New(ctx context.Context, handler http.Handler, logger *slog.Logger) *Server {
	server := &Server{
		ctx: ctx, handler: handler, logger: logger, clock: clock.System{},
		api: webrtc.NewAPI(webrtc.WithSettingEngine(settings())),
		bounds: bounds{
			connectWithin: connectWithin, perSource: perSourceLimit, channels: channelLimit,
			requestDeadline: requestDeadline,
		},
		connections: map[*webrtc.PeerConnection]bool{}, paths: map[Path]int{},
		sources: map[*webrtc.PeerConnection]netip.Prefix{},
	}
	server.relayedAPI = webrtc.NewAPI(webrtc.WithSettingEngine(server.relayedSettings()))
	return server
}

// ListenLAN has every connection gather its host candidates on one UDP port, which the deployment
// publishes at the house's addresses, lanIPs, on the same port, and offer those addresses at it as
// host candidates too: a core in a container, whose own addresses nothing at home reaches, is reached
// by a device in the house directly, not out through the VPS and back (D300 §1). It is called once,
// before the first offer.
func (server *Server) ListenLAN(port int, lanIPs []netip.Addr) error {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		return fmt.Errorf("listen for WebRTC on UDP port %d: %w", port, err)
	}
	mux := webrtc.NewICEUDPMux(nil, conn)
	external := make([]string, 0, len(lanIPs))
	for _, address := range lanIPs {
		external = append(external, address.Unmap().String())
	}
	onLAN := func(engine webrtc.SettingEngine) (webrtc.SettingEngine, error) {
		engine.SetICEUDPMux(mux)
		ruleErr := engine.SetICEAddressRewriteRules(webrtc.ICEAddressRewriteRule{
			External: external, AsCandidateType: webrtc.ICECandidateTypeHost,
			Mode: webrtc.ICEAddressRewriteAppend, Networks: []webrtc.NetworkType{webrtc.NetworkTypeUDP4},
		})
		return engine, ruleErr
	}
	own, err := onLAN(settings())
	if err != nil {
		return errors.Join(fmt.Errorf("offer the house's addresses: %w", err), mux.Close())
	}
	relayed, err := onLAN(server.relayedSettings())
	if err != nil {
		return errors.Join(fmt.Errorf("offer the house's addresses: %w", err), mux.Close())
	}
	server.api = webrtc.NewAPI(webrtc.WithSettingEngine(own))
	server.relayedAPI = webrtc.NewAPI(webrtc.WithSettingEngine(relayed))
	return nil
}

// relayedSettings are a relayed connection's: every connection's, checking only the remote
// addresses checksRelayed takes.
func (server *Server) relayedSettings() webrtc.SettingEngine {
	relayed := settings()
	relayed.SetRemoteIPFilter(server.checksRelayed)
	return relayed
}

// settings are every connection's: the congestion window's growth, and what a client may send.
func settings() webrtc.SettingEngine {
	engine := webrtc.SettingEngine{}
	engine.SetSCTPCwndCAStep(cwndStepBytes)
	engine.SetSCTPMaxReceiveBufferSize(sctpReceiveBytes)
	engine.SetSCTPMaxMessageSize(MaxMessageBytes)
	engine.SetSTUNGatherTimeout(stunGatherTimeout)
	return engine
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
	return server.answerWith(ctx, server.api, offer.SDP)
}

// answerWith answers an offer's SDP with a connection api makes.
func (server *Server) answerWith(ctx context.Context, api *webrtc.API, offerSDP string) (Description, error) {
	connection, err := server.open(api)
	if err != nil {
		return Description{}, err
	}
	answer, err := server.answer(ctx, connection, offerSDP)
	if err != nil {
		server.forget(connection)
		return Description{}, errors.Join(err, connection.Close())
	}
	return answer, nil
}

// open makes a peer connection with api, unless the core holds MaxConnections. One that has not
// connected within its bound is closed, and so is one past its network's share once it connects.
func (server *Server) open(api *webrtc.API) (*webrtc.PeerConnection, error) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if len(server.connections) >= MaxConnections {
		return nil, ErrTooManyConnections
	}
	configuration := webrtc.Configuration{}
	if source := server.iceServers.Load(); source != nil {
		configuration.ICEServers = (*source)()
	}
	connection, err := api.NewPeerConnection(configuration)
	if err != nil {
		return nil, fmt.Errorf("make a peer connection: %w", err)
	}
	server.connections[connection] = true
	openedAt := server.clock.Now()
	var isConnected atomic.Bool
	own := &heldNetworks{}
	unconnected := time.AfterFunc(server.bounds.connectWithin, func() {
		if !isConnected.Load() {
			local, remote, pairs := candidatesOf(connection.GetStats())
			server.logger.Info("rtcapi: closed a connection that did not connect in time",
				"local", joined(local), "remote", joined(remote), "pairs", joined(pairs))
			server.forget(connection)
			_ = connection.Close() //nolint:errcheck // never connected
		}
	})
	connection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected && isConnected.CompareAndSwap(false, true) {
			unconnected.Stop()
			own.keep(connection.GetStats())
			server.recordPath(connection, selectedPath(connection), server.clock.Now().Sub(openedAt))
			server.holdWithinItsSourcesShare(connection)
		}
		if state == webrtc.PeerConnectionStateFailed && !isConnected.Load() {
			server.recordPath(connection, PathFailed, server.clock.Now().Sub(openedAt))
		}
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			server.forget(connection)
			_ = connection.Close() //nolint:errcheck // a failed or closed connection, gone either way
		}
	})
	inFlight := &atomic.Int32{}
	connection.OnDataChannel(func(channel *webrtc.DataChannel) {
		if channel.Label() == NegotiationLabel {
			return
		}
		if inFlight.Add(1) > int32(server.bounds.channels) { //nolint:gosec // a small constant
			inFlight.Add(-1)
			server.logger.Warn("rtcapi: refused a request past a connection's limit", "limit", server.bounds.channels)
			_ = channel.Close() //nolint:errcheck // refused
			return
		}
		remote := func() string { return remoteAddress(connection) }
		serveChannel(server.ctx, server.handler, channel, remote, server.logger, servedChannel{
			deadline: server.bounds.requestDeadline, ended: func() { inFlight.Add(-1) },
			path:        func() Path { return selectedPath(connection) },
			ownNetworks: own.held,
		})
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
	reachedOut := make(chan struct{})
	var reaching sync.Once
	connection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil &&
			(candidate.Typ == webrtc.ICECandidateTypeSrflx || candidate.Typ == webrtc.ICECandidateTypeRelay) {
			reaching.Do(func() { time.AfterFunc(gatherAfterReflexive, func() { close(reachedOut) }) })
		}
	})
	if err = connection.SetLocalDescription(answer); err != nil {
		return Description{}, fmt.Errorf("set the answer: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, gatherTimeout)
	defer cancel()
	select {
	case <-gathered:
	case <-reachedOut:
	case <-waitCtx.Done():
		return Description{}, fmt.Errorf("gather the core's candidates: %w", waitCtx.Err())
	}
	local := connection.LocalDescription()
	return Description{Type: local.Type.String(), SDP: local.SDP}, nil
}

// holdWithinItsSourcesShare closes a connection that just connected when its client's network holds
// more than its share of the core's connections already.
func (server *Server) holdWithinItsSourcesShare(connection *webrtc.PeerConnection) {
	source := sourceOf(remoteAddress(connection))
	server.mutex.Lock()
	server.sources[connection] = source
	held := 0
	for _, each := range server.sources {
		if each == source {
			held++
		}
	}
	server.mutex.Unlock()
	if held > server.bounds.perSource {
		server.logger.Warn("rtcapi: closed a connection past its network's share", "limit", server.bounds.perSource)
		server.forget(connection)
		_ = connection.Close() //nolint:errcheck // refused
	}
}

// sourceOf is the network a client's address is counted under: an IPv4 address, or an IPv6 /56.
func sourceOf(address string) netip.Prefix {
	addrPort, err := netip.ParseAddrPort(address)
	if err != nil {
		return netip.Prefix{}
	}
	host := addrPort.Addr().Unmap()
	if host.Is4() {
		return netip.PrefixFrom(host, 32)
	}
	return netip.PrefixFrom(host, sourceIPv6Prefix).Masked()
}

// forget drops a connection from those held.
func (server *Server) forget(connection *webrtc.PeerConnection) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	delete(server.connections, connection)
	delete(server.sources, connection)
}

// Close closes every connection held.
func (server *Server) Close() error {
	server.mutex.Lock()
	held := make([]*webrtc.PeerConnection, 0, len(server.connections))
	for connection := range server.connections {
		held = append(held, connection)
	}
	server.connections = map[*webrtc.PeerConnection]bool{}
	server.sources = map[*webrtc.PeerConnection]netip.Prefix{}
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
