// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net"

	"github.com/kstenerud/orpheum/internal/core/turn"
	"github.com/kstenerud/orpheum/internal/platform/clock"
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
	logger.InfoContext(ctx, "relaying voice", "address", relay.Address().String())
	return relay.Close, nil
}
