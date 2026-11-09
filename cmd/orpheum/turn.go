// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/turn"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// startRelay serves the voice relay on turn.listen, making its shared secret first when there is
// none (D194, D243 §8); nothing for a server with no turn.public_address. It returns what stops it.
func startRelay(ctx context.Context, config Config, opened openedServices, logger *slog.Logger) (func() error, error) {
	if !config.TURN.isOn() {
		return func() error { return nil }, nil
	}
	if err := turn.EnsureSecret(ctx, opened.secrets, rand.Reader); err != nil {
		return nil, err
	}
	relayConfig := turn.Config{
		Listen: config.TURN.Listen, PublicAddress: net.ParseIP(config.TURN.PublicAddress),
		RelayPortMin: uint16(config.TURN.RelayPortMin), //nolint:gosec // TURNConfig.check: a port
		RelayPortMax: uint16(config.TURN.RelayPortMax), //nolint:gosec // TURNConfig.check: a port
	}
	relay, err := turn.Start(ctx, relayConfig, turn.Parts{
		Secrets: opened.secrets, IsMember: opened.together.IsMember, Clock: clock.System{}, Logger: logger,
	})
	if err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "relaying voice", slogx.EventKey, "voiceRelayStarted", "address", relay.Address().String())
	return relay.Close, nil
}

// relayAddressOf is the voice relay's public host:port, turn.public_address and turn.listen's port;
// empty when the server relays nothing.
func relayAddressOf(config Config) string {
	if !config.TURN.isOn() {
		return ""
	}
	_, port, err := net.SplitHostPort(config.TURN.Listen)
	if err != nil {
		return "" // TURNConfig.check refused it before the server started
	}
	return net.JoinHostPort(config.TURN.PublicAddress, port)
}

// newICEIssuer hands out the ICE servers: to Watch Together's members, the STUN servers that find
// each browser's public address, the core's own and its discovery's (D406), and the relay where there
// is one; to the discovery, the relay alone. A listener who has not pressed talk offers no address but
// an mDNS name without STUN. A server with no turn.public_address relays nothing (D243 §8).
func newICEIssuer(config Config, opened openedServices, logger *slog.Logger) *turn.Issuer {
	return &turn.Issuer{
		Address: relayAddressOf(config), Secrets: opened.secrets, Settings: opened.settings, Logger: logger,
		STUN: func() []string {
			var named []discoveryproto.ICEServer
			if opened.discovery != nil {
				named = opened.discovery.Status().ICEServers
			}
			return stunServersFor(config.WebRTC.STUNServers, named)
		},
	}
}
