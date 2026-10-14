// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"log/slog"
	"net"
	"net/http"
	"testing"
)

// A relayed connection checks no address of the core's own networks until a development build says
// it may, as its browser tests need with everything on one machine.
func TestServer_ARelayedConnectionChecksTheCoresOwnNetworksOnlyWhenTold(t *testing.T) {
	t.Parallel()
	server := New(t.Context(), http.NotFoundHandler(), slog.New(slog.DiscardHandler))
	loopback, private, public := net.ParseIP("127.0.0.1"), net.ParseIP("192.168.1.20"), net.ParseIP("2001:db8::20")
	if server.checksRelayed(loopback) || server.checksRelayed(private) || server.AcceptsAnyRelayedAddress() {
		t.Error("a new server checks a relayed connection's loopback or private address")
	}
	if !server.checksRelayed(public) {
		t.Error("a new server refuses a relayed connection's public address")
	}
	server.AcceptAnyRelayedAddress()
	if !server.checksRelayed(loopback) || !server.checksRelayed(private) || !server.AcceptsAnyRelayedAddress() {
		t.Error("told to, the server still refuses a relayed connection's loopback or private address")
	}
}
