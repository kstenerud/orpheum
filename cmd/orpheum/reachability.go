// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"crypto/rand"
	"net"
	"time"

	"github.com/kstenerud/orpheum/internal/core/reachability"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// reachabilityDialWait is how long the reachability check waits to reach the relay's port: a VPS
// a continent away answers in well under a second.
const reachabilityDialWait = 5 * time.Second

// newReachability is the System page's check of the server's public name and the voice relay's
// port, from outside in (D243 §7): either is left unchecked when the config has none.
func newReachability(config Config, opened openedServices) *reachability.Checker {
	relayAddress := ""
	if issuer := newRelayIssuer(config, opened); issuer != nil {
		relayAddress = issuer.Address
	}
	return reachability.New(reachability.Parts{
		Name: config.HTTPS.Domain, RelayAddress: relayAddress, HTTP: opened.jobServices.outboundClient,
		Resolver: net.DefaultResolver, Dialer: &net.Dialer{Timeout: reachabilityDialWait},
		Clock: clock.System{}, Random: rand.Reader,
	})
}
