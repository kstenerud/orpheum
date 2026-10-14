// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// localSuffixes are the names public DNS never answers, so no page out on the internet can rebind one
// to the core's address: mDNS's, the home network's (RFC 8375), the private-use one ICANN set aside in
// 2024, and the .lan routers commonly give [I: a list taken past the security audit's S15, so a
// household's own name for its server keeps working].
func localSuffixes() []string {
	return []string{".local", ".lan", ".home.arpa", ".internal", ".localdomain"}
}

// LANHandler is Handler for the core's plain-HTTP listener on the house's network: a request naming
// a host a rebinding page could have pointed at the core is refused with 421, and anything else
// answered (the discovery protocol's §7 item 10; S15). It takes an IP literal, localhost, a name
// public DNS never answers, a name the config gives (names), and a name under the core's own at its
// discovery.
func (server *Server) LANHandler(names []string) http.Handler {
	configured := make([]string, 0, len(names))
	for _, name := range names {
		if name != "" {
			configured = append(configured, strings.ToLower(name))
		}
	}
	handler := server.Handler()
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !server.isLANHost(request.Host, configured) {
			http.Error(writer, "this server does not answer to that name", http.StatusMisdirectedRequest)
			return
		}
		handler.ServeHTTP(writer, request)
	})
}

// isLANHost is whether host, a request's Host with or without its port, may reach the plain listener.
func (server *Server) isLANHost(host string, configured []string) bool {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	switch {
	case host == "" || host == "localhost" || !strings.Contains(host, "."):
		return host != ""
	case slices.Contains(configured, host):
		return true
	}
	for _, suffix := range localSuffixes() {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return server.isDiscoveryName(host)
}

// isDiscoveryName is whether host is a name under the core's own at its discovery, which only the
// discovery writes (the protocol's §2.7).
func (server *Server) isDiscoveryName(host string) bool {
	dependencies, err := server.readyDependencies()
	if err != nil || dependencies.Discovery == nil {
		return false
	}
	wildcard := dependencies.Discovery.Status().CertificateName()
	return wildcard != "" && strings.HasSuffix(host, strings.TrimPrefix(wildcard, "*"))
}
