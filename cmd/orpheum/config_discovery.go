// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// DiscoveryConfig is what reaches the core that its discovery is told of: addresses the machine does
// not hold. How long the core waits between its calls are settings (discovery_retry_*_seconds; the
// constants sweep, 2026-10-20).
type DiscoveryConfig struct {
	// ConfiguredAddresses are public addresses that reach the core which the machine does not hold,
	// "address:port": the admin's port-forward, or their own VPS through its tunnel, sent to the
	// discovery as configured candidates (the protocol's §2.4; discovery-core.md K5; N11).
	ConfiguredAddresses []string `toml:"configured_addresses"`
	// LANAddresses are the house's addresses that reach the core which the machine's interfaces do
	// not show it, "address:port": in a container, the machine's own address on the house's network
	// and the port HTTPS is published on there, which the discovery names for devices at home, so they
	// reach the core in the house over HTTPS, not out through the VPS and back (the owner, 2026-10-14).
	LANAddresses []string `toml:"lan_addresses"`
	// SendInterfaceAddresses is whether the core sends the addresses of the interfaces it sees: true
	// for a core run on its machine, false in a container, whose interfaces are its own network's and
	// its tunnel's, which nothing at home reaches; there the deployment names its addresses (the owner,
	// 2026-10-14).
	SendInterfaceAddresses bool `toml:"send_interface_addresses"`
}

// configuredCandidates are the configured addresses, public and the house's, as the discovery is
// sent them.
func (discovery DiscoveryConfig) configuredCandidates() []discoveryproto.Candidate {
	named := append(slices.Clone(discovery.ConfiguredAddresses), discovery.LANAddresses...)
	candidates := make([]discoveryproto.Candidate, 0, len(named))
	for _, configured := range named {
		if endpoint, err := netip.ParseAddrPort(configured); err == nil {
			candidates = append(candidates, discoveryproto.Candidate{
				Address: endpoint.Addr().Unmap().String(), Port: int(endpoint.Port()),
				Source: discoveryproto.SourceConfigured,
			})
		}
	}
	return candidates
}

// lanIPs are the house's addresses the LAN addresses name, which the core's WebRTC offers too.
func (discovery DiscoveryConfig) lanIPs() []netip.Addr {
	addresses := make([]netip.Addr, 0, len(discovery.LANAddresses))
	for _, named := range discovery.LANAddresses {
		if endpoint, err := netip.ParseAddrPort(named); err == nil {
			addresses = append(addresses, endpoint.Addr().Unmap())
		}
	}
	return addresses
}

// WebRTCConfig is where the core takes WebRTC from devices at home: the UDP port its connections'
// host candidates share, which a deployment publishes at the house's address on the same port, so a
// device in the house reaches the core directly over WebRTC (D300 §1). Zero gives each connection
// ports of its own, which nothing at home reaches in a container. STUNServers are the servers its
// connections find its public addresses through: its own, not its discovery's (the Plex-chain
// review's L3). MaxConnections is the most connections it holds at once, of every network (D348).
type WebRTCConfig struct {
	UDPPort        int      `toml:"udp_port"`
	STUNServers    []string `toml:"stun_servers"`
	MaxConnections int      `toml:"max_connections"`
}

// check refuses a port that is not one, a STUN server that is not a stun: URL, and a limit on
// connections that allows none.
func (webrtc WebRTCConfig) check() error {
	var problems []error
	if webrtc.UDPPort < 0 || webrtc.UDPPort > 65535 {
		problems = append(problems, fmt.Errorf("webrtc.udp_port %d: want a UDP port, or 0 for none", webrtc.UDPPort))
	}
	if webrtc.MaxConnections < 1 {
		problems = append(problems,
			fmt.Errorf("webrtc.max_connections %d: want at least 1", webrtc.MaxConnections))
	}
	for _, server := range webrtc.STUNServers {
		if !strings.HasPrefix(server, "stun:") && !strings.HasPrefix(server, "stuns:") {
			problems = append(problems,
				fmt.Errorf("webrtc.stun_servers %q: want a STUN server, \"stun:stun.example.org:3478\"", server))
		}
	}
	return errors.Join(problems...)
}

// check refuses a configured address that is not a public address with a port: a private one is the
// machine's own, sent already.
func (discovery DiscoveryConfig) check() error {
	for _, configured := range discovery.ConfiguredAddresses {
		endpoint, err := netip.ParseAddrPort(configured)
		if err != nil || endpoint.Port() == 0 || !discoveryproto.IsPublic(endpoint.Addr()) {
			return fmt.Errorf("discovery.configured_addresses %q: want a public address and its port, "+
				`"<IPv4>:443" or "[<IPv6>]:443"`, configured)
		}
	}
	for _, named := range discovery.LANAddresses {
		endpoint, err := netip.ParseAddrPort(named)
		if err != nil || endpoint.Port() == 0 || !endpoint.Addr().Unmap().IsPrivate() {
			return fmt.Errorf("discovery.lan_addresses %q: want an address on the house's network and its "+
				`port, "192.168.1.20:8443"`, named)
		}
	}
	return nil
}
