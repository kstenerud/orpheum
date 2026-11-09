// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"net/netip"

	"github.com/kstenerud/orpheum/internal/core/home"
)

// homeNetworks are HomeNetworks parsed; check refused any that is not one.
func (server ServerConfig) homeNetworks() []netip.Prefix {
	return parsedNetworks(server.HomeNetworks)
}

// notHomeNetworks are NotHomeNetworks parsed; check refused any that is not one.
func (server ServerConfig) notHomeNetworks() []netip.Prefix {
	return parsedNetworks(server.NotHomeNetworks)
}

// parsedNetworks are the networks named, masked, leaving out any that is not one.
func parsedNetworks(named []string) []netip.Prefix {
	networks := make([]netip.Prefix, 0, len(named))
	for _, each := range named {
		if network, err := netip.ParsePrefix(each); err == nil {
			networks = append(networks, network.Masked())
		}
	}
	return networks
}

// homeNetworksOf is the home rule's networks as the config names them, with interfaces the networks
// of the core's own interfaces (D452 §1).
func (server ServerConfig) homeNetworksOf(interfaces []netip.Prefix) home.Networks {
	return home.Networks{
		Listed:        server.homeNetworks(),
		PrivateIsHome: server.HomeIsPrivate,
		NotHome:       server.notHomeNetworks(),
		Interfaces:    interfaces,
	}
}

// checkHomeNetworks refuses a home or never-home network that is not one.
func (server ServerConfig) checkHomeNetworks() []error {
	var problems []error
	for _, each := range []struct {
		key      string
		networks []string
	}{{"server.home_networks", server.HomeNetworks}, {"server.not_home_networks", server.NotHomeNetworks}} {
		for _, named := range each.networks {
			if _, err := netip.ParsePrefix(named); err != nil {
				problems = append(problems, fmt.Errorf("%s %q: want a network, \"192.168.1.0/24\"", each.key, named))
			}
		}
	}
	return problems
}
