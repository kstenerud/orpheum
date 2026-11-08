// SPDX-License-Identifier: AGPL-3.0-or-later

package outbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"time"
)

// ipv4Delay is how long an IPv6 connection is given before IPv4 is tried beside it: Go's own
// fallback delay, Happy Eyeballs' (RFC 8305) 250 ms rounded as Go does.
const ipv4Delay = 300 * time.Millisecond

// lookupFunc and dialFunc are the resolver's and the dialer's, which a test replaces.
type (
	lookupFunc func(ctx context.Context, network, host string) ([]netip.Addr, error)
	dialFunc   func(ctx context.Context, network, address string) (net.Conn, error)
)

// ipv6First dials a host's IPv6 addresses before its IPv4 ones, IPv4 starting ipv4Delay after IPv6
// if no connection is made by then, and the first made wins. Go orders a host's addresses by RFC
// 6724, which puts IPv4 first from a machine whose only IPv6 address is a private one, as a core in
// Docker's IPv6 network is: its requests reached the paid service over the house's shared DS-Lite
// IPv4, never as the house's own IPv6 address, which the service judges a device at home by
// (the production NAS, 2026-10-20). Docker translates the private address to the house's on the way out.
//
//go:noinline // one closure, whose code TestNewClient_DialsThroughIPv6First recognises
func ipv6First(lookup lookupFunc, dial dialFunc) dialFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("dial %s: %w", address, err)
		}
		if _, err = netip.ParseAddr(host); err == nil {
			return dial(ctx, network, address)
		}
		found, err := lookup(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("look up %s: %w", host, err)
		}
		ipv6 := slices.DeleteFunc(slices.Clone(found), func(each netip.Addr) bool { return each.Unmap().Is4() })
		ipv4 := slices.DeleteFunc(slices.Clone(found), func(each netip.Addr) bool { return !each.Unmap().Is4() })
		return race(ctx, network, port, ipv6, ipv4, dial)
	}
}

// dialled is one family's attempt: its connection, or why there is none.
type dialled struct {
	connection net.Conn
	err        error
}

// race dials first's addresses at once and second's after ipv4Delay, or at once when first has none
// or fails, and returns the first connection made, closing any made after it.
func race(ctx context.Context, network, port string, first, second []netip.Addr, dial dialFunc) (net.Conn, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan dialled, 2)
	attempt := func(addresses []netip.Addr) {
		results <- inTurn(ctx, network, port, addresses, dial)
	}
	go attempt(first)
	head := time.NewTimer(ipv4Delay)
	defer head.Stop()
	isSecondStarted := len(first) == 0
	if isSecondStarted {
		go attempt(second)
	}
	var errs []error
	for pending := 2; pending > 0; {
		select {
		case <-head.C:
			if !isSecondStarted {
				isSecondStarted = true
				go attempt(second)
			}
		case result := <-results:
			pending--
			if result.err == nil {
				go closeLater(results, pending)
				return result.connection, nil
			}
			errs = append(errs, result.err)
			if !isSecondStarted {
				isSecondStarted = true
				go attempt(second)
			}
		}
	}
	return nil, errors.Join(errs...)
}

// inTurn dials addresses one after another, and is the first connection made.
func inTurn(ctx context.Context, network, port string, addresses []netip.Addr, dial dialFunc) dialled {
	if len(addresses) == 0 {
		return dialled{err: errors.New("no address of this family")}
	}
	var errs []error
	for _, address := range addresses {
		connection, err := dial(ctx, network, net.JoinHostPort(address.Unmap().String(), port))
		if err == nil {
			return dialled{connection: connection}
		}
		errs = append(errs, err)
	}
	return dialled{err: errors.Join(errs...)}
}

// closeLater closes the connections still to come on results, pending of them, made after the race
// was won.
func closeLater(results <-chan dialled, pending int) {
	for range pending {
		if late := <-results; late.connection != nil {
			_ = late.connection.Close() //nolint:errcheck // a connection nobody will use
		}
	}
}
