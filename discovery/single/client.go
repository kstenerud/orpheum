// SPDX-License-Identifier: AGPL-3.0-or-later

package single

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
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
		if request.Method == http.MethodPost && strings.HasPrefix(request.URL.Path, discoveryproto.PathCores) &&
			strings.HasSuffix(request.URL.Path, "/offer") {
			discovery.offers(writer, request)
			return
		}
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
	_, isHeld := discovery.heldHash(asked)
	return isHeld
}

// heldHash is the hash the request found the core by, which an offer is passed on with: a registered
// locator's, or a live bootstrap token's link's.
func (discovery *Discovery) heldHash(asked discoveryproto.PathsRequest) (string, bool) {
	now := discovery.parts.Now()
	discovery.mutex.Lock()
	defer discovery.mutex.Unlock()
	if asked.Locator != "" && discovery.locators[link.LocatorHash(asked.Locator)] {
		return link.LocatorHash(asked.Locator), true
	}
	held, isHeld := discovery.bootstraps[asked.BootstrapToken]
	if asked.BootstrapToken == "" || !isHeld || !now.Before(held.expires) {
		return "", false
	}
	return held.locateHash, true
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

// openOffersMost is how many offers one locator may have waiting on the core at once (S5).
const openOffersMost = 8

// offerWait is how long an offer waits for the core's answer.
const offerWait = 15 * time.Second

// offers passes a device's offers to the core, each with the hash the device found it by and its own
// proof, at most MaxOffersAtOnce at a time and openOffersMost open for one hash, and answers the core's
// answers in the offers' order (the protocol's §2.5).
func (discovery *Discovery) offers(writer http.ResponseWriter, request *http.Request) {
	var asked discoveryproto.OfferRequest
	if !read(writer, request, &asked) {
		return
	}
	hash, isHeld := discovery.heldHash(asked.PathsRequest)
	record, err := discovery.parts.Store.Read()
	if err != nil {
		discovery.fail(writer, request, err)
		return
	}
	if !isHeld || record.Core == nil || request.URL.Path != discoveryproto.OfferPath(record.Core.ID) {
		refuse(writer, http.StatusNotFound, discoveryproto.RefusalNotFound, "no server is found by this")
		return
	}
	if len(asked.Offers) == 0 || len(asked.Offers) > discoveryproto.MaxOffersAtOnce {
		http.Error(writer, fmt.Sprintf("send 1 to %d offers", discoveryproto.MaxOffersAtOnce), http.StatusBadRequest)
		return
	}
	if !discovery.openOffers(hash, len(asked.Offers)) {
		http.Error(writer, "too many offers open for this", http.StatusTooManyRequests)
		return
	}
	defer discovery.closeOffers(hash, len(asked.Offers))
	ctx, cancel := context.WithTimeout(request.Context(), offerWait)
	defer cancel()
	answers := make([]discoveryproto.OfferAnswer, len(asked.Offers))
	var waiting sync.WaitGroup
	for index, offer := range asked.Offers {
		waiting.Go(func() {
			answered, offerErr := discovery.Offer(ctx, offer, hash)
			answers[index] = discoveryproto.OfferAnswer{
				OfferID: answered.OfferID, SDP: answered.SDP, Signature: answered.Signature,
			}
			if offerErr != nil {
				answers[index].Refusal = offerErr.Error()
			}
		})
	}
	waiting.Wait()
	answer(writer, discoveryproto.OfferResponse{Answers: answers})
}

// openOffers counts count offers open for hash, unless that passes openOffersMost.
func (discovery *Discovery) openOffers(hash string, count int) bool {
	discovery.mutex.Lock()
	defer discovery.mutex.Unlock()
	if discovery.openByHash[hash]+count > openOffersMost {
		return false
	}
	discovery.openByHash[hash] += count
	return true
}

// closeOffers counts count of hash's offers closed.
func (discovery *Discovery) closeOffers(hash string, count int) {
	discovery.mutex.Lock()
	defer discovery.mutex.Unlock()
	discovery.openByHash[hash] -= count
	if discovery.openByHash[hash] <= 0 {
		delete(discovery.openByHash, hash)
	}
}
