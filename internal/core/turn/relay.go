// SPDX-License-Identifier: AGPL-3.0-or-later

package turn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"syscall"

	pionturn "github.com/pion/turn/v4"

	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// realm is the relay's realm, which a TURN client hashes its credentials with.
const realm = "orpheum"

// Config is where the relay listens and what it tells the browsers (D243 §8).
type Config struct {
	// Listen is the address TURN is served on, over UDP and TCP on the same port, ":3478".
	Listen string
	// PublicAddress is the IP address the browsers reach the relay at: the VPS's (D243 §7).
	PublicAddress net.IP
	// RelayPortMin and RelayPortMax are the UDP ports relayed media flows through, both included.
	RelayPortMin, RelayPortMax uint16
}

// IsMember is whether a membership is still live: credentials end with it (D243 §8).
type IsMember func(sessionID, memberID int64) bool

// Relay is the TURN server embedded in the binary, which carries Watch Together's voice when two
// browsers cannot reach each other directly (D68, D193 §4).
type Relay struct {
	server  *pionturn.Server
	address net.Addr
}

// Parts are what the relay needs besides its config.
type Parts struct {
	Secrets  Secrets
	IsMember IsMember
	Clock    clock.Clock
	Logger   *slog.Logger
}

// portChoices is how many ports the system may choose before a relay listening on port 0 gives up:
// UDP's chosen port can be taken over TCP by someone else.
const portChoices = 5

// listenOnOnePort listens on address over UDP, then over TCP (with listenTCP) on the port UDP took.
// When address leaves the port to the system and that port is taken over TCP, it lets the system
// choose again, up to portChoices times; a port named in address is taken or refused as it is.
func listenOnOnePort(ctx context.Context, address string,
	listenTCP func(context.Context, string, string) (net.Listener, error),
) (net.PacketConn, net.Listener, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, nil, fmt.Errorf("TURN's listen address %q: %w", address, err)
	}
	for choice := 1; ; choice++ {
		packets, err := (&net.ListenConfig{}).ListenPacket(ctx, "udp", address)
		if err != nil {
			return nil, nil, fmt.Errorf("listen for TURN on UDP %s: %w", address, err)
		}
		streams, err := listenTCP(ctx, "tcp", packets.LocalAddr().String())
		if err == nil {
			return packets, streams, nil
		}
		_ = packets.Close() //nolint:errcheck // the listen's error is the one to report
		if port != "0" && port != "" || !errors.Is(err, syscall.EADDRINUSE) || choice == portChoices {
			return nil, nil, fmt.Errorf("listen for TURN on TCP %s: %w", packets.LocalAddr(), err)
		}
	}
}

// Start serves TURN on config.Listen, over UDP and TCP, admitting the credentials Issue makes while
// their membership lasts and until they expire. Relayed media leaves from the listen address's
// host, so a relay listening on the tunnel's address answers through the tunnel (D243 §7). The
// relay reads the shared secret, and logs, under ctx.
func Start(ctx context.Context, config Config, parts Parts) (*Relay, error) {
	host, _, err := net.SplitHostPort(config.Listen)
	if err != nil {
		return nil, fmt.Errorf("TURN's listen address %q: %w", config.Listen, err)
	}
	if host == "" {
		host = "0.0.0.0"
	}
	packets, streams, err := listenOnOnePort(ctx, config.Listen, (&net.ListenConfig{}).Listen)
	if err != nil {
		return nil, err
	}
	relayed := func() pionturn.RelayAddressGenerator {
		return &pionturn.RelayAddressGeneratorPortRange{
			RelayAddress: config.PublicAddress, Address: host,
			MinPort: config.RelayPortMin, MaxPort: config.RelayPortMax,
		}
	}
	granted := &grantedAllocations{held: map[grantKey]bool{}}
	server, err := pionturn.NewServer(pionturn.ServerConfig{
		Realm:         realm,
		AuthHandler:   authHandler(ctx, parts, granted),
		EventHandler:  granted.events(),
		LoggerFactory: slogFactory{ctx: ctx, logger: parts.Logger},
		PacketConnConfigs: []pionturn.PacketConnConfig{{
			PacketConn: packets, RelayAddressGenerator: relayed(), PermissionHandler: mayRelayTo,
		}},
		ListenerConfigs: []pionturn.ListenerConfig{{
			Listener: streams, RelayAddressGenerator: relayed(), PermissionHandler: mayRelayTo,
		}},
	})
	if err != nil {
		_ = packets.Close() //nolint:errcheck // the start's error is the one to report
		_ = streams.Close() //nolint:errcheck // the start's error is the one to report
		return nil, fmt.Errorf("start the TURN relay: %w", err)
	}
	return &Relay{server: server, address: packets.LocalAddr()}, nil
}

// notPublic are the IPv4 ranges that are not the public internet though Go counts them global
// unicast: shared address space (a carrier's NAT), the IETF's (DS-Lite's 192.0.0.0/29 among them),
// benchmarking, and the reserved.
func notPublic() []netip.Prefix {
	return []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("240.0.0.0/4"),
	}
}

// mayRelayTo admits a peer on the public internet alone: a member's relay reaches the other members'
// browsers, never the server's own machine or the network it is on.
func mayRelayTo(_ net.Addr, peer net.IP) bool {
	address, isIP := netip.AddrFromSlice(peer)
	if !isIP {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() {
		return false
	}
	for _, prefix := range notPublic() {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

// Address is where the relay listens, on UDP and TCP: config.Listen, its port chosen when that
// named none.
func (relay *Relay) Address() net.Addr { return relay.address }

// Close stops the relay and every allocation on it.
func (relay *Relay) Close() error {
	if err := relay.server.Close(); err != nil {
		return fmt.Errorf("stop the TURN relay: %w", err)
	}
	return nil
}

// grantKey is an allocation by the address it was asked from and the username it was granted to.
type grantKey struct{ source, username string }

// grantedAllocations are the allocations made with a discovery's credentials while they were good,
// which their refreshes, permissions and channels keep being admitted for after the credentials
// expire: they last foundLifetime, shorter than a film, and the allocation ends with the device.
type grantedAllocations struct {
	mutex sync.Mutex
	held  map[grantKey]bool
}

// events keeps an allocation from when it is made with a discovery's credentials until it is
// deleted.
func (granted *grantedAllocations) events() pionturn.EventHandler {
	return pionturn.EventHandler{
		OnAllocationCreated: func(source, _ net.Addr, _, username, _ string, _ net.Addr, _ int) {
			if held, _, err := parseUsername(username); err == nil && held.isFound {
				granted.mutex.Lock()
				defer granted.mutex.Unlock()
				granted.held[grantKey{source: source.String(), username: username}] = true
			}
		},
		OnAllocationDeleted: func(source, _ net.Addr, _, username, _ string) {
			granted.mutex.Lock()
			defer granted.mutex.Unlock()
			delete(granted.held, grantKey{source: source.String(), username: username})
		},
	}
}

// has is whether an allocation from source with username is held.
func (granted *grantedAllocations) has(source net.Addr, username string) bool {
	granted.mutex.Lock()
	defer granted.mutex.Unlock()
	return source != nil && granted.held[grantKey{source: source.String(), username: username}]
}

// authHandler admits a username Issue made, unexpired, of a live membership, or one IssueFound
// made, unexpired or held by an allocation it was granted, and gives TURN the key its password
// makes; anything else is refused.
func authHandler(ctx context.Context, parts Parts, granted *grantedAllocations) pionturn.AuthHandler {
	return func(username, _ string, source net.Addr) ([]byte, bool) {
		held, expiresAt, err := parseUsername(username)
		if err != nil {
			return nil, false
		}
		isGood := parts.Clock.Now().Before(expiresAt) || held.isFound && granted.has(source, username)
		if !isGood || !held.isFound && !parts.IsMember(held.membership.SessionID, held.membership.MemberID) {
			return nil, false
		}
		secret, err := parts.Secrets.Open(ctx, secrets.TURNSecret)
		if err != nil {
			parts.Logger.WarnContext(ctx, "the TURN secret could not be read; voice relays refuse everyone",
				slog.Any("err", err))
			return nil, false
		}
		return pionturn.GenerateAuthKey(username, realm, password(secret, username)), true
	}
}
