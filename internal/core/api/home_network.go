// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/rtcapi"
)

// homeKey is the context key of whether a request came from one of the house's networks.
type homeKey struct{}

// SetHomeNetworks names the networks an admin administers from: a request from one, made directly to
// the core, carries an admin's rights; one from anywhere else, or over WebRTC, whose source the core
// cannot know, does not (D297 §6). None named, no request administers.
func (server *Server) SetHomeNetworks(networks []netip.Prefix) {
	held := slices.Clone(networks)
	server.homeNetworks.Store(&held)
}

// markingHome notes in each request's context whether it came from a home network.
func (server *Server) markingHome(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		isHome := !rtcapi.IsOverWebRTC(request.Context()) && server.isHomeAddress(request.RemoteAddr)
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), homeKey{}, isHome)))
	})
}

// isHomeAddress is whether remote, a request's "host:port", is in one of the home networks.
func (server *Server) isHomeAddress(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	address = address.Unmap().WithZone("")
	networks := server.homeNetworks.Load()
	if networks == nil {
		return false
	}
	return slices.ContainsFunc(*networks, func(network netip.Prefix) bool {
		return network.Contains(address)
	})
}

// isFromHome is whether the request whose context ctx is came from a home network.
func isFromHome(ctx context.Context) bool {
	isHome, isMarked := ctx.Value(homeKey{}).(bool)
	return isMarked && isHome
}

// pathOf names the way the request whose context ctx is reached the core, for its log: over WebRTC,
// directly from a home network, or directly from anywhere else; the last is a page at home that came
// the long way, through a public name, and so is offered no admin (the owner, 2026-10-15).
func pathOf(ctx context.Context) string {
	switch {
	case rtcapi.IsOverWebRTC(ctx):
		return "webrtc"
	case isFromHome(ctx):
		return "direct from home"
	default:
		return "direct from away"
	}
}
