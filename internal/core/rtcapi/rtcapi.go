// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"context"
	"errors"
	"fmt"
	"io"
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

	"github.com/kstenerud/orpheum/internal/core/home"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// MaxMessageBytes is the most a message on a channel holds, which every browser carries.
const MaxMessageBytes = 64 << 10

// DefaultMaxConnections bounds the peer connections the core holds at once until its config names
// another (SetMaxConnections, cmd/orpheum's webrtc.max_connections, D348): a client opens a few
// (ClientConnections), and a household has a handful of people and devices.
const DefaultMaxConnections = 64

// ClientConnections is how many peer connections a client opens at most and spreads its requests
// across, keeping one and opening the others only while its requests crowd it: one is bound by its
// congestion window, and four carried three times as much at a 50 ms round trip
// (reachability-approaches-2026-10-09.md §9.4).
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
//     household's usual delegation), a few households' clients behind one address, until the core
//     reads its setting, webrtcConnectionsPerNetwork (SetConnectionsPerNetwork), which replaces it;
//   - channelLimit: the requests in flight on one connection, each a channel, its body bounded by
//     MaxRequestBodyBytes;
//   - requestDeadline: how long a request's head and body may take to arrive whole;
//   - sctpReceiveBytes: the most SCTP buffers of what a client sends, which is requests' small bodies.
const (
	connectWithin    = 10 * time.Second
	perSourceLimit   = 64
	channelLimit     = 16
	requestDeadline  = 30 * time.Second
	sctpReceiveBytes = 1 << 20
	sourceIPv6Prefix = 56
)

// ErrTooManyConnections is an offer refused because the core holds its most connections already.
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
	api        atomic.Pointer[webrtc.API]
	relayedAPI atomic.Pointer[webrtc.API]
	logger     *slog.Logger
	// clock times each connection from its offer to its connecting, for the log.
	clock clock.Clock
	// bounds are the bounds on each connection: the constants above, shorter in the tests.
	bounds bounds
	// pacer paces what the connections send (pacer.go).
	pacer *pacer

	// iceServers are the STUN and TURN servers a connection gathers its candidates through: its
	// discovery's (D274); none gathers only the core's own addresses.
	iceServers atomic.Pointer[func() []webrtc.ICEServer]
	// anyRelayedAddress lets a relayed connection check any remote address, the core's own networks'
	// too: for a development build alone, whose browser tests put the browser, the discovery and the
	// core on one machine (AcceptAnyRelayedAddress).
	anyRelayedAddress atomic.Bool
	// homeRule is the rule of the house's addresses (SetHome); none until named.
	homeRule atomic.Pointer[home.Rule]
	// ownIPv6 are the core's own public IPv6 networks the last connection to connect found
	// (OwnIPv6Networks); none until one has.
	ownIPv6 atomic.Pointer[[]netip.Prefix]

	mutex sync.Mutex
	// Guarded by mutex: connections are the peer connections held now, and paths how many
	// connections took each path.
	connections map[*webrtc.PeerConnection]bool
	paths       map[Path]int
	// localPath is what the connections said of the path from the house (D354).
	localPath localPathWatch
	// sources are the networks connected clients are counted under, by their connections.
	sources map[*webrtc.PeerConnection]netip.Prefix
	// held are the records of the connections held, for the log, by their connections, numbered
	// from numbered.
	held     map[*webrtc.PeerConnection]*heldConnection
	numbered int64
	// lanPort, lanIPs and lanMux are where the core takes WebRTC at home (ListenLAN,
	// SetLANAddresses): 0 and none until then.
	lanPort int
	lanIPs  []netip.Addr
	lanMux  udpMux
}

// bounds are the bounds on each connection.
type bounds struct {
	// connections are the most the core holds at once, of every network.
	connections     int
	connectWithin   time.Duration
	perSource       int
	channels        int
	requestDeadline time.Duration
	// answerLingers is how long a response's channel may stay open after the core's last byte for it
	// before its close is logged (answerWatch).
	answerLingers time.Duration
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
	return server.anyRelayedAddress.Load() || isFromOutside(ip, home.InterfaceNetworks)
}

// SetHome names the rule of the house's addresses, the one api judges a direct request by (D452 §1):
// a relayed connection answers a check from one, and a request over a connection whose selected pair
// reaches one directly is from home (D300 §2, §3).
func (server *Server) SetHome(rule *home.Rule) {
	server.homeRule.Store(rule)
}

// isHomeIP is whether ip is home by the rule SetHome named.
func (server *Server) isHomeIP(ip net.IP) bool {
	return server.homeRule.Load().ContainsIP(ip)
}

// OwnIPv6Networks are the /64s of the core's own public IPv6 addresses as the last connection to
// connect found them through the core's own STUN servers (ownIPv6Networks): the house's IPv6 network,
// which the home rule counts for a direct request too (D452 §2). None until a connection has
// connected, or when the last found none.
func (server *Server) OwnIPv6Networks() []netip.Prefix {
	if own := server.ownIPv6.Load(); own != nil {
		return slices.Clone(*own)
	}
	return nil
}

// AcceptsAnyRelayedAddress is whether AcceptAnyRelayedAddress was called.
func (server *Server) AcceptsAnyRelayedAddress() bool {
	return server.anyRelayedAddress.Load()
}

// New returns a server whose channels' requests handler serves, until ctx ends.
func New(ctx context.Context, handler http.Handler, logger *slog.Logger) *Server {
	paced := newPacer(ctx, clock.System{})
	go paced.run()
	server := &Server{
		ctx: ctx, handler: handler, logger: logger, clock: clock.System{}, pacer: paced,

		bounds: bounds{
			connections:   DefaultMaxConnections,
			connectWithin: connectWithin, perSource: perSourceLimit, channels: channelLimit,
			requestDeadline: requestDeadline, answerLingers: answerLingersAfter,
		},
		connections: map[*webrtc.PeerConnection]bool{}, paths: map[Path]int{},
		sources: map[*webrtc.PeerConnection]netip.Prefix{},
		held:    map[*webrtc.PeerConnection]*heldConnection{},
	}
	server.api.Store(newAPI(settings(paced)))
	server.relayedAPI.Store(newAPI(server.relayedSettings()))
	return server
}

// ListenLAN has every connection gather its host candidates on one UDP port, which the deployment
// publishes at the house's addresses, lanIPs, on the same port, and offer those addresses at it as
// its IPv4 host candidates in place of its own: a core in a container, whose own addresses nothing at
// home reaches, is reached by a device in the house directly, not out through the VPS and back (D300
// §1). Its host candidates on the port are one address of each family (onePerFamily), and the
// house's in place of the container's IPv4 one, not beside it (D441). It is called once, before the
// first offer; SetLANAddresses replaces the addresses later, on the same socket. With no address yet
// the port is still taken, and the core's own addresses offered on it.
func (server *Server) ListenLAN(port int, lanIPs []netip.Addr) error {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		return fmt.Errorf("listen for WebRTC on UDP port %d: %w", port, err)
	}
	mux := onePerFamily{webrtc.NewICEUDPMux(nil, pacedPacketConn{PacketConn: conn, pacer: server.pacer})}
	server.mutex.Lock()
	server.lanMux, server.lanPort = mux, port
	server.mutex.Unlock()
	if err = server.SetLANAddresses(lanIPs); err != nil {
		return errors.Join(err, mux.Close())
	}
	return nil
}

// SetLANAddresses replaces the house's addresses ListenLAN offers as host candidates, as the address
// at home is learned (D452 §3): an offer answered after it offers the new ones, on the same socket;
// a connection already made keeps its own. Before ListenLAN it does nothing.
func (server *Server) SetLANAddresses(lanIPs []netip.Addr) error {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if server.lanMux == nil {
		return nil
	}
	external := make([]string, 0, len(lanIPs))
	for _, address := range lanIPs {
		external = append(external, address.Unmap().String())
	}
	onLAN := func(engine webrtc.SettingEngine) (webrtc.SettingEngine, error) {
		engine.SetICEUDPMux(server.lanMux)
		if len(external) == 0 {
			return engine, nil
		}
		ruleErr := engine.SetICEAddressRewriteRules(webrtc.ICEAddressRewriteRule{
			External: external, AsCandidateType: webrtc.ICECandidateTypeHost,
			Mode: webrtc.ICEAddressRewriteReplace, Networks: []webrtc.NetworkType{webrtc.NetworkTypeUDP4},
		})
		return engine, ruleErr
	}
	own, err := onLAN(settings(server.pacer))
	if err != nil {
		return fmt.Errorf("offer the house's addresses: %w", err)
	}
	relayed, err := onLAN(server.relayedSettings())
	if err != nil {
		return fmt.Errorf("offer the house's addresses: %w", err)
	}
	server.api.Store(newAPI(own))
	server.relayedAPI.Store(newAPI(relayed))
	server.lanIPs = slices.Clone(lanIPs)
	return nil
}

// udpMux is pion's ICE UDP mux, as webrtc.NewICEUDPMux returns it and SettingEngine.SetICEUDPMux
// takes it.
type udpMux interface {
	io.Closer
	GetConn(ufrag string, addr net.Addr) (net.PacketConn, error)
	RemoveConnByUfrag(ufrag string)
	GetListenAddresses() []net.Addr
}

// onePerFamily is a mux that names one listening address of each family. Listening on every address,
// pion's mux names them all, and ICE gathers a host candidate at each, every one of a family reading
// the one socket pion's mux keeps for a connection and family: a check reaching the socket was
// credited to whichever candidate read it first, so a connection's pairs never agreed and the core,
// the DTLS client, never started DTLS. Firefox, connecting over IPv6 alone and checking rarely, did
// not connect at all (idiosyncrasies.md, D441).
type onePerFamily struct{ udpMux }

// GetListenAddresses is one of the mux's addresses of each family, as listenAddressPerFamily picks it.
func (mux onePerFamily) GetListenAddresses() []net.Addr {
	return listenAddressPerFamily(mux.udpMux.GetListenAddresses())
}

// listenAddressPerFamily is, of each family in addresses, the first that is neither loopback nor
// link-local, or failing that the first that is not loopback: ICE gathers no host candidate on the
// loopback, and a link-local address reaches only the link the core's container is on.
func listenAddressPerFamily(addresses []net.Addr) []net.Addr {
	rank := func(ip net.IP) int {
		switch {
		case ip.IsLoopback():
			return 2
		case ip.IsLinkLocalUnicast():
			return 1
		default:
			return 0
		}
	}
	best := map[bool]int{}
	picked := map[bool]net.Addr{}
	for _, address := range addresses {
		udp, isUDP := address.(*net.UDPAddr)
		if !isUDP || rank(udp.IP) == 2 {
			continue
		}
		isIPv4 := udp.IP.To4() != nil
		if current, seen := best[isIPv4]; !seen || rank(udp.IP) < current {
			best[isIPv4], picked[isIPv4] = rank(udp.IP), address
		}
	}
	return slices.DeleteFunc([]net.Addr{picked[true], picked[false]}, func(address net.Addr) bool {
		return address == nil
	})
}

// relayedSettings are a relayed connection's: every connection's, checking only the remote
// addresses checksRelayed takes.
func (server *Server) relayedSettings() webrtc.SettingEngine {
	relayed := settings(server.pacer)
	relayed.SetRemoteIPFilter(server.checksRelayed)
	return relayed
}

// settings are every connection's: the congestion window's growth, and what a client may send. A
// browser closing its connection sends DTLS close_notify, on which pion closes the connection itself,
// dropping SCTP's association before its end could be logged (logEnded): the core closes it instead
// as its SCTP ends. Every socket ICE opens sends through paced (pacer.go).
func settings(paced *pacer) webrtc.SettingEngine {
	engine := webrtc.SettingEngine{}
	engine.DisableCloseByDTLS(true)
	if network, err := newPacedNet(paced); err == nil {
		engine.SetNet(network)
	}
	engine.SetSCTPCwndCAStep(cwndStepBytes)
	engine.SetSCTPMaxReceiveBufferSize(sctpReceiveBytes)
	engine.SetSCTPMaxMessageSize(MaxMessageBytes)
	engine.SetSTUNGatherTimeout(stunGatherTimeout)
	return engine
}

// SetConnectionsPerNetwork sets how many connections one network may hold at once, its share
// (webrtcConnectionsPerNetwork): a connection past it is turned away. One already held is kept.
func (server *Server) SetConnectionsPerNetwork(limit int) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.bounds.perSource = limit
}

// SetMaxConnections sets the most connections the core holds at once, of every network (the config's
// webrtc.max_connections, D348): an offer past it is refused. One already held is kept.
func (server *Server) SetMaxConnections(limit int) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.bounds.connections = limit
}

// MaxConnections is the most connections the core holds at once, of every network.
func (server *Server) MaxConnections() int {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return server.bounds.connections
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
	return server.answerWith(ctx, server.api.Load(), offer.SDP, nil)
}

// answerWith answers an offer's SDP with a connection api makes, telling onProvenAtHome, unless nil,
// when the connection proves itself from home.
func (server *Server) answerWith(
	ctx context.Context, api *webrtc.API, offerSDP string, onProvenAtHome func(ProvenAtHome),
) (Description, error) {
	connection, err := server.open(api, onProvenAtHome)
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

// open makes a peer connection with api, unless the core holds its most connections. One that has not
// connected within its bound is closed, and so is one past its network's share once it connects. Once
// it connects, onProvenAtHome, unless nil, is told when it proved itself from home (D356 §1).
func (server *Server) open(api *webrtc.API, onProvenAtHome func(ProvenAtHome)) (*webrtc.PeerConnection, error) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if len(server.connections) >= server.bounds.connections {
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
	server.numbered++
	held := newHeldConnection(server.numbered, openedAt)
	server.held[connection] = held
	var isConnected atomic.Bool
	own := &heldNetworks{}
	verdict := &verdictTeller{}
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
			server.keepOwnIPv6Networks(own, connection.GetStats())
			pair := selectedPair(connection)
			server.noteConnected(connection, pair, held, server.clock.Now().Sub(openedAt))
			server.tellIfProvenAtHome(pair, own.held(), onProvenAtHome)
			server.holdWithinItsSourcesShare(connection, verdict)
		}
		if state == webrtc.PeerConnectionStateFailed && !isConnected.Load() {
			server.recordPath(PathFailed, nil, server.clock.Now().Sub(openedAt))
		}
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			server.logEnded(connection, held, endOfState(state))
			server.forget(connection)
			_ = connection.Close() //nolint:errcheck // a failed or closed connection, gone either way
		}
	})
	server.endWithItsSCTP(connection, held)
	inFlight := &atomic.Int32{}
	connection.OnDataChannel(func(channel *webrtc.DataChannel) {
		if channel.Label() == NegotiationLabel {
			verdict.heard(channel)
			return
		}
		if verdict.isRefused() {
			_ = channel.Close() //nolint:errcheck // a connection turned away carries no request
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
			fromHome: func() bool { return server.isDirectlyFromHome(selectedPair(connection), own.held()) },
			held:     held,
			now:      server.clock.Now, answerLingers: server.bounds.answerLingers,
		})
	})
	return connection, nil
}

// keepOwnIPv6Networks keeps the core's own IPv6 networks a connection that just connected found, in
// own for its requests, and as the last found (OwnIPv6Networks).
func (server *Server) keepOwnIPv6Networks(own *heldNetworks, report webrtc.StatsReport) {
	own.keep(report)
	found := own.held()
	server.ownIPv6.Store(&found)
}

// recordSelectedPath records, and returns, the path a connection that just connected took, by its
// selected pair, took after its offer.
func (server *Server) recordSelectedPath(pair *webrtc.ICECandidatePair, took time.Duration) Path {
	path := PathFailed
	if pair != nil {
		path = pathOf(pair.Local, pair.Remote)
	}
	server.recordPath(path, pair, took)
	return path
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

// holdWithinItsSourcesShare turns away a connection that just connected when its client's network
// holds more than its share of the core's connections already: it tells the client why (Verdict),
// serves it nothing, and closes it once the client has had turnedAwayClosesWithin to read that and
// close it itself. A connection within the share is told it is held. Either way it logs the network's
// connections by number and whose each is, so a network turned away says whose connections filled
// its share.
func (server *Server) holdWithinItsSourcesShare(connection *webrtc.PeerConnection, verdict *verdictTeller) {
	source := sourceOf(remoteAddress(connection))
	server.mutex.Lock()
	server.sources[connection] = source
	var ofSource []*heldConnection
	for each, eachSource := range server.sources {
		if eachSource == source {
			if record := server.held[each]; record != nil {
				ofSource = append(ofSource, record)
			}
		}
	}
	own := server.held[connection]
	limit := server.bounds.perSource
	server.mutex.Unlock()
	name := "?"
	if own != nil {
		name = own.name()
	}
	if len(ofSource) > limit {
		server.logger.Warn("rtcapi: turned a connection away past its network's share, telling its client why",
			"limit", limit, "connection", name, "network", source.String(),
			"reason", RefusedNetworkFull, "networkHolds", namesOf(ofSource))
		verdict.decide(Verdict{Refused: RefusedNetworkFull})
		server.forget(connection)
		time.AfterFunc(turnedAwayClosesWithin, func() {
			server.logEnded(connection, own, endCoreTurnedAway)
			_ = connection.Close() //nolint:errcheck // refused, closed by its client first or now
		})
		return
	}
	verdict.decide(Verdict{IsHeld: true})
	server.logger.Info("rtcapi: a connection is held",
		slogx.EventKey, "connectionHeld",
		"connection", name, "network", source.String(),
		"networkHeld", len(ofSource), "networkHolds", namesOf(ofSource))
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

// forget drops a connection from those held, and logs the end of one that had connected: whose it
// was, how long it lived and how many requests it carried.
func (server *Server) forget(connection *webrtc.PeerConnection) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	record, wasHeld := server.held[connection]
	_, hadConnected := server.sources[connection]
	delete(server.connections, connection)
	delete(server.sources, connection)
	delete(server.held, connection)
	if wasHeld {
		if to := record.pacedTo.Load(); to != nil {
			server.pacer.forget(*to)
		}
	}
	if wasHeld && hadConnected {
		server.logger.Info("rtcapi: a connection ended", "connection", record.name(),
			"livedMs", server.clock.Now().Sub(record.openedAt).Milliseconds(),
			"requests", record.requests.Load())
	}
}

// Close closes every connection held.
func (server *Server) Close() error {
	server.mutex.Lock()
	held := make([]*webrtc.PeerConnection, 0, len(server.connections))
	records := make([]*heldConnection, 0, len(server.connections))
	for connection := range server.connections {
		held = append(held, connection)
		records = append(records, server.held[connection])
	}
	server.connections = map[*webrtc.PeerConnection]bool{}
	server.sources = map[*webrtc.PeerConnection]netip.Prefix{}
	server.mutex.Unlock()
	var errs []error
	for index, connection := range held {
		server.logEnded(connection, records[index], endCoreStopped)
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
