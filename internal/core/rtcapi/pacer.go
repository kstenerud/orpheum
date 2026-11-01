// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/pion/transport/v5"
	"github.com/pion/transport/v5/stdnet"
	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// The pacing of the packets a connection sends (webrtc-throughput-review-2026-10-28.md §4.1, fix 4;
// D397). The pion library sends its whole congestion window back to back, and Chrome reads every WebRTC socket
// through a 64 KiB receive buffer, about 50 packets: the burst overflows it, pion takes the drops
// for congestion and halves its window. Paced, the same window arrives spread out, and nothing is
// dropped. The kernel paces a UDP socket only under the fq queueing discipline, which a house's NAS
// need not have (internal/platform/pacing paces TCP), so the core paces in its own sends:
//   - paceGain: a connection's packets go at most paceGain times its window per smoothed round trip,
//     the window spread over half a round trip, as Linux paces TCP in slow start;
//   - paceCeilingBytes: and never faster than 300 Mb/s, the rate the review measured unpaced
//     Chrome's receive path to keep up with (§4.1, "Paced 300");
//   - paceBurstBytes: what may go back to back, 16 KiB, a fraction of the 64 KiB buffer;
//   - paceRefresh: how often the rate is read again from the window and the round trip, apart from
//     the sends: SCTP sends holding its association's lock, which reading its window takes.
const (
	paceGain         = 2
	paceCeilingBytes = 300_000_000 / 8
	paceBurstBytes   = 16 << 10
	paceRefresh      = 10 * time.Millisecond
)

// window is a connection's congestion window and smoothed round trip, as its SCTP reports them.
type window func() (cwndBytes uint32, srtt time.Duration)

// pacer paces what the core sends to each connection's far end, at a rate from the connection's
// window (paceRate). An address no connection follows is not paced: STUN, and ICE's checks before a
// connection connects.
type pacer struct {
	ctx   context.Context //nolint:containedctx // the server's lifetime, which ends every wait
	clock clock.Clock

	mutex sync.Mutex
	ends  map[netip.AddrPort]*pacedEnd
}

// newPacer is a pacer timed by timer, whose waits end with ctx.
func newPacer(ctx context.Context, timer clock.Clock) *pacer {
	return &pacer{ctx: ctx, clock: timer, ends: map[netip.AddrPort]*pacedEnd{}}
}

// follow paces what goes to address at a rate from of: the far end of a connection that connected.
// Until the next refresh it goes at the ceiling.
func (pacer *pacer) follow(address netip.AddrPort, of window) {
	pacer.mutex.Lock()
	defer pacer.mutex.Unlock()
	pacer.ends[unmapped(address)] = &pacedEnd{window: of, rate: paceCeilingBytes, tokens: paceBurstBytes}
}

// refresh reads each followed connection's rate again from its window.
func (pacer *pacer) refresh() {
	pacer.mutex.Lock()
	ends := make([]*pacedEnd, 0, len(pacer.ends))
	for _, end := range pacer.ends {
		ends = append(ends, end)
	}
	pacer.mutex.Unlock()
	for _, end := range ends {
		rate := paceRate(end.window())
		end.mutex.Lock()
		end.rate = rate
		end.mutex.Unlock()
	}
}

// run refreshes the rates every paceRefresh until the pacer's context ends.
func (pacer *pacer) run() {
	pacer.mutex.Lock()
	timer := pacer.clock.NewTimer(paceRefresh)
	pacer.mutex.Unlock()
	defer timer.Stop()
	for {
		select {
		case <-pacer.ctx.Done():
			return
		case <-timer.C():
			pacer.refresh()
			timer.Reset(paceRefresh)
		}
	}
}

// forget stops pacing what goes to address.
func (pacer *pacer) forget(address netip.AddrPort) {
	pacer.mutex.Lock()
	defer pacer.mutex.Unlock()
	delete(pacer.ends, unmapped(address))
}

// wait waits until size bytes may go to to.
func (pacer *pacer) wait(to net.Addr, size int) {
	udp, isUDP := to.(*net.UDPAddr)
	if !isUDP {
		return
	}
	pacer.mutex.Lock()
	end, timer := pacer.ends[unmapped(udp.AddrPort())], pacer.clock
	pacer.mutex.Unlock()
	if end == nil {
		return
	}
	if delay := end.take(timer.Now(), size); delay > 0 {
		_ = timer.Sleep(pacer.ctx, delay) //nolint:errcheck // the server is ending: send at once
	}
}

// unmapped is address with an IPv4 address as itself, never IPv4-mapped IPv6: one end, one key.
func unmapped(address netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(address.Addr().Unmap(), address.Port())
}

// paceRate is the rate, in bytes a second, at which a connection with window cwndBytes and smoothed
// round trip srtt is paced: paceGain windows a round trip, at most paceCeilingBytes; the ceiling
// before SCTP has measured a round trip.
func paceRate(cwndBytes uint32, srtt time.Duration) float64 {
	if cwndBytes == 0 || srtt <= 0 {
		return paceCeilingBytes
	}
	return min(paceCeilingBytes, paceGain*float64(cwndBytes)/srtt.Seconds())
}

// pacedEnd is a token bucket for one far end: tokens are the bytes that may go now, at most
// paceBurstBytes, refilled at rate; below zero, what was sent ahead of the rate.
type pacedEnd struct {
	window window

	mutex    sync.Mutex
	rate     float64
	tokens   float64
	filledAt time.Time
}

// take takes size bytes at now, and is how long their sender waits before they go.
func (end *pacedEnd) take(now time.Time, size int) time.Duration {
	end.mutex.Lock()
	defer end.mutex.Unlock()
	if !end.filledAt.IsZero() {
		end.tokens = min(paceBurstBytes, end.tokens+now.Sub(end.filledAt).Seconds()*end.rate)
	}
	end.filledAt = now
	end.tokens -= float64(size)
	if end.tokens >= 0 {
		return 0
	}
	return time.Duration(-end.tokens / end.rate * float64(time.Second))
}

// sctpWindow is a connection's window as its SCTP transport reports it.
func sctpWindow(connection *webrtc.PeerConnection) window {
	return func() (uint32, time.Duration) {
		sctp := connection.SCTP()
		if sctp == nil {
			return 0, 0
		}
		stats := sctp.Stats()
		return stats.CongestionWindow, time.Duration(stats.SmoothedRoundTripTime * float64(time.Second))
	}
}

// pacedPacketConn is a packet socket whose sends the pacer paces: the LAN port's, under ICE's mux.
type pacedPacketConn struct {
	net.PacketConn
	pacer *pacer
}

// WriteTo sends packet to its address once the pacer lets it.
func (conn pacedPacketConn) WriteTo(packet []byte, to net.Addr) (int, error) {
	conn.pacer.wait(to, len(packet))
	return conn.PacketConn.WriteTo(packet, to)
}

// pacedUDPConn is a UDP socket ICE opens for one connection whose sends the pacer paces.
type pacedUDPConn struct {
	transport.UDPConn
	pacer *pacer
}

// WriteTo sends packet to its address once the pacer lets it.
func (conn pacedUDPConn) WriteTo(packet []byte, to net.Addr) (int, error) {
	conn.pacer.wait(to, len(packet))
	return conn.UDPConn.WriteTo(packet, to)
}

// WriteToUDP sends packet to its address once the pacer lets it.
func (conn pacedUDPConn) WriteToUDP(packet []byte, to *net.UDPAddr) (int, error) {
	conn.pacer.wait(to, len(packet))
	return conn.UDPConn.WriteToUDP(packet, to)
}

// Write sends packet to the socket's far end once the pacer lets it.
func (conn pacedUDPConn) Write(packet []byte) (int, error) {
	if to := conn.RemoteAddr(); to != nil {
		conn.pacer.wait(to, len(packet))
	}
	return conn.UDPConn.Write(packet)
}

// pacedNet is the network ICE opens its sockets on, each paced.
type pacedNet struct {
	transport.Net
	pacer *pacer
}

// newPacedNet is the machine's network, its UDP sockets paced by pacer.
func newPacedNet(pacer *pacer) (pacedNet, error) {
	machine, err := stdnet.NewNet()
	if err != nil {
		return pacedNet{}, err
	}
	return pacedNet{Net: machine, pacer: pacer}, nil
}

// ListenUDP opens a paced UDP socket.
func (network pacedNet) ListenUDP(kind string, local *net.UDPAddr) (transport.UDPConn, error) {
	conn, err := network.Net.ListenUDP(kind, local)
	if err != nil {
		return nil, err
	}
	return pacedUDPConn{UDPConn: conn, pacer: network.pacer}, nil
}

// DialUDP opens a paced UDP socket to remote: a TURN server's.
func (network pacedNet) DialUDP(kind string, local, remote *net.UDPAddr) (transport.UDPConn, error) {
	conn, err := network.Net.DialUDP(kind, local, remote)
	if err != nil {
		return nil, err
	}
	return pacedUDPConn{UDPConn: conn, pacer: network.pacer}, nil
}

// ListenPacket opens a paced packet socket.
func (network pacedNet) ListenPacket(kind, address string) (net.PacketConn, error) {
	conn, err := network.Net.ListenPacket(kind, address)
	if err != nil {
		return nil, err
	}
	return pacedPacketConn{PacketConn: conn, pacer: network.pacer}, nil
}

// paceTo has the pacer follow a connection that just connected over pair: what goes to the client's
// end, at a rate from the connection's window. A pair whose core end is a relay of the core's own
// sends to its TURN server instead, which is not paced.
func (server *Server) paceTo(connection *webrtc.PeerConnection, pair *webrtc.ICECandidatePair, held *heldConnection) {
	if pair == nil || pair.Local == nil || pair.Remote == nil || pair.Local.Typ == webrtc.ICECandidateTypeRelay {
		return
	}
	address, err := netip.ParseAddr(pair.Remote.Address)
	if err != nil {
		return
	}
	to := netip.AddrPortFrom(address, pair.Remote.Port)
	held.pacedTo.Store(&to)
	server.pacer.follow(to, sctpWindow(connection))
}

// noteConnected notes that connection connected over pair, took after it opened: for its end line,
// its path, and the pacer.
func (server *Server) noteConnected(connection *webrtc.PeerConnection, pair *webrtc.ICECandidatePair,
	held *heldConnection, took time.Duration,
) {
	held.end.connected(server.recordSelectedPath(pair, took), server.clock.Now())
	server.paceTo(connection, pair, held)
}
