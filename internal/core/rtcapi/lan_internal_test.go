// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"net"
	"slices"
	"testing"
)

// The mux names one address of each family, not loopback and not link-local when another is there:
// two of a family read one socket and a check went to either, so the core never started DTLS (D441).
func TestListenAddressPerFamily_IsOneOfEachFamilyNeitherLoopbackNorLinkLocal(t *testing.T) {
	t.Parallel()
	udpAt := func(ip string) net.Addr { return &net.UDPAddr{IP: net.ParseIP(ip), Port: 8443} }
	got := listenAddressPerFamily([]net.Addr{
		udpAt("127.0.0.1"), udpAt("::1"), udpAt("fe80::68c4:c6ff:feb0:ed05"), udpAt("172.18.0.2"),
		udpAt("fd77:5e:0:43df::2"), udpAt("169.254.3.4"), udpAt("10.0.0.9"), udpAt("2001:db8::7"),
	})
	want := []string{"172.18.0.2:8443", "[fd77:5e:0:43df::2]:8443"}
	if names := addressNames(got); !slices.Equal(names, want) {
		t.Errorf("the mux names %v; want %v", names, want)
	}
	if names := addressNames(listenAddressPerFamily([]net.Addr{udpAt("::1"), udpAt("fe80::1")})); !slices.Equal(
		names, []string{"[fe80::1]:8443"}) {
		t.Errorf("with only a link-local IPv6 address the mux names %v; want that address", names)
	}
}

func addressNames(addresses []net.Addr) []string {
	names := make([]string, 0, len(addresses))
	for _, address := range addresses {
		names = append(names, address.String())
	}
	return names
}
