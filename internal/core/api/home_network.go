// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
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
// rights, made directly to the core or over a WebRTC connection whose selected pair reaches it
// directly from one; one from anywhere else, or over a relayed connection, does not (D297 §6, D300
// §3). None named, no request administers.
func (server *Server) SetHomeNetworks(networks []netip.Prefix) {
	held := slices.Clone(networks)
	server.homeNetworks.Store(&held)
}

// markingHome notes in each request's context whether it came from a home network.
func (server *Server) markingHome(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		isHome := server.isHomeAddress(request.RemoteAddr)
		if rtcapi.IsOverWebRTC(request.Context()) {
			isHome = isHomeOverWebRTC(isHome, request.RemoteAddr, rtcapi.OwnIPv6NetworksOf(request.Context()),
				rtcapi.PathOf(request.Context()))
		}
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), homeKey{}, isHome)))
	})
}

// isHomeOverWebRTC is whether a request over WebRTC is from home: its remote address, the client's end
// of the pair the core's own end selected, which no page writes, is in a home network, or in one of
// the core's own public IPv6 networks on that connection, the house's IPv6 network (D297 §6); and no
// relay is between, whose address it would be otherwise (D300 §3).
func isHomeOverWebRTC(isHomeAddress bool, remote string, ownNetworks []netip.Prefix, path rtcapi.Path) bool {
	return (isHomeAddress || isInAny(remote, ownNetworks)) && isDirectPath(path)
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

// isDirectPath is whether a WebRTC connection's path reaches the core with no relay between.
func isDirectPath(path rtcapi.Path) bool {
	return path != rtcapi.PathRelayed && path != rtcapi.PathFailed
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
