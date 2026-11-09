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
	"github.com/kstenerud/orpheum/internal/core/portmap"
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
	// HouseIPv6Networks are the house's public IPv6 networks, "2a0f:5000:1:2::/64", sent to the
	// discovery as configured house networks so it recognises the house's devices, for a house whose
	// devices cannot make the direct WebRTC pair the core learns them from; they go stale when the
	// provider moves the prefix (D356 §6).
	HouseIPv6Networks []string `toml:"house_ipv6_networks"`
	// Router is the house's router, which port mapping asks to forward the core's HTTPS port: in a
	// container, whose default route is its own network's bridge, the machine's default gateway, which
	// the deployment names; empty asks the default route's (item 15 of the owner's comments of
	// 2026-11-02).
	Router string `toml:"router"`
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
			"the house's public IPv6 /64s, \"2a0f:5000:1:2::/64\", for a house whose devices cannot reach the " +
				"core directly over WebRTC to teach them; the discovery recognises devices at home by them. " +
				"Stale when the provider moves the prefix (D356 §6)",
		},
		{
			tableDiscovery, "router", `""`,
			"the house's router, which port mapping asks to forward the core's HTTPS port: in a container, " +
				"this machine's default gateway, which the container cannot see; empty asks the default route's",
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
				"(D348). Each network's share is the webrtcConnectionsPerNetwork setting",
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

// checkRouter refuses a router that is not an IPv4 address on the house's network.
func (discovery DiscoveryConfig) checkRouter() error {
	if router, err := netip.ParseAddr(discovery.Router); discovery.Router != "" &&
		(err != nil || !router.Is4() || !router.IsPrivate()) {
		return fmt.Errorf("discovery.router %q: want the router's address on the house's network, "+
			`"192.168.1.1"`, discovery.Router)
	}
	return nil
}

// portMappingGateway is the router port mapping asks: the one the config names, else the default
// route's, except in a container, where that is its own network's bridge (item 15 of the owner's
// comments of 2026-11-02).
func (discovery DiscoveryConfig) portMappingGateway() func() (netip.Addr, error) {
	if router, err := netip.ParseAddr(discovery.Router); err == nil {
		return func() (netip.Addr, error) { return router, nil }
	}
	if !discovery.SendInterfaceAddresses {
		return func() (netip.Addr, error) { return netip.Addr{}, errRouterUnnamed }
	}
	return defaultGateway
}

// searchAddress is where UPnP's search for the router goes: to the router the config names, since a
// search sent to SSDP's group from a container's network never leaves the machine; else to the group.
func (discovery DiscoveryConfig) searchAddress() string {
	if router, err := netip.ParseAddr(discovery.Router); err == nil {
		return netip.AddrPortFrom(router, portmap.SSDPPort).String()
	}
	return portmap.SSDPAddress
}

// forwardTo is where port mapping asks the router to forward, in a container: the first IPv4 LAN
// address, the machine's address at home and its published HTTPS port; none outside a container,
// where the router forwards to the machine's own address.
func (discovery DiscoveryConfig) forwardTo() netip.AddrPort {
	if discovery.SendInterfaceAddresses {
		return netip.AddrPort{}
	}
	for _, named := range discovery.LANAddresses {
		if endpoint, err := netip.ParseAddrPort(named); err == nil && endpoint.Addr().Unmap().Is4() {
			return netip.AddrPortFrom(endpoint.Addr().Unmap(), endpoint.Port())
		}
	}
	return netip.AddrPort{}
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
				`"2a0f:5000:1:2::/64"`, named)
		}
	}
	if err := discovery.checkRouter(); err != nil {
		return err
	}
	if len(discovery.HouseIPv6Networks) > discoveryproto.MaxHouseNetworks {
		return fmt.Errorf("discovery.house_ipv6_networks names %d networks; a discovery counts at most %d",
			len(discovery.HouseIPv6Networks), discoveryproto.MaxHouseNetworks)
	}
	return nil
}
