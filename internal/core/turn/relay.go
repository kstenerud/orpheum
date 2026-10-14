// SPDX-License-Identifier: AGPL-3.0-or-later

package turn

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"

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
	packets, err := (&net.ListenConfig{}).ListenPacket(ctx, "udp", config.Listen)
	if err != nil {
		return nil, fmt.Errorf("listen for TURN on UDP %s: %w", config.Listen, err)
	}
	// TCP on the port UDP took, which a listen address with no port chose.
	streams, err := (&net.ListenConfig{}).Listen(ctx, "tcp", packets.LocalAddr().String())
	if err != nil {
		_ = packets.Close() //nolint:errcheck // the listen's error is the one to report
		return nil, fmt.Errorf("listen for TURN on TCP %s: %w", config.Listen, err)
	}
	relayed := func() pionturn.RelayAddressGenerator {
		return &pionturn.RelayAddressGeneratorPortRange{
			RelayAddress: config.PublicAddress, Address: host,
			MinPort: config.RelayPortMin, MaxPort: config.RelayPortMax,
		}
	}
	server, err := pionturn.NewServer(pionturn.ServerConfig{
		Realm:         realm,
		AuthHandler:   authHandler(ctx, parts),
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

// authHandler admits a username Issue made, unexpired, of a live membership, or one IssueFound
// made, unexpired, and gives TURN the key its password makes; anything else is refused.
func authHandler(ctx context.Context, parts Parts) pionturn.AuthHandler {
	return func(username, _ string, _ net.Addr) ([]byte, bool) {
		held, expiresAt, err := parseUsername(username)
		if err != nil || !parts.Clock.Now().Before(expiresAt) ||
			!held.isFound && !parts.IsMember(held.membership.SessionID, held.membership.MemberID) {
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
