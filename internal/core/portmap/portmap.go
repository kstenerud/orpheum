// SPDX-License-Identifier: AGPL-3.0-or-later

package portmap

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// The ways a router is asked, in the order they are tried for IPv4, and the one for IPv6.
const (
	MethodPCP    = "pcp"
	MethodNATPMP = "natpmp"
	MethodUPnP   = "upnp"
)

// lifetime is what each mapping is asked for; it is renewed at half of what the router grants.
const lifetime = time.Hour

// checkEvery is how often the loop looks whether the mappings are still wanted and whether the
// machine's addresses changed.
const checkEvery = time.Minute

// retryAfter is how long the loop waits to ask again when no router took a mapping.
const retryAfter = 5 * time.Minute

// permanentRenewal is how often a mapping the router made for good is asked for again, in case
// the router restarted and forgot it.
const permanentRenewal = 30 * time.Minute

// releaseWait bounds the removal of the mappings when the core stops.
const releaseWait = 3 * time.Second

// Parts are what the service is built from.
type Parts struct {
	// HTTP is the client UPnP's requests to the router go through: internal/platform/outbound's.
	HTTP   *http.Client
	Clock  clock.Clock
	Logger *slog.Logger
	// Port is the core's HTTPS port, the one opened: internal and, where the router agrees,
	// external.
	Port int
	// IsWanted is whether the mappings should exist now: should_map_ports on, and a discovery
	// publishing the core (D273 §7).
	IsWanted func(ctx context.Context) bool
	// Gateway is the router of the default route: GatewayOf Linux's routing table.
	Gateway func() (netip.Addr, error)
	// IPv6Addresses are the machine's addresses the router's firewall is asked to open: GlobalIPv6Addresses.
	IPv6Addresses func() []netip.Addr
	// GatewayPort and SearchAddress are GatewayPort and SSDPAddress; a test's fake router's otherwise.
	GatewayPort   int
	SearchAddress string
}

// Mapping is one opening a router made for the core: a public IPv4 address and port forwarded to
// the core, or an IPv6 address of the core its firewall lets through.
type Mapping struct {
	Method string
	// Internal is the core's address and port, External where the internet reaches it: the
	// router's public address and the port it granted, or for IPv6 the same as Internal.
	Internal, External netip.AddrPort
	// ExpiresAt is when the router drops it unless renewed; zero for one it made for good.
	ExpiresAt time.Time
}

// Report is what the System page says of port mapping.
type Report struct {
	// IsWanted is whether the core is asking: the setting on and a discovery publishing it.
	IsWanted  bool
	CheckedAt time.Time
	Mappings  []Mapping
	// Problems are why each way that failed failed, in the router's words where it gave any.
	Problems []string
}

// held is a mapping the core keeps, with what renews and removes it.
type held struct {
	Mapping
	renewAt  time.Time
	pinhole  string
	upnp     router
	isIPv6   bool
	external uint16
}

// Service keeps the core's HTTPS port open at the home router while it is wanted (D273 §7): by
// PCP, NAT-PMP or UPnP for IPv4, by UPnP's IPv6 firewall control for each global IPv6 address.
type Service struct {
	parts Parts
	nonce [pcpNonceSize]byte

	mutex sync.Mutex
	// Guarded by mutex:
	held   []held
	report Report
	// shape is the gateway, the internal IPv4 address and the IPv6 addresses the mappings were made
	// for: a change asks again at once.
	shape string
}

// New returns the service.
func New(parts Parts) (*Service, error) {
	service := &Service{parts: parts}
	if _, err := rand.Read(service.nonce[:]); err != nil {
		return nil, fmt.Errorf("make the PCP nonce: %w", err)
	}
	return service, nil
}

// Report is what the service did last.
func (service *Service) Report() Report {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	report := service.report
	report.Mappings = slices.Clone(report.Mappings)
	report.Problems = slices.Clone(report.Problems)
	return report
}

// Candidates are the public addresses the routers mapped to the core, for its discovery: IPv6
// openings add none, since the core's interface addresses already name them.
func (service *Service) Candidates() []discoveryproto.Candidate {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	var candidates []discoveryproto.Candidate
	for _, each := range service.held {
		if !each.isIPv6 {
			candidates = append(candidates, discoveryproto.Candidate{
				Address: each.External.Addr().String(), Port: int(each.External.Port()),
				Source: discoveryproto.SourceMapped, Method: each.Method,
			})
		}
	}
	return candidates
}

// Run keeps the mappings while they are wanted, until ctx ends, and then removes them.
func (service *Service) Run(ctx context.Context) {
	defer func() {
		releasing, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseWait)
		defer cancel()
		service.release(releasing)
	}()
	for {
		service.step(ctx)
		timer := service.parts.Clock.NewTimer(checkEvery)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C():
		}
	}
}

// step removes the mappings when they are no longer wanted, and makes or renews them when they are
// due or the machine's addresses changed.
func (service *Service) step(ctx context.Context) {
	if !service.parts.IsWanted(ctx) {
		service.release(ctx)
		service.mutex.Lock()
		service.report = Report{CheckedAt: service.parts.Clock.Now()}
		service.mutex.Unlock()
		return
	}
	routerAddress, gatewayErr := service.parts.Gateway()
	local, localErr := service.localAddress(ctx, routerAddress, gatewayErr)
	ipv6 := service.parts.IPv6Addresses()
	shape := fmt.Sprint(routerAddress, local, ipv6)
	now := service.parts.Clock.Now()
	service.mutex.Lock()
	isDue := shape != service.shape || !service.report.IsWanted ||
		slices.ContainsFunc(service.held, func(each held) bool { return !now.Before(each.renewAt) }) ||
		(len(service.held) == 0 && now.Sub(service.report.CheckedAt) >= retryAfter)
	isNewShape := shape != service.shape
	service.mutex.Unlock()
	if !isDue {
		return
	}
	if isNewShape {
		service.release(ctx)
	}
	report := Report{IsWanted: true, CheckedAt: now}
	if err := errors.Join(gatewayErr, localErr); err != nil {
		report.Problems = append(report.Problems, "IPv4: "+err.Error())
	}
	kept := service.refresh(ctx, routerAddress, local, ipv6, &report)
	service.mutex.Lock()
	service.held, service.report, service.shape = kept, report, shape
	service.mutex.Unlock()
}

// localAddress is the machine's IPv4 address toward gateway: what the router forwards to.
func (service *Service) localAddress(
	ctx context.Context, routerAddress netip.Addr, gatewayErr error,
) (netip.Addr, error) {
	if gatewayErr != nil {
		return netip.Addr{}, nil
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "udp4", service.gatewayAt(routerAddress).address.String())
	if err != nil {
		return netip.Addr{}, fmt.Errorf("find this machine's address toward the router: %w", err)
	}
	defer func() { _ = connection.Close() }() //nolint:errcheck // nothing was sent
	local, isUDP := connection.LocalAddr().(*net.UDPAddr)
	if !isUDP {
		return netip.Addr{}, errors.New("find this machine's address toward the router: not a UDP address")
	}
	return local.AddrPort().Addr().Unmap(), nil
}

// refresh makes or renews each mapping: IPv4 by the first way the router takes, each IPv6 address
// by UPnP; what failed is said in report.
func (service *Service) refresh(
	ctx context.Context, routerAddress, local netip.Addr, ipv6 []netip.Addr, report *Report,
) []held {
	var kept []held
	var found *router
	findUPnP := func() (router, error) {
		if found != nil {
			return *found, nil
		}
		searched, err := service.findRouter(ctx, routerAddress, service.parts.SearchAddress)
		if err != nil {
			return router{}, err
		}
		found = &searched
		return searched, nil
	}
	if local.IsValid() {
		if mapped, ok := service.mapIPv4(ctx, routerAddress, local, findUPnP, report); ok {
			kept = append(kept, mapped)
		}
	}
	for _, address := range ipv6 {
		if opened, ok := service.mapIPv6(ctx, address, findUPnP, report); ok {
			kept = append(kept, opened)
		}
	}
	for _, each := range kept {
		service.parts.Logger.InfoContext(ctx, "port mapping: the router opened the core's port", "method",
			each.Method, "internal", each.Internal.String(), "external", each.External.String())
	}
	for _, problem := range report.Problems {
		service.parts.Logger.InfoContext(ctx, "port mapping: a way failed", "problem", problem)
	}
	report.Mappings = make([]Mapping, 0, len(kept))
	for _, each := range kept {
		report.Mappings = append(report.Mappings, each.Mapping)
	}
	return kept
}

// mapIPv4 asks the router to forward its public port to local, by PCP, then NAT-PMP, then UPnP.
func (service *Service) mapIPv4(
	ctx context.Context, routerAddress, local netip.Addr, findUPnP func() (router, error), report *Report,
) (held, bool) {
	internal := netip.AddrPortFrom(local, uint16(service.parts.Port)) //nolint:gosec // a port
	asked := service.gatewayAt(routerAddress)
	external, granted, err := asked.mapPCP(ctx, service.nonce, internal, internal.Port(), lifetime)
	method := MethodPCP
	if errors.Is(err, errOldProtocol) {
		method = MethodNATPMP
		external, granted, err = asked.mapNATPMP(ctx, internal, internal.Port(), lifetime)
	}
	if err == nil {
		return service.heldFor(method, internal, external, granted, router{}), true
	}
	report.Problems = append(report.Problems, method+": "+err.Error())
	found, err := findUPnP()
	if err == nil {
		external, granted, err = service.mapUPnP(ctx, found, internal, internal.Port(), lifetime)
	}
	if err != nil {
		report.Problems = append(report.Problems, "upnp: "+err.Error())
		return held{}, false
	}
	return service.heldFor(MethodUPnP, internal, external, granted, found), true
}

// mapIPv6 asks the router's IPv6 firewall, by UPnP, to let the internet reach address's port.
func (service *Service) mapIPv6(
	ctx context.Context, address netip.Addr, findUPnP func() (router, error), report *Report,
) (held, bool) {
	internal := netip.AddrPortFrom(address, uint16(service.parts.Port)) //nolint:gosec // a port
	found, err := findUPnP()
	var id string
	if err == nil {
		id, err = service.openPinhole(ctx, found, internal, lifetime)
	}
	if err != nil {
		report.Problems = append(report.Problems, "upnp, IPv6 "+address.String()+": "+err.Error())
		return held{}, false
	}
	opened := service.heldFor(MethodUPnP, internal, internal, lifetime, found)
	opened.pinhole, opened.isIPv6 = id, true
	return opened, true
}

// heldFor is a mapping the router granted for granted, zero for good, renewed at half of it.
func (service *Service) heldFor(
	method string, internal, external netip.AddrPort, granted time.Duration, upnp router,
) held {
	now := service.parts.Clock.Now()
	mapping := held{
		Mapping:  Mapping{Method: method, Internal: internal, External: external},
		renewAt:  now.Add(permanentRenewal),
		upnp:     upnp,
		external: external.Port(),
	}
	if granted > 0 {
		mapping.ExpiresAt, mapping.renewAt = now.Add(granted), now.Add(granted/2)
	}
	return mapping
}

// release removes every mapping the core holds; one the router will not remove lapses by itself.
func (service *Service) release(ctx context.Context) {
	service.mutex.Lock()
	releasing := service.held
	service.held, service.shape = nil, ""
	service.mutex.Unlock()
	for _, each := range releasing {
		if err := service.remove(ctx, each); err != nil {
			service.parts.Logger.WarnContext(ctx, "port mapping: could not remove a mapping; it lapses by itself",
				"method", each.Method, "internal", each.Internal.String(), "err", err)
		}
	}
}

// remove removes one mapping the way it was made.
func (service *Service) remove(ctx context.Context, mapping held) error {
	routerAddress, err := service.parts.Gateway()
	if err != nil && !mapping.isIPv6 && mapping.Method != MethodUPnP {
		return err
	}
	asked := service.gatewayAt(routerAddress)
	switch {
	case mapping.isIPv6:
		err = service.closePinhole(ctx, mapping.upnp, mapping.pinhole)
	case mapping.Method == MethodUPnP:
		err = service.unmapUPnP(ctx, mapping.upnp, mapping.external)
	case mapping.Method == MethodPCP:
		_, _, err = asked.mapPCP(ctx, service.nonce, mapping.Internal, 0, 0)
	default:
		_, _, err = asked.mapNATPMP(ctx, mapping.Internal, 0, 0)
	}
	return err
}

// gatewayAt is the router at routerAddress, asked on its PCP and NAT-PMP port.
func (service *Service) gatewayAt(routerAddress netip.Addr) gateway {
	return gateway{
		address: netip.AddrPortFrom(routerAddress, uint16(service.parts.GatewayPort)), //nolint:gosec // a port
		dialer:  &net.Dialer{},
	}
}
