// SPDX-License-Identifier: GPL-3.0-or-later

package portmap

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"strings"
)

// errNoDefaultRoute is a machine whose routing table has no default IPv4 route.
var errNoDefaultRoute = errors.New("the machine has no default IPv4 route, so no router to ask")

// GatewayOf is the gateway of the first default route in a /proc/net/route table: its destination
// and mask 0, its gateway's address written in hex, least significant byte first.
func GatewayOf(table string) (netip.Addr, error) {
	scanner := bufio.NewScanner(strings.NewReader(table))
	scanner.Scan() // the heading
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 || fields[1] != "00000000" || fields[7] != "00000000" {
			continue
		}
		raw, err := hex.DecodeString(fields[2])
		if err != nil || len(raw) != 4 {
			continue
		}
		var address [4]byte
		binary.LittleEndian.PutUint32(address[:], binary.BigEndian.Uint32(raw))
		if gateway := netip.AddrFrom4(address); !gateway.IsUnspecified() {
			return gateway, nil
		}
	}
	return netip.Addr{}, errNoDefaultRoute
}

// GlobalIPv6Addresses are the machine's IPv6 addresses the internet can reach: global unicast,
// not unique-local.
func GlobalIPv6Addresses() []netip.Addr {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var global []netip.Addr
	for _, address := range addresses {
		prefix, err := netip.ParsePrefix(address.String())
		if err != nil {
			continue
		}
		if ip := prefix.Addr(); ip.Is6() && !ip.Is4In6() && ip.IsGlobalUnicast() && !ip.IsPrivate() {
			global = append(global, ip)
		}
	}
	return global
}
