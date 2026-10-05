// SPDX-License-Identifier: GPL-3.0-or-later

package turn

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	pionturn "github.com/pion/turn/v4"

	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// realm is the relay's realm, which a TURN client hashes its credentials with.
const realm = "orpheum"

// Config is where the relay listens and what it tells the browsers (D243 §8).
type Config struct {
	// Listen is the UDP address TURN is served on, ":3478".
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

// Start serves TURN on config.Listen, admitting the credentials Issue makes while their membership
// lasts and until they expire. The relay reads the shared secret, and logs, under ctx.
func Start(ctx context.Context, config Config, parts Parts) (*Relay, error) {
	connection, err := (&net.ListenConfig{}).ListenPacket(ctx, "udp", config.Listen)
	if err != nil {
		return nil, fmt.Errorf("listen for TURN on %s: %w", config.Listen, err)
	}
	server, err := pionturn.NewServer(pionturn.ServerConfig{
		Realm:         realm,
		AuthHandler:   authHandler(ctx, parts),
		LoggerFactory: slogFactory{ctx: ctx, logger: parts.Logger},
		PacketConnConfigs: []pionturn.PacketConnConfig{{
			PacketConn: connection,
			RelayAddressGenerator: &pionturn.RelayAddressGeneratorPortRange{
				RelayAddress: config.PublicAddress, Address: "0.0.0.0",
				MinPort: config.RelayPortMin, MaxPort: config.RelayPortMax,
			},
		}},
	})
	if err != nil {
		_ = connection.Close() //nolint:errcheck // the start's error is the one to report
		return nil, fmt.Errorf("start the TURN relay: %w", err)
	}
	return &Relay{server: server, address: connection.LocalAddr()}, nil
}

// Address is where the relay listens: config.Listen, its port chosen when that named none.
func (relay *Relay) Address() net.Addr { return relay.address }

// Close stops the relay and every allocation on it.
func (relay *Relay) Close() error {
	if err := relay.server.Close(); err != nil {
		return fmt.Errorf("stop the TURN relay: %w", err)
	}
	return nil
}

// authHandler admits a username Issue made, unexpired, of a live membership, and gives TURN the key
// its password makes; anything else is refused.
func authHandler(ctx context.Context, parts Parts) pionturn.AuthHandler {
	return func(username, _ string, _ net.Addr) ([]byte, bool) {
		membership, expiresAt, err := parseUsername(username)
		if err != nil || !parts.Clock.Now().Before(expiresAt) ||
			!parts.IsMember(membership.SessionID, membership.MemberID) {
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
