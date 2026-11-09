// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/home"
)

// Ports a Host that names none means: plain HTTP's and HTTPS's.
const (
	plainHTTPDefaultPort = 80
	httpsDefaultPort     = 443
)

// keepAddressAtHome is the server's address at home (D452 §3, §4): the config's first LAN address,
// which wins, else the one the settings keep, else, for a core sharing the machine's network, the
// default route's interface's. The API learns it from the requests at home that name the server on
// its plain or HTTPS port, and WebRTC offers it as the host candidate of its port at home from the
// next offer after each change.
func keepAddressAtHome(
	ctx context.Context, config Config, server *api.Server, opened openedServices, logger *slog.Logger,
) (*home.AddressAtHome, error) {
	parts := home.AddressParts{
		Rule: server.Home(), Settings: opened.settings, Database: opened.database, Logger: logger,
		Resolve: systemResolve, Interfaces: systemInterfaces,
	}
	if configured := config.Discovery.lanIPs(); len(configured) > 0 {
		parts.Configured = configured[0]
	}
	if config.Server.AddressAtHomeFromDefaultRoute {
		parts.DefaultRoute = defaultRouteAddress
	}
	keeper, err := home.NewAddressAtHome(ctx, parts)
	if err != nil {
		return nil, err
	}
	server.SetAddressAtHome(keeper, teachingPorts(config)...)
	if rtc := server.RTC(); rtc != nil && !parts.Configured.IsValid() {
		keeper.OnChange(func(address netip.Addr) {
			if err := rtc.SetLANAddresses(addressesOf(address)); err != nil {
				logger.ErrorContext(ctx, "WebRTC at home goes on offering the old address at home", "err", err)
			}
		})
	}
	return keeper, nil
}

// teachingPorts are the ports a request naming the server's address at home teaches it on: the plain
// listener's and HTTPS's, and the ones a Host naming no port means; never this machine's own port.
func teachingPorts(config Config) []uint16 {
	ports := []uint16{plainHTTPDefaultPort, httpsDefaultPort}
	for _, port := range []int{portOf(config.Server.Listen), httpsPort(config)} {
		if port > 0 && port <= 0xFFFF {
			ports = append(ports, uint16(port))
		}
	}
	return ports
}

// addressesOf is address alone, or none when it is invalid.
func addressesOf(address netip.Addr) []netip.Addr {
	if !address.IsValid() {
		return nil
	}
	return []netip.Addr{address}
}

// portOf is the port listen, "host:port", names; 0 for none.
func portOf(listen string) int {
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return 0
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return number
}

// systemInterfaces are the machine's interfaces that are up, with their addresses; none when they
// cannot be read.
func systemInterfaces() []home.Interface {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	found := make([]home.Interface, 0, len(interfaces))
	for _, each := range interfaces {
		if each.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, err := each.Addrs()
		if err != nil {
			continue
		}
		held := home.Interface{Name: each.Name}
		for _, address := range addresses {
			if network, isNetwork := address.(*net.IPNet); isNetwork {
				if parsed, isAddress := netip.AddrFromSlice(network.IP); isAddress {
					held.Addresses = append(held.Addresses, parsed.Unmap())
				}
			}
		}
		found = append(found, held)
	}
	return found
}

// systemResolve looks name up with the machine's resolver.
func systemResolve(ctx context.Context, name string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip4", name)
}

// defaultRouteAddress is the address of this machine's interface carrying the default route, from its
// routing table (home.DefaultRouteAddress); invalid when the table cannot be read.
func defaultRouteAddress() netip.Addr {
	table, err := os.ReadFile(routesFile)
	if err != nil {
		return netip.Addr{}
	}
	return home.DefaultRouteAddress(string(table), systemInterfaces())
}
