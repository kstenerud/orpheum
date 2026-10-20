// SPDX-License-Identifier: AGPL-3.0-or-later

package outbound

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"reflect"
	"slices"
	"sync"
	"testing"
)

// fakeNetwork resolves every host to addresses and connects to those in reachable, recording each
// address dialled.
type fakeNetwork struct {
	mutex     sync.Mutex
	addresses []netip.Addr
	reachable []string
	dialled   []string
}

func (network *fakeNetwork) lookup(context.Context, string, string) ([]netip.Addr, error) {
	return network.addresses, nil
}

func (network *fakeNetwork) dial(_ context.Context, _, address string) (net.Conn, error) {
	network.mutex.Lock()
	network.dialled = append(network.dialled, address)
	network.mutex.Unlock()
	if !slices.Contains(network.reachable, address) {
		return nil, errors.New("unreachable")
	}
	near, far := net.Pipe()
	_ = far.Close() //nolint:errcheck // the far end is never used
	return near, nil
}

// A host with both families is dialled over IPv6 first, and over IPv4 at once when IPv6 fails or the
// host has none; an address given as such is dialled as it is. From a private IPv6 address Go put IPv4
// first, and a core in Docker reached the paid service over the house's shared IPv4 (nas.example,
// 2026-10-20).
func TestIPv6First_DialsIPv6FirstAndIPv4WhenItFails(t *testing.T) {
	t.Parallel()
	both := []netip.Addr{netip.MustParseAddr("104.21.12.109"), netip.MustParseAddr("2606:4700:3036::6815:c6d")}
	for _, each := range []struct {
		name      string
		addresses []netip.Addr
		reachable []string
		asked     string
		want      string
	}{
		{
			"IPv6 answering", both,
			[]string{"[2606:4700:3036::6815:c6d]:443", "104.21.12.109:443"},
			"api.example:443", "[2606:4700:3036::6815:c6d]:443",
		},
		{"IPv6 failing", both, []string{"104.21.12.109:443"}, "api.example:443", "104.21.12.109:443"},
		{"IPv4 alone", both[:1], []string{"104.21.12.109:443"}, "api.example:443", "104.21.12.109:443"},
		{"an address", nil, []string{"192.0.2.1:443"}, "192.0.2.1:443", "192.0.2.1:443"},
	} {
		network := &fakeNetwork{addresses: each.addresses, reachable: each.reachable}
		connection, err := ipv6First(network.lookup, network.dial)(t.Context(), "tcp", each.asked)
		if err != nil {
			t.Errorf("%s: %v", each.name, err)
			continue
		}
		_ = connection.Close() //nolint:errcheck // a pipe
		network.mutex.Lock()
		if len(network.dialled) == 0 || network.dialled[len(network.dialled)-1] != each.want ||
			(each.name == "IPv6 answering" && len(network.dialled) != 1) {
			t.Errorf("%s: dialled %v; want %s, IPv6 first", each.name, network.dialled, each.want)
		}
		network.mutex.Unlock()
	}
	unreachable := &fakeNetwork{addresses: both}
	if _, err := ipv6First(unreachable.lookup, unreachable.dial)(t.Context(), "tcp", "api.example:443"); err == nil {
		t.Error("a host reachable at neither family connected")
	}
}

// The client's transport dials through ipv6First, not Go's default dialer, which put IPv4 first.
func TestNewClient_DialsThroughIPv6First(t *testing.T) {
	t.Parallel()
	client, err := NewClient(0, "test")
	if err != nil {
		t.Fatal(err)
	}
	identified, isIdentified := client.Transport.(userAgentTransport)
	// Every dialer ipv6First makes is the one closure's code, whatever it was made over.
	want := reflect.ValueOf(ipv6First(nil, nil)).Pointer()
	if !isIdentified || identified.DialContext == nil || reflect.ValueOf(identified.DialContext).Pointer() != want {
		t.Errorf("the client's transport %T dials with Go's default dialer; want ipv6First", client.Transport)
	}
}
