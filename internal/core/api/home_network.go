// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

// homeKey is the context key of whether a request came from one of the house's networks.
type homeKey struct{}

// SetHomeNetworks names the networks an admin administers from: a request from one carries an admin's
// rights, made directly to the core; over WebRTC, rtcapi judges it by its own home networks
// (rtcapi.IsDirectlyFromHome, D439). One from anywhere else does not (D297 §6). None named, no
// request administers.
func (server *Server) SetHomeNetworks(networks []netip.Prefix) {
	held := slices.Clone(networks)
	server.homeNetworks.Store(&held)
}

// SetThisMachinePort names the port only the server's own machine reaches: the deployment publishes it
// on the host's loopback alone, so a request on it is from that machine, and so from home, though
// Docker hands it the core from its gateway, as every request the machine makes to its own published
// ports (D334 §4). 0 names none.
func (server *Server) SetThisMachinePort(port int) {
	server.thisMachinePort.Store(int32(port)) //nolint:gosec // G115: a port, which int32 holds
}

// thisMachineHint is what the refusal of an admin away from home adds when the server has a port for
// its own machine: on that machine, the browser opened the house's address and arrived from Docker's
// gateway, and this is the address that administers.
func (server *Server) thisMachineHint() string {
	port := server.thisMachinePort.Load()
	if port == 0 {
		return ""
	}
	return fmt.Sprintf("; on the server's own computer, open http://localhost:%d", port)
}

// markingHome notes in each request's context whether it came from a home network, or through the
// port only the server's own machine reaches; for a request over WebRTC, rtcapi's judgement of its
// connection's pair, which counts a direct pair from home alone (D439).
func (server *Server) markingHome(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		isHome := server.isHomeAddress(request.RemoteAddr) || server.isOnThisMachinesPort(request)
		if rtcapi.IsOverWebRTC(request.Context()) {
			isHome = rtcapi.IsDirectlyFromHome(request.Context())
		}
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), homeKey{}, isHome)))
	})
}

// isInAny is whether remote, a "host:port", is in one of networks.
func isInAny(remote string, networks []netip.Prefix) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	address = address.Unmap().WithZone("")
	return slices.ContainsFunc(networks, func(network netip.Prefix) bool { return network.Contains(address) })
}

// isOnThisMachinesPort is whether request came in on the port only the server's own machine reaches.
func (server *Server) isOnThisMachinesPort(request *http.Request) bool {
	port := server.thisMachinePort.Load()
	local, isAddress := request.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if port == 0 || !isAddress {
		return false
	}
	address, err := netip.ParseAddrPort(local.String())
	return err == nil && int32(address.Port()) == port
}

// isHomeAddress is whether remote, a request's "host:port", is in one of the home networks.
func (server *Server) isHomeAddress(remote string) bool {
	networks := server.homeNetworks.Load()
	if networks == nil {
		return false
	}
	return isInAny(remote, *networks)
}

// isFromHome is whether the request whose context ctx is came from a home network.
func isFromHome(ctx context.Context) bool {
	isHome, isMarked := ctx.Value(homeKey{}).(bool)
	return isMarked && isHome
}

// pathOf names the way the request whose context ctx is reached the core, for its log: over WebRTC
// from home or from away, directly from a home network, or directly from anywhere else; the last is a
// page at home that came the long way, through a public name, and so is offered no admin (the owner,
// 2026-10-15).
func pathOf(ctx context.Context) string {
	switch {
	case rtcapi.IsOverWebRTC(ctx) && isFromHome(ctx):
		return "webrtc from home"
	case rtcapi.IsOverWebRTC(ctx):
		return "webrtc"
	case isFromHome(ctx):
		return "direct from home"
	default:
		return "direct from away"
	}
}

// administersHere is whether principal administers on the request whose context ctx is: an admin's
// session from home, or from anywhere when the admin set it so (D297 §6, D302). The server alone
// decides it, from where the request came and the person's stored setting; nothing a page sends.
func administersHere(ctx context.Context, principal auth.Principal) bool {
	return principal.IsAdmin() && (isFromHome(ctx) || principal.AdministersFromAway())
}
