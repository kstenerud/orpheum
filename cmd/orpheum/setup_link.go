// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"net"
	"net/netip"
	"net/url"
	"strconv"

	"github.com/kstenerud/orpheum/internal/core/home"
)

// setupLinkRoute is the page the first admin is made on: the web app's root, which is the sign-in
// that opens on setting up while no admin exists (web/apps/web/src/sign-in.tsx). There is no route of
// its own.
const setupLinkRoute = "/"

// setupLinkCodeKey is the fragment's key the web app reads the setup code from
// (web/apps/web/src/set-up-link.ts): a fragment, so no server log or proxy ever sees the code (D527 §1.2).
const setupLinkCodeKey = "code"

// setupReach is how a browser at home reaches the core: the HTTP port it takes plain connections on
// and the machine's interfaces, read when asked, which a test replaces.
type setupReach struct {
	port       int
	interfaces func() []home.Interface
}

// links are the magic links (D527 §1.2) that make the first admin with code, one for each private
// address of this machine; none when the machine has no such address or the port is not known.
func (reach setupReach) links(code string) []string {
	if reach.port <= 0 || reach.interfaces == nil {
		return nil
	}
	var links []string
	for _, address := range home.AddressesAtHome(reach.interfaces(), nil) {
		links = append(links, setupLink(address, reach.port, code))
	}
	return links
}

// setupLink is the address that opens the web app on setting up with code already filled in:
// http://address:port/#code=code, the port left out when it is the scheme's own.
func setupLink(address netip.Addr, port int, code string) string {
	host := address.String()
	if port != plainHTTPDefaultPort {
		host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	link := url.URL{Scheme: "http", Host: host, Path: setupLinkRoute}
	return link.String() + "#" + setupLinkCodeKey + "=" + url.QueryEscape(code)
}
