// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"net"
	"testing"

	"github.com/kstenerud/orpheum/internal/discoveryproto"
)

// The heartbeat's own candidates are the machine's global addresses, a private one marked as
// reachable only at home; loopback and link-local ones are left out (D273).
func TestCandidatesOf_SendsGlobalAddressesAndMarksPrivateOnesAsHome(t *testing.T) {
	t.Parallel()
	var addresses []net.Addr
	for _, cidr := range []string{
		"127.0.0.1/8", "::1/128", "fe80::1/64", "192.168.1.20/24", "2001:db8::20/64", "fd00::20/64",
	} {
		ip, network, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatal(err)
		}
		addresses = append(addresses, &net.IPNet{IP: ip, Mask: network.Mask})
	}
	got := candidatesOf(addresses, 8443)
	want := []discoveryproto.Candidate{
		{Address: "192.168.1.20", Port: 8443, Source: discoveryproto.SourceInterface, Scope: discoveryproto.ScopeLAN},
		{Address: "2001:db8::20", Port: 8443, Source: discoveryproto.SourceInterface},
		{Address: "fd00::20", Port: 8443, Source: discoveryproto.SourceInterface, Scope: discoveryproto.ScopeLAN},
	}
	if len(got) != len(want) {
		t.Fatalf("candidates = %+v; want %+v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("candidate %d = %+v; want %+v", index, got[index], want[index])
		}
	}
}
