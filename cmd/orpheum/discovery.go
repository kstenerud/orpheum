// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/core/portmap"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/together"
	"github.com/kstenerud/orpheum/internal/core/turn"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// apiMajor is the API's major version, the one in api.Prefix, which the discovery passes on so it
// serves a client that speaks it.
const apiMajor = 1

// newDiscovery is the core's side of its discovery (D273): its identity key, made on first start
// beside secret.key, and the service that enrols with a discovery when an admin asks and answers the
// WebRTC offers it passes on through answerer.
func newDiscovery(
	config Config, opened openedServices, answerer discovery.Answerer, onClaimed func(context.Context),
	mapped *portmap.Service, logger *slog.Logger,
) (*discovery.Service, error) {
	key, err := identity.LoadOrMake(opened.directories, rand.Reader)
	if err != nil {
		return nil, err
	}
	parts := discoveryParts(config, opened, answerer, onClaimed, mapped, logger)
	parts.Key = key
	return discovery.New(parts)
}

// discoveryParts is what the core's side of its discovery is made of, but for its identity key:
// what it tells its discovery in each heartbeat, and what it answers the offers passed on to it with.
func discoveryParts(
	config Config, opened openedServices, answerer discovery.Answerer, onClaimed func(context.Context),
	mapped *portmap.Service, logger *slog.Logger,
) discovery.Parts {
	return discovery.Parts{
		Files: opened.directories, Settings: opened.settings, HTTP: opened.outboundClient,
		Clock:  clock.System{},
		Logger: logger, Version: thisBuild().Version, APIMajor: apiMajor, Suggested: discovery.Suggested(),
		Candidates: heartbeatCandidates(config, mapped.Candidates),
		Answerer:   answerer, OnClaimed: onClaimed,
		Random: rand.Reader,
		Links:  linksOf(opened.auth, opened.together), OfferKey: offerKeyOf(opened.auth, opened.together),
		HandOffOfferKeys: opened.auth.HandOffOfferKeys,
		// A person's realm key is made for the realm the discovery finds the core by, and checks the
		// realm sign-ins it passes on (D297 §3).
		RealmSignIn: opened.auth.TakeRealmSignIn, OnRealm: opened.auth.SetRealm,
		TURN: relayForFound(newRelayIssuer(config, opened, logger), logger), ProbeNonce: opened.reachability.ProbeNonce,
		HasCertificate:          opened.certificates.hasDiscoveryCertificate,
		ConfiguredHouseNetworks: config.Discovery.houseNetworks(),
	}
}

// heartbeatCandidates is what the core may be reached at, read afresh for each heartbeat: the machine's own
// addresses, the router's mappings, and the addresses the config names (the protocol's §2.4).
func heartbeatCandidates(config Config, mapped func() []discoveryproto.Candidate) func() []discoveryproto.Candidate {
	return func() []discoveryproto.Candidate {
		var candidates []discoveryproto.Candidate
		if config.Discovery.SendInterfaceAddresses {
			candidates = interfaceCandidates(httpsPort(config))
		}
		return append(append(candidates, mapped()...), config.Discovery.configuredCandidates()...)
	}
}

// relayForFound is the core's own relay as its discovery hands it to the devices it finds the core
// for, with credentials fresh for each heartbeat (the protocol's §2.4); none for a core that relays
// nothing.
func relayForFound(issuer *turn.Issuer, logger *slog.Logger) func(ctx context.Context) []discoveryproto.ICEServer {
	return func(ctx context.Context) []discoveryproto.ICEServer {
		servers, err := issuer.ICEServersForFound(ctx, clock.System{}.Now())
		if err != nil {
			logger.WarnContext(ctx, "the relay's credentials were not made for the discovery", "err", err)
			return nil
		}
		found := make([]discoveryproto.ICEServer, 0, len(servers))
		for _, server := range servers {
			found = append(found, discoveryproto.ICEServer{
				URLs: server.URLs, Username: server.Credentials.Username, Credential: server.Credentials.Password,
			})
		}
		return found
	}
}

// linksOf is what a discovery finds the core by, as accounts and Watch Together keep it: every live
// link's locate hash and when it ends, and every live session's and membership device's locator's hash
// (discovery-core.md K7).
func linksOf(
	accounts *auth.Service, sessions *together.Hub,
) func(context.Context) ([]discovery.Link, []string, error) {
	return func(ctx context.Context) ([]discovery.Link, []string, error) {
		live, err := accounts.LiveLinks(ctx)
		if err != nil {
			return nil, nil, err
		}
		joining := sessions.LiveLinks()
		links := make([]discovery.Link, 0, len(live)+len(joining))
		for _, one := range live {
			links = append(links, discovery.Link{LocateHash: one.LocateHash, ExpiresAt: one.ExpiresAt})
		}
		for _, one := range joining {
			links = append(links, discovery.Link{LocateHash: one.LocateHash, ExpiresAt: one.ExpiresAt})
		}
		locators, err := accounts.LiveLocators(ctx)
		return links, append(locators, sessions.LiveLocators()...), err
	}
}

// offerKeyOf is what an offer under hash is proven under: a session's or a link's, as accounts keeps
// them, else a Watch Together membership device's or unused link's.
func offerKeyOf(accounts *auth.Service, sessions *together.Hub) func(context.Context, string) ([]byte, error) {
	return func(ctx context.Context, hash string) ([]byte, error) {
		key, err := accounts.OfferKey(ctx, hash)
		if !errors.Is(err, auth.ErrNoOfferKey) {
			return key, err
		}
		if key, isFound := sessions.OfferKey(hash); isFound {
			return key, nil
		}
		return nil, err
	}
}

// openReachedFromOutside opens what lets the server be reached from outside: its discovery, which
// passes the WebRTC offers it is sent on to answerer, and the certificates of its public name and
// of the names its discovery gives it, asking at once for each it lacks.
func openReachedFromOutside(
	ctx context.Context, config Config, opened openedServices, outside wiring, rtc *rtcapi.Server,
	logger *slog.Logger,
) (openedServices, error) {
	opened, err := openCertificates(config, opened, outside, logger)
	if err != nil {
		return opened, err
	}
	served := opened.certificates
	var discovered *discovery.Service
	onClaimed := func(ctx context.Context) {
		if useErr := served.useDiscoveryName(discovered.Status().CertificateName()); useErr != nil {
			logger.ErrorContext(ctx, "the certificate of the discovery's names could not be read", "err", useErr)
		}
		if _, queueErr := served.QueueRenewal(ctx); queueErr != nil {
			logger.ErrorContext(ctx, "the certificate of the discovery's names could not be asked for", "err", queueErr)
		}
	}
	opened.reachability = newReachability(config, opened)
	mapped, err := newPortMapping(config, opened, func() discovery.Status { return discovered.Status() }, logger)
	if err != nil {
		return opened, err
	}
	discovered, err = newDiscovery(config, opened, rtc, onClaimed, mapped, logger)
	if err != nil {
		return opened, err
	}
	opened.auth.OnLinksChanged(discovered.LinksChanged)
	opened.together.OnLinksChanged(discovered.LinksChanged)
	// A code the core did not issue is claimed from the paid service, which a TV that knows no core
	// asked (D293 §2).
	opened.auth.ClaimDeviceCodesFrom(discovered)
	opened.auth.OnDeviceActivated(logActivation(logger))
	rtc.UseICEServers(func() []webrtc.ICEServer {
		return iceServersFor(config.WebRTC.STUNServers, discovered.Status().ICEServers)
	})
	opened.discovery, opened.portMapping = discovered, mapped
	served.discoveryParts.DNS = discovered
	if useErr := served.useDiscoveryName(discovered.Status().CertificateName()); useErr != nil {
		return opened, useErr
	}
	_, err = served.QueueRenewal(ctx)
	return opened, err
}

// iceServersFor are the servers a WebRTC connection gathers through: the core's own STUN servers, and
// the TURN servers its discovery names with their credentials. The discovery's STUN servers are left
// out: an IPv6 address a STUN server finds names the house's network, from which an admin
// administers, so a discovery naming its own would make its network home (the Plex-chain review's
// L3). A relay's candidates never count as home (rtcapi's isOwnCandidate).
func iceServersFor(ownSTUN []string, named []discoveryproto.ICEServer) []webrtc.ICEServer {
	var servers []webrtc.ICEServer
	if len(ownSTUN) > 0 {
		servers = append(servers, webrtc.ICEServer{URLs: slices.Clone(ownSTUN)})
	}
	for _, server := range named {
		turnURLs := slices.DeleteFunc(slices.Clone(server.URLs), func(url string) bool {
			return strings.HasPrefix(url, "stun:") || strings.HasPrefix(url, "stuns:")
		})
		if len(turnURLs) > 0 {
			servers = append(servers, webrtc.ICEServer{
				URLs: turnURLs, Username: server.Username, Credential: server.Credential,
			})
		}
	}
	return servers
}

// newPortMapping keeps the core's HTTPS port open at the home router while its discovery publishes
// it and should_map_ports is on (D273 §7).
func newPortMapping(
	config Config, opened openedServices, standing func() discovery.Status, logger *slog.Logger,
) (*portmap.Service, error) {
	return portmap.New(portMappingParts(config, opened, standing, logger))
}

// portMappingParts are what the port mapping is made of: the server's settings among them, which say
// how long it asks for a mapping and how often again.
func portMappingParts(
	config Config, opened openedServices, standing func() discovery.Status, logger *slog.Logger,
) portmap.Parts {
	return portmap.Parts{
		Settings: opened.settings, HTTP: opened.outboundClient, Clock: clock.System{}, Logger: logger,
		Port: httpsPort(config),
		IsWanted: func(ctx context.Context) bool {
			if !standing().IsPublished() {
				return false
			}
			isOn, err := opened.settings.Bool(ctx, settings.ShouldMapPorts)
			if err != nil {
				logger.WarnContext(ctx, "port mapping: could not read should_map_ports; not mapping", "err", err)
			}
			return isOn
		},
		Gateway: defaultGateway, IPv6Addresses: portmap.GlobalIPv6Addresses,
		GatewayPort: portmap.GatewayPort, SearchAddress: portmap.SSDPAddress,
	}
}

// routesFile is Linux's IPv4 routing table.
const routesFile = "/proc/net/route"

// defaultGateway is the router of the machine's default IPv4 route, which port mapping asks.
func defaultGateway() (netip.Addr, error) {
	table, err := os.ReadFile(routesFile)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("read the routing table: %w", err)
	}
	return portmap.GatewayOf(string(table))
}

// httpsPort is the port a client reaches the core on through its discovery, whose names are served
// over HTTPS: https.listen's; zero when it names none.
func httpsPort(config Config) int {
	_, port, err := net.SplitHostPort(config.HTTPS.Listen)
	if err != nil {
		return 0
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return number
}

// interfaceCandidates are the machine's own addresses at port: each public one (discoveryproto.IsPublic),
// and each private one marked as reachable only at home (discoveryproto.ScopeLAN). A core in a container sees the
// container's (discovery-core.md, Found while planning).
func interfaceCandidates(port int) []discoveryproto.Candidate {
	if port == 0 {
		return nil
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	return candidatesOf(addresses, port)
}

// candidatesOf is interfaceCandidates' choice among addresses.
func candidatesOf(addresses []net.Addr, port int) []discoveryproto.Candidate {
	var candidates []discoveryproto.Candidate
	for _, address := range addresses {
		prefix, err := netip.ParsePrefix(address.String())
		if err != nil {
			continue
		}
		found := prefix.Addr().Unmap()
		candidate := discoveryproto.Candidate{
			Address: found.String(), Port: port, Source: discoveryproto.SourceInterface,
		}
		switch {
		case discoveryproto.IsPublic(found):
		case found.IsPrivate():
			candidate.Scope = discoveryproto.ScopeLAN
		default:
			// Loopback, link-local, carrier NAT, DS-Lite's own address: nobody reaches the core there.
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

// logActivation logs each device a code activates, who entered the code and for whom, so a device
// someone talked a person into activating is on the record (the device code audit's M4).
func logActivation(logger *slog.Logger) func(context.Context, auth.Activation) {
	return func(ctx context.Context, activated auth.Activation) {
		logger.InfoContext(ctx, "a device was activated by a code",
			"byUser", activated.ByUserID, "forUser", activated.ForUserID, "device", activated.DeviceName,
			"platform", activated.Platform, "throughService", activated.IsThroughService)
	}
}
