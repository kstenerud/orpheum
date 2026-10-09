// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net"
	"net/netip"
	"strconv"

	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/discoveryproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// apiMajor is the API's major version, the one in api.Prefix, which the discovery passes on so it
// serves a client that speaks it.
const apiMajor = 1

// newDiscovery is the core's side of its discovery (D273): its identity key, made on first start
// beside secret.key, and the service that enrols with a discovery when an admin asks and answers the
// WebRTC offers it passes on through answerer.
func newDiscovery(
	config Config, opened openedServices, answerer discovery.Answerer, logger *slog.Logger,
) (*discovery.Service, error) {
	key, err := identity.LoadOrMake(opened.directories, rand.Reader)
	if err != nil {
		return nil, err
	}
	return discovery.New(discovery.Parts{
		Files: opened.directories, Key: key, HTTP: opened.jobServices.outboundClient, Clock: clock.System{},
		Logger: logger, Version: thisBuild().Version, APIMajor: apiMajor,
		Candidates: func() []discoveryproto.Candidate { return interfaceCandidates(reachablePort(config)) },
		Answerer:   answerer,
	})
}

// openReachedFromOutside opens what lets the server be reached from outside: the certificate of
// its public name, and its discovery, which passes the WebRTC offers it is sent on to answerer.
func openReachedFromOutside(
	ctx context.Context, config Config, opened openedServices, outside wiring, answerer discovery.Answerer,
	logger *slog.Logger,
) (openedServices, error) {
	opened, err := openCertificates(ctx, config, opened, outside, logger)
	if err != nil {
		return opened, err
	}
	opened.jobServices.discovery, err = newDiscovery(config, opened, answerer, logger)
	return opened, err
}

// reachablePort is the port a client reaches the core on: HTTPS's when the core serves it, the
// plain listener's otherwise; zero when neither names one.
func reachablePort(config Config) int {
	listen := config.Server.Listen
	if config.HTTPS.isOn() {
		listen = config.HTTPS.Listen
	}
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return 0
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return number
}

// interfaceCandidates are the machine's own addresses at port: each global unicast address, a private
// one marked as reachable only at home (discoveryproto.ScopeLAN). A core in a container sees the
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
		if !found.IsGlobalUnicast() {
			continue
		}
		candidate := discoveryproto.Candidate{
			Address: found.String(), Port: port, Source: discoveryproto.SourceInterface,
		}
		if found.IsPrivate() {
			candidate.Scope = discoveryproto.ScopeLAN
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}
