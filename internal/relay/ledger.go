// SPDX-License-Identifier: AGPL-3.0-or-later

package relay

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	pionturn "github.com/pion/turn/v4"

	"github.com/kstenerud/orpheum/discovery/relaycred"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/platform/tokenbucket"
)

// The limits on allocations, each a refusal (the plan's §2.4; D559 §3, the relayed ports being
// 2,000). A client credential's is what one browser asks for on it: Chrome allocates once for each of
// its interfaces and each of the relay's URLs, on every connection, and the web client holds up to
// four connections, so a machine with two interfaces asks for sixteen. At two, the second
// connection's allocations were all refused (486) and it offered no candidate (staging, 2026-11-26).
// A source address's is above a household's: a core's allocations all come from its one address.
const (
	allocationsPerClientCredential = 16
	allocationsPerHousehold        = 64
	allocationsPerSource           = 128
)

// Transports an allocation is made over, as its ended line names them.
const (
	transportUDP = "udp"
	transportTCP = "tcp"
	transportTLS = "tls"
)

// Why a datagram is dropped.
const (
	dropNotTheOtherEnd = "notTheOtherEnd"
	dropBeyondTheRate  = "beyondTheRate"
)

// allocationKey is an allocation by its five-tuple's two ends: pion's events name it so.
type allocationKey struct{ source, destination string }

// allocation is a live allocation: whose it is, what it is relayed through, and what it carried.
type allocation struct {
	credential relaycred.Credential
	username   string
	source     net.Addr
	transport  string
	port       int
	started    time.Time
	// toClient and fromClient are the bytes relayed to this allocation's TURN client and from it.
	toClient, fromClient int64
	dropped              int64
	// window is the bytes relayed to this allocation's client, over the last ten seconds.
	window peakWindow
}

// household is what the relay keeps of one household: its rate, and its day so far.
type household struct {
	bucket *tokenbucket.Bucket
	live   int
	// The day's totals: allocations ended, bytes toward its clients and from them, datagrams dropped,
	// and the bytes toward its clients over the last ten seconds.
	ended                  int
	toClients, fromClients int64
	dropped                int64
	window                 peakWindow
}

// ledger keeps the relay's live allocations, by the port each is relayed through, and the
// households they are of: it decides where a datagram may go, and records what each carried.
type ledger struct {
	ctx    context.Context //nolint:containedctx // pion's callbacks carry none; the relay's is the one to log under
	public net.IP
	// bound is where an allocation's datagram to another goes: the relayed sockets' own host.
	bound   net.IP
	parts   Parts
	tlsPort int
	dropped atomic.Int64

	mutex      sync.Mutex
	byPort     map[int]*allocation
	byKey      map[allocationKey]*allocation
	households map[string]*household
	// perCredential and perSource count the live allocations of each client username and source IP.
	perCredential map[string]int
	perSource     map[netip.Addr]int
}

func newLedger(ctx context.Context, public, bound net.IP, parts Parts, tlsPort int) *ledger {
	return &ledger{
		ctx: ctx, public: public, bound: bound, parts: parts, tlsPort: tlsPort,
		byPort: map[int]*allocation{}, byKey: map[allocationKey]*allocation{}, households: map[string]*household{},
		perCredential: map[string]int{}, perSource: map[netip.Addr]int{},
	}
}

// authHandler admits a credential whole, under one of the relay's secrets, and unexpired, or
// expired but held by an allocation it made while it was good, which its refreshes, permissions
// and channels keep needing (the plan's §1, Lifetimes).
func (ledger *ledger) authHandler(picker *secretPicker) pionturn.AuthHandler {
	return func(username, _ string, source net.Addr) ([]byte, bool) {
		credential, err := relaycred.Parse(username)
		if err != nil {
			return nil, false
		}
		key, found := picker.take(source, username)
		if !found {
			return nil, false
		}
		if !ledger.parts.Clock.Now().After(credential.ExpiresAt) || ledger.holds(source, username) {
			return key, true
		}
		return nil, false
	}
}

// holds is whether a live allocation from source was made with username.
func (ledger *ledger) holds(source net.Addr, username string) bool {
	ledger.mutex.Lock()
	defer ledger.mutex.Unlock()
	for key, live := range ledger.byKey {
		if key.source == source.String() && live.username == username {
			return true
		}
	}
	return false
}

// mayAllocate refuses an allocation past a limit: a client credential's, a household's or a
// source address's. Each refusal is a line naming the limit: a browser refused its allocations
// offered no candidate and said only that it could not connect (staging, 2026-11-26).
func (ledger *ledger) mayAllocate(username, _ string, source net.Addr) bool {
	credential, err := relaycred.Parse(username)
	if err != nil {
		return false
	}
	ledger.mutex.Lock()
	limit := ledger.limitReached(credential, username, source)
	ledger.mutex.Unlock()
	if limit == "" {
		return true
	}
	ledger.parts.Logger.WarnContext(ledger.ctx, "refused an allocation past a limit",
		slog.String(slogx.EventKey, "relayAllocationRefused"), slog.String("limit", limit),
		slog.String("household", credential.Household), slog.String("role", string(credential.Role)),
		slog.String("source", shortened(source)))
	return false
}

// limitReached names the limit an allocation on credential from source would pass, "" for none;
// the ledger's mutex is held.
func (ledger *ledger) limitReached(credential relaycred.Credential, username string, source net.Addr) string {
	switch {
	case credential.Role == relaycred.RoleClient && ledger.perCredential[username] >= allocationsPerClientCredential:
		return "clientCredential"
	case ledger.households[credential.Household] != nil &&
		ledger.households[credential.Household].live >= allocationsPerHousehold:
		return "household"
	case ledger.perSource[sourceIP(source)] >= allocationsPerSource:
		return "sourceAddress"
	default:
		return ""
	}
}

// events keeps the ledger's allocations from pion's.
func (ledger *ledger) events() pionturn.EventHandler {
	return pionturn.EventHandler{
		OnAllocationCreated: func(source, destination net.Addr, _, username, _ string, relayed net.Addr, _ int) {
			ledger.created(source, destination, username, relayed)
		},
		OnAllocationDeleted: func(source, destination net.Addr, _, _, _ string) {
			ledger.deleted(source, destination)
		},
	}
}

// created keeps an allocation pion made.
func (ledger *ledger) created(source, destination net.Addr, username string, relayed net.Addr) {
	credential, err := relaycred.Parse(username)
	address, isUDP := relayed.(*net.UDPAddr)
	if err != nil || !isUDP {
		return
	}
	now := ledger.parts.Clock.Now()
	live := &allocation{
		credential: credential, username: username, source: source, transport: ledger.transportOf(destination),
		port: address.Port, started: now,
	}
	rate := float64(credential.Kbps) * 1000 / 8
	ledger.mutex.Lock()
	defer ledger.mutex.Unlock()
	ledger.byPort[live.port] = live
	ledger.byKey[allocationKey{source: source.String(), destination: destination.String()}] = live
	held := ledger.households[credential.Household]
	if held == nil {
		held = &household{bucket: tokenbucket.New(rate, now)}
		ledger.households[credential.Household] = held
	} else {
		held.bucket.SetRate(rate, now)
	}
	held.live++
	if credential.Role == relaycred.RoleClient {
		ledger.perCredential[username]++
	}
	ledger.perSource[sourceIP(source)]++
}

// deleted forgets an allocation that ended, and records it.
func (ledger *ledger) deleted(source, destination net.Addr) {
	key := allocationKey{source: source.String(), destination: destination.String()}
	ledger.mutex.Lock()
	ended := ledger.byKey[key]
	if ended == nil {
		ledger.mutex.Unlock()
		return
	}
	delete(ledger.byKey, key)
	delete(ledger.byPort, ended.port)
	if held := ledger.households[ended.credential.Household]; held != nil {
		held.live--
		held.ended++
	}
	if ended.credential.Role == relaycred.RoleClient {
		ledger.perCredential[ended.username]--
		if ledger.perCredential[ended.username] <= 0 {
			delete(ledger.perCredential, ended.username)
		}
	}
	ledger.perSource[sourceIP(source)]--
	if ledger.perSource[sourceIP(source)] <= 0 {
		delete(ledger.perSource, sourceIP(source))
	}
	now := ledger.parts.Clock.Now()
	line := []any{
		slog.String(slogx.EventKey, "relayAllocationEnded"),
		slog.String("household", ended.credential.Household), slog.String("role", string(ended.credential.Role)),
		slog.String("transport", ended.transport), slog.String("source", shortened(source)),
		slog.Int64("seconds", int64(now.Sub(ended.started).Seconds())),
		slog.Int64("bytesToClient", ended.toClient), slog.Int64("bytesFromClient", ended.fromClient),
		slog.Int64("peakKbpsToClient", ended.window.peakKbps()), slog.Int64("dropped", ended.dropped),
	}
	ledger.mutex.Unlock()
	ledger.parts.Logger.InfoContext(ledger.ctx, "a relayed allocation ended", line...)
}

// transportOf is what an allocation reached the relay over, told by where it reached it: pion names
// every allocation's five-tuple UDP, a stream's too.
func (ledger *ledger) transportOf(destination net.Addr) string {
	address, isTCP := destination.(*net.TCPAddr)
	switch {
	case !isTCP:
		return transportUDP
	case address.Port == ledger.tlsPort:
		return transportTLS
	default:
		return transportTCP
	}
}

// route is where a datagram from the allocation relayed through port to address may go, and
// whether it may: only to a live allocation of the same household and the other role, and toward
// a client within the household's rate. It counts what it lets through and what it drops.
func (ledger *ledger) route(port int, address net.Addr, size int) (net.Addr, bool) {
	ledger.mutex.Lock()
	reason := ""
	defer func() {
		ledger.mutex.Unlock()
		if reason != "" {
			ledger.drop(reason)
		}
	}()
	from := ledger.byPort[port]
	target := ledger.otherEnd(from, address, ledger.public)
	if target == nil {
		if from != nil {
			from.dropped++
			ledger.households[from.credential.Household].dropped++
		}
		reason = dropNotTheOtherEnd
		return nil, false
	}
	now := ledger.parts.Clock.Now()
	held := ledger.households[from.credential.Household]
	if target.credential.Role == relaycred.RoleClient && !held.bucket.TakeIfHeld(float64(size), now) {
		from.dropped++
		held.dropped++
		reason = dropBeyondTheRate
		return nil, false
	}
	from.fromClient += int64(size)
	target.toClient += int64(size)
	target.window.add(now, int64(size))
	if target.credential.Role == relaycred.RoleClient {
		held.toClients += int64(size)
		held.window.add(now, int64(size))
	} else {
		held.fromClients += int64(size)
	}
	return &net.UDPAddr{IP: ledger.bound, Port: target.port}, true
}

// arrived is what a datagram the allocation relayed through port read from source is from, as its
// client is told, and whether it may be passed on: only from a live allocation of the same
// household and the other role, which route sent toward the relayed sockets' own host.
func (ledger *ledger) arrived(port int, source net.Addr) (net.Addr, bool) {
	ledger.mutex.Lock()
	defer ledger.mutex.Unlock()
	from := ledger.otherEnd(ledger.byPort[port], source, ledger.bound)
	if from == nil {
		return nil, false
	}
	return &net.UDPAddr{IP: ledger.public, Port: from.port}, true
}

// otherEnd is the live allocation at address, on host, when it is of self's household and not of
// its role; else nil. The caller holds the mutex.
func (ledger *ledger) otherEnd(self *allocation, address net.Addr, host net.IP) *allocation {
	udp, isUDP := address.(*net.UDPAddr)
	if self == nil || !isUDP || !udp.IP.Equal(host) {
		return nil
	}
	other := ledger.byPort[udp.Port]
	if other == nil || other.credential.Household != self.credential.Household ||
		other.credential.Role == self.credential.Role {
		return nil
	}
	return other
}

// drop counts a dropped datagram.
func (ledger *ledger) drop(reason string) {
	ledger.dropped.Add(1)
	if ledger.parts.afterDrop != nil {
		ledger.parts.afterDrop(reason)
	}
}

// keepDays logs each household's day at each UTC midnight, until ctx ends.
func (ledger *ledger) keepDays(ctx context.Context) {
	for {
		now := ledger.parts.Clock.Now().UTC()
		midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		timer := ledger.parts.Clock.NewTimer(midnight.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C():
			ledger.turnDay(midnight.AddDate(0, 0, -1))
		}
	}
}

// turnDay logs each household's totals for day and begins the next, forgetting the households
// with nothing live.
func (ledger *ledger) turnDay(day time.Time) {
	ledger.mutex.Lock()
	lines := make([][]any, 0, len(ledger.households))
	for name, held := range ledger.households {
		lines = append(lines, []any{
			slog.String(slogx.EventKey, "relayHouseholdDay"), slog.String("household", name),
			slog.String("day", day.Format(time.DateOnly)), slog.Int("allocationsEnded", held.ended),
			slog.Int("allocationsLive", held.live),
			slog.Int64("bytesToClients", held.toClients), slog.Int64("bytesFromClients", held.fromClients),
			slog.Int64("peakKbpsToClients", held.window.peakKbps()), slog.Int64("dropped", held.dropped),
		})
		if held.live == 0 {
			delete(ledger.households, name)
			continue
		}
		held.ended, held.toClients, held.fromClients, held.dropped = 0, 0, 0, 0
		held.window.peak = 0
	}
	ledger.mutex.Unlock()
	for _, line := range lines {
		ledger.parts.Logger.InfoContext(ledger.ctx, "a household's day on the relay", line...)
	}
}

// sourceIP is a source address's IP, or the zero address.
func sourceIP(source net.Addr) netip.Addr {
	if address, err := netip.ParseAddrPort(source.String()); err == nil {
		return address.Addr().Unmap()
	}
	return netip.Addr{}
}

// shortened is a source address as the records keep it: an IPv4 address's /24, an IPv6's /48
// (D558 §11).
func shortened(source net.Addr) string {
	address := sourceIP(source)
	bits := 48
	if address.Is4() {
		bits = 24
	}
	prefix, err := address.Prefix(bits)
	if err != nil {
		return "unknown"
	}
	return prefix.String()
}

// peakWindow is a count of bytes over the last ten seconds, a second at a time, and the most it
// has held.
type peakWindow struct {
	last    int64
	seconds [peakSeconds]int64
	// peak is the most bytes any ten seconds held.
	peak int64
}

// peakSeconds is how long the peak rate is taken over.
const peakSeconds = 10

// add counts size bytes at now.
func (window *peakWindow) add(now time.Time, size int64) {
	second := now.Unix()
	switch {
	case second-window.last >= peakSeconds:
		window.seconds = [peakSeconds]int64{}
		window.last = second
	case second > window.last:
		for passed := window.last + 1; passed <= second; passed++ {
			window.seconds[passed%peakSeconds] = 0
		}
		window.last = second
	}
	window.seconds[window.last%peakSeconds] += size
	var held int64
	for _, count := range window.seconds {
		held += count
	}
	window.peak = max(window.peak, held)
}

// peakKbps is the highest rate any ten seconds held, in kbit/s, rounded.
func (window *peakWindow) peakKbps() int64 {
	return (window.peak*8 + peakSeconds*500) / (peakSeconds * 1000)
}

// closedRelays makes pion's relayed sockets, each wrapped so that it reaches only its household's
// other end.
type closedRelays struct {
	*pionturn.RelayAddressGeneratorPortRange
	ledger *ledger
}

// AllocatePacketConn is a relayed socket from the range, closed.
func (relays *closedRelays) AllocatePacketConn(network string, requestedPort int) (net.PacketConn, net.Addr, error) {
	conn, relayed, err := relays.RelayAddressGeneratorPortRange.AllocatePacketConn(network, requestedPort)
	if err != nil {
		return nil, nil, err
	}
	return &closedConn{PacketConn: conn, port: portOf(conn.LocalAddr()), ledger: relays.ledger}, relayed, nil
}

// closedConn is a relayed socket whose datagrams go to and come from its household's other end
// alone.
type closedConn struct {
	net.PacketConn
	port   int
	ledger *ledger
}

// WriteTo sends a datagram where route lets it go, and drops it otherwise, as a network would.
func (conn *closedConn) WriteTo(payload []byte, address net.Addr) (int, error) {
	target, may := conn.ledger.route(conn.port, address, len(payload))
	if !may {
		return len(payload), nil
	}
	return conn.PacketConn.WriteTo(payload, target)
}

// ReadFrom reads the next datagram from its household's other end, dropping any other.
func (conn *closedConn) ReadFrom(buffer []byte) (int, net.Addr, error) {
	for {
		count, source, err := conn.PacketConn.ReadFrom(buffer)
		if err != nil {
			return count, source, err
		}
		if from, may := conn.ledger.arrived(conn.port, source); may {
			return count, from, nil
		}
		conn.ledger.drop(dropNotTheOtherEnd)
	}
}
