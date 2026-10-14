// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/link"
)

// bootstrapLasts is how long a bootstrap token works, never renewed: long enough to sign in or
// register over a path it found (the protocol's §3.1; X10) [I: "a few minutes"].
const bootstrapLasts = 5 * time.Minute

// locatable is a link a client may locate the core by: when it ends, and, for a link with a number of
// uses, how many it has left.
type locatable struct {
	expires   time.Time
	isLimited bool
	usesLeft  int
}

// bootstrap is a bootstrap token's link, by its locate hash, and when the token ends.
type bootstrap struct {
	locateHash string
	expires    time.Time
}

// ClientHandler answers the client API, at paths relative to <base>/client/v1: a link's hash, a
// locator or a bootstrap token finds the core's paths (the protocol's §3, §4). The core's own calls
// are Handler's.
func (discovery *Discovery) ClientHandler() http.Handler {
	return http.HandlerFunc(discovery.serveClient)
}

func (discovery *Discovery) serveClient(writer http.ResponseWriter, request *http.Request) {
	version := request.Header.Get(discoveryproto.ProtocolHeader)
	if !discoveryproto.Serves([]int{discoveryproto.ProtocolMajor}, version) {
		refuse(writer, http.StatusBadRequest, discoveryproto.RefusalVersion,
			discoveryproto.ProtocolHeader+" "+version+": this discovery serves 1.x")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, bodyMost)
	switch request.Method + " " + request.URL.Path {
	case http.MethodPost + " " + discoveryproto.PathLocate:
		discovery.locate(writer, request)
	case http.MethodPost + " " + discoveryproto.PathPaths:
		discovery.paths(writer, request)
	default:
		http.NotFound(writer, request)
	}
}

// locate finds the core by a link's hash, spending one of its uses when it has a number of them, and
// answers its paths and a bootstrap token.
func (discovery *Discovery) locate(writer http.ResponseWriter, request *http.Request) {
	var asked discoveryproto.LocateRequest
	if !read(writer, request, &asked) {
		return
	}
	now := discovery.parts.Now()
	token, err := discovery.secret()
	if err != nil {
		discovery.fail(writer, request, err)
		return
	}
	discovery.mutex.Lock()
	held, isHeld := discovery.locateHashes[asked.Hash]
	isLive := isHeld && now.Before(held.expires) && (!held.isLimited || held.usesLeft > 0)
	if isLive {
		if held.isLimited {
			held.usesLeft--
			discovery.locateHashes[asked.Hash] = held
		}
		discovery.pruneBootstraps(now)
		discovery.bootstraps[token] = bootstrap{locateHash: asked.Hash, expires: now.Add(bootstrapLasts)}
	}
	discovery.mutex.Unlock()
	if !isLive {
		refuse(writer, http.StatusNotFound, discoveryproto.RefusalNotFound, "no server is found by this link")
		return
	}
	paths, ok := discovery.pathsFor(writer, request)
	if !ok {
		return
	}
	answer(writer, discoveryproto.LocateResponse{PathsResponse: paths, BootstrapToken: token})
}

// paths answers the core's paths to a device holding one of its locators, or a live bootstrap token.
func (discovery *Discovery) paths(writer http.ResponseWriter, request *http.Request) {
	var asked discoveryproto.PathsRequest
	if !read(writer, request, &asked) {
		return
	}
	if !discovery.holds(asked) {
		refuse(writer, http.StatusNotFound, discoveryproto.RefusalNotFound, "no server is found by this")
		return
	}
	paths, ok := discovery.pathsFor(writer, request)
	if !ok {
		return
	}
	answer(writer, paths)
}

// holds is whether the request names the core: a locator it registered, or a bootstrap token not yet
// ended.
func (discovery *Discovery) holds(asked discoveryproto.PathsRequest) bool {
	now := discovery.parts.Now()
	discovery.mutex.Lock()
	defer discovery.mutex.Unlock()
	if asked.Locator != "" && discovery.locators[link.LocatorHash(asked.Locator)] {
		return true
	}
	held, isHeld := discovery.bootstraps[asked.BootstrapToken]
	return asked.BootstrapToken != "" && isHeld && now.Before(held.expires)
}

// pruneBootstraps forgets the bootstrap tokens that have ended. The caller holds the mutex.
func (discovery *Discovery) pruneBootstraps(now time.Time) {
	for token, held := range discovery.bootstraps {
		if !now.Before(held.expires) {
			delete(discovery.bootstraps, token)
		}
	}
}

// pathsFor is the claimed core's paths, as a client at request's source is told them; false, after
// answering, when there is no claimed core.
func (discovery *Discovery) pathsFor(
	writer http.ResponseWriter, request *http.Request,
) (discoveryproto.PathsResponse, bool) {
	record, err := discovery.parts.Store.Read()
	if err != nil {
		discovery.fail(writer, request, err)
		return discoveryproto.PathsResponse{}, false
	}
	held := record.Core
	if held == nil || held.State != discoveryproto.StateClaimed {
		refuse(writer, http.StatusNotFound, discoveryproto.RefusalNotFound, "no server is found by this")
		return discoveryproto.PathsResponse{}, false
	}
	paths := discoveryproto.PathsResponse{
		CoreID: held.ID, State: discoveryproto.CoreOnline, ClientPrefix: prefixOf(request.RemoteAddr),
		// WebRTC alone until single publishes direct names (W4b.4; D274 §5).
		Paths: []discoveryproto.Path{
			{Kind: discoveryproto.PathKindWebRTC, Offer: discoveryproto.OfferPath(held.ID)},
		},
		ICEServers: append([]discoveryproto.ICEServer{}, discovery.parts.ICEServers...),
	}
	if held.Addresses != nil {
		paths.APIMajor = held.Addresses.APIMajor
		paths.ICEServers = append(paths.ICEServers, held.Addresses.TURN...)
	}
	if isOnline, since := discovery.signal.standing(); !isOnline {
		paths.State = discoveryproto.CoreOffline
		if !since.IsZero() {
			paths.OfflineSince = since.UnixMilli()
		}
	}
	return paths, true
}

// prefixOf is a client's network as the discovery sees it, the key a client remembers what worked
// under (D293 §8): an IPv6 address's /64, an IPv4 address's /32 [I: the lengths].
func prefixOf(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		host = remoteAddress
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return ""
	}
	address = address.Unmap()
	bits := 32
	if address.Is6() {
		bits = 64
	}
	prefix, err := address.Prefix(bits)
	if err != nil {
		return ""
	}
	return prefix.String()
}
