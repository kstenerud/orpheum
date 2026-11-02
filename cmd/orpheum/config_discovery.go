// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
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
	// HouseIPv6Networks are the house's public IPv6 networks, "2a0f:1000:1:2::/64", sent to the
	// discovery as configured house networks so it recognises the house's devices, for a house whose
	// devices cannot make the direct WebRTC pair the core learns them from; they go stale when the
	// provider moves the prefix (D356 §6).
	HouseIPv6Networks []string `toml:"house_ipv6_networks"`
}

// discoveryConfigKeys are the core's [discovery] keys, in the order the file is written.
func discoveryConfigKeys() []configKey {
	return []configKey{
		{
			tableDiscovery, "configured_addresses", "[]",
			"public addresses that reach the core which the machine does not hold, \"address:port\": a " +
				"port-forward, or the VPS's addresses through its tunnel; the discovery publishes them",
		},
		{
			tableDiscovery, "lan_addresses", "[]",
			"the house's addresses that reach the core which its interfaces do not show it, \"address:port\": " +
				"in a container, the machine's address at home and its published HTTPS port; the discovery " +
				"names them for devices at home",
		},
		{
			tableDiscovery, "send_interface_addresses", "true",
			"whether the core sends its discovery the addresses of the interfaces it sees; false in a " +
				"container, whose interfaces nothing at home reaches",
		},
		{
			tableDiscovery, "house_ipv6_networks", "[]",
			"the house's public IPv6 /64s, \"2a0f:1000:1:2::/64\", for a house whose devices cannot reach the " +
				"core directly over WebRTC to teach them; the discovery recognises devices at home by them. " +
				"Stale when the provider moves the prefix (D356 §6)",
		},
	}
}

// webRTCConfigKeys are the core's [webrtc] keys, in the order the file is written.
func webRTCConfigKeys() []configKey {
	return []configKey{
		{
			tableWebRTC, "udp_port", "0",
			"the UDP port every WebRTC connection's host candidates share, published at the house's " +
				"address on the same port, which the core then offers too (D300); 0 gives each connection " +
				"ports of its own",
		},
		{
			tableWebRTC, "stun_servers", "[]",
			"the STUN servers WebRTC connections find the core's public addresses through, \"stun:host:port\": " +
				"the core's own choice, never its discovery's, since an IPv6 address one finds names the " +
				"house's network, from which an admin administers (the Plex-chain review's L3); none finds " +
				"only the machine's own addresses",
		},
		{
			tableWebRTC, "max_connections", strconv.Itoa(rtcapi.DefaultMaxConnections),
			"the most WebRTC connections the core holds at once, of every network: a client opens up to four, " +
				"so 64 holds 16 remote devices, Watch Together guests among them; an offer past it is refused " +
				"(D348). Each network's share is the webrtc_connections_per_network setting",
		},
	}
}

// houseNetworks are the house's IPv6 networks the config names, which check has found house networks.
func (discovery DiscoveryConfig) houseNetworks() []netip.Prefix {
	networks := make([]netip.Prefix, 0, len(discovery.HouseIPv6Networks))
	for _, named := range discovery.HouseIPv6Networks {
		if network, err := netip.ParsePrefix(named); err == nil {
			networks = append(networks, network)
		}
	}
	return networks
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
// review's L3); Watch Together's browsers are handed them too, beside the discovery's (D406).
// MaxConnections is the most connections it holds at once, of every network (D348).
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
	for _, named := range discovery.HouseIPv6Networks {
		if network, err := netip.ParsePrefix(named); err != nil || !discoveryproto.IsHouseNetwork(network) {
			return fmt.Errorf("discovery.house_ipv6_networks %q: want a public IPv6 /64 of the house's, "+
				`"2a0f:1000:1:2::/64"`, named)
		}
	}
	if len(discovery.HouseIPv6Networks) > discoveryproto.MaxHouseNetworks {
		return fmt.Errorf("discovery.house_ipv6_networks names %d networks; a discovery counts at most %d",
			len(discovery.HouseIPv6Networks), discoveryproto.MaxHouseNetworks)
	}
	return nil
}
