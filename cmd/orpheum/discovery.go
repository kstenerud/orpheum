// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"

	"github.com/pion/webrtc/v4"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/core/portmap"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/core/settings"
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
	retryFirst, retryMost := config.Discovery.retries()
	return discovery.New(discovery.Parts{
		Files: opened.directories, Key: key, HTTP: opened.jobServices.outboundClient, Clock: clock.System{},
		Logger: logger, Version: thisBuild().Version, APIMajor: apiMajor,
		Candidates: func() []discoveryproto.Candidate {
			return append(interfaceCandidates(httpsPort(config)), mapped.Candidates()...)
		},
		Answerer: answerer, OnClaimed: onClaimed,
		RetryFirst: retryFirst, RetryMost: retryMost, Random: rand.Reader,
		Links: linksOf(opened.auth), OfferKey: opened.auth.OfferKey,
	})
}

// linksOf is what a discovery finds the core by, as accounts keeps it: every live link's locate hash
// and when it ends, and every live session's device locator's hash (discovery-core.md K7).
func linksOf(accounts *auth.Service) func(context.Context) ([]discovery.Link, []string, error) {
	return func(ctx context.Context) ([]discovery.Link, []string, error) {
		live, err := accounts.LiveLinks(ctx)
		if err != nil {
			return nil, nil, err
		}
		links := make([]discovery.Link, 0, len(live))
		for _, one := range live {
			links = append(links, discovery.Link{LocateHash: one.LocateHash, ExpiresAt: one.ExpiresAt})
		}
		locators, err := accounts.LiveLocators(ctx)
		return links, locators, err
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
	served := opened.jobServices.certificates
	var discovered *discovery.Service
	onClaimed := func(ctx context.Context) {
		if useErr := served.useDiscoveryName(discovered.Status().CertificateName()); useErr != nil {
			logger.ErrorContext(ctx, "the certificate of the discovery's names could not be read", "err", useErr)
		}
		if _, queueErr := served.QueueRenewal(ctx); queueErr != nil {
			logger.ErrorContext(ctx, "the certificate of the discovery's names could not be asked for", "err", queueErr)
		}
	}
	mapped, err := newPortMapping(config, opened, func() discovery.Status { return discovered.Status() }, logger)
	if err != nil {
		return opened, err
	}
	discovered, err = newDiscovery(config, opened, rtc, onClaimed, mapped, logger)
	if err != nil {
		return opened, err
	}
	opened.auth.OnLinksChanged(discovered.LinksChanged)
	rtc.UseICEServers(func() []webrtc.ICEServer { return iceServersOf(discovered.Status().ICEServers) })
	opened.jobServices.discovery, opened.jobServices.portMapping = discovered, mapped
	served.discoveryParts.DNS = discovered
	if useErr := served.useDiscoveryName(discovered.Status().CertificateName()); useErr != nil {
		return opened, useErr
	}
	_, err = served.QueueRenewal(ctx)
	return opened, err
}

// iceServersOf are the discovery's STUN servers as a WebRTC connection takes them.
func iceServersOf(servers []discoveryproto.ICEServer) []webrtc.ICEServer {
	converted := make([]webrtc.ICEServer, 0, len(servers))
	for _, server := range servers {
		converted = append(converted, webrtc.ICEServer{
			URLs: server.URLs, Username: server.Username, Credential: server.Credential,
		})
	}
	return converted
}

// newPortMapping keeps the core's HTTPS port open at the home router while its discovery publishes
// it and should_map_ports is on (D273 §7).
func newPortMapping(
	config Config, opened openedServices, standing func() discovery.Status, logger *slog.Logger,
) (*portmap.Service, error) {
	return portmap.New(portmap.Parts{
		HTTP: opened.jobServices.outboundClient, Clock: clock.System{}, Logger: logger, Port: httpsPort(config),
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
	})
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
