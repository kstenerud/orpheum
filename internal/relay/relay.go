// SPDX-License-Identifier: AGPL-3.0-or-later

package relay

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	pionturn "github.com/pion/turn/v4"

	"github.com/kstenerud/orpheum/discovery/relaycred"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/platform/turnserver"
)

// Config is where the relay listens, what it tells its clients and whose credentials it admits.
type Config struct {
	// Name is the relay's DNS name, which the discovery names it by and its certificate is for.
	Name string
	// PublicAddress is the IPv4 address the relay's clients reach it at, and the one address a
	// relayed datagram may go to or come from (D558 §4, D560).
	PublicAddress net.IP
	// Listen is where TURN is served over UDP and TCP on one port, ":3478".
	Listen string
	// ListenTLS is where TURN over TLS is served, ":443". Its port is not Listen's.
	ListenTLS string
	// RelayPortMin and RelayPortMax are the UDP ports relayed datagrams flow through, both
	// included. They are bound on Listen's host.
	RelayPortMin, RelayPortMax uint16
	// CertificateFile and KeyFile are the TLS certificate chain and its key, in PEM, read again
	// when either changes.
	CertificateFile, KeyFile string
	// Secrets are those a credential may be made under: one or several, each at least 32 bytes.
	Secrets []relaycred.Secret
	// Announce are the discoveries the relay announces itself to each minute an allocation on
	// PublicAddress works (D567 §1). A secret no entry names is announced to no one.
	Announce []Announce
}

// Parts are what the relay needs besides its config.
type Parts struct {
	Clock  clock.Clock
	Logger *slog.Logger
	// HTTP posts the announcements, with a timeout; needed when Config.Announce names any.
	HTTP *http.Client
	// afterDrop, when set, is told of each datagram the relay drops, with why: tests wait on it.
	afterDrop func(reason string)
}

// Relay is the hosted TURN relay (D558): closed to all but a household's own core and clients, at
// the household's rate, recording what each allocation carried and nothing of what it was.
type Relay struct {
	server      *pionturn.Server
	address     net.Addr
	tlsAddress  net.Addr
	stopDays    context.CancelFunc
	daysStopped chan struct{}
	ledger      *ledger
	// announced is closed once the announcer has stopped, or at once when there is none.
	announced chan struct{}
}

// Start serves TURN on config.Listen over UDP and TCP and on config.ListenTLS over TLS, until Close.
func Start(ctx context.Context, config Config, parts Parts) (*Relay, error) {
	if err := checkStart(config, parts); err != nil {
		return nil, err
	}
	host, _, err := net.SplitHostPort(config.Listen)
	if err != nil {
		return nil, fmt.Errorf("the relay's listen address %q: %w", config.Listen, err)
	}
	// Datagrams between two allocations go to the relayed sockets' own host, not out to the
	// public address and back, which a VPS behind a 1:1 NAT may not loop.
	bound := net.ParseIP(host)
	if bound == nil || bound.IsUnspecified() {
		host, bound = "0.0.0.0", net.IPv4(127, 0, 0, 1)
	}
	certificate, err := loadCertificateFiles(ctx, config.CertificateFile, config.KeyFile, parts.Logger)
	if err != nil {
		return nil, err
	}
	listened, err := listen(ctx, config)
	if err != nil {
		return nil, err
	}
	packets, streams, secured := listened.packets, listened.streams, listened.secured
	ledger := newLedger(ctx, config.PublicAddress, bound, parts, portOf(secured.Addr()))
	picker := newSecretPicker(config.Secrets)
	relayed := func() pionturn.RelayAddressGenerator {
		return &closedRelays{ledger: ledger, RelayAddressGeneratorPortRange: &pionturn.RelayAddressGeneratorPortRange{
			RelayAddress: config.PublicAddress, Address: host,
			MinPort: config.RelayPortMin, MaxPort: config.RelayPortMax,
		}}
	}
	// Every permission is granted, and the relayed sockets alone keep the relay closed (D560):
	// pion refuses one with no error code, which Firefox takes as the relay having failed, and
	// a browser asks one for each of the core's candidates.
	permit := func(net.Addr, net.IP) bool { return true }
	tlsStreams := tls.NewListener(secured, &tls.Config{
		MinVersion: tls.VersionTLS12, GetCertificate: certificate.get,
	})
	server, err := pionturn.NewServer(pionturn.ServerConfig{
		Realm:         relaycred.Realm,
		AuthHandler:   ledger.authHandler(picker),
		QuotaHandler:  ledger.mayAllocate,
		EventHandler:  ledger.events(),
		LoggerFactory: turnserver.Loggers(ctx, parts.Logger),
		PacketConnConfigs: []pionturn.PacketConnConfig{{
			PacketConn:            &inspectedPackets{PacketConn: packets, picker: picker},
			RelayAddressGenerator: relayed(), PermissionHandler: permit,
		}},
		ListenerConfigs: []pionturn.ListenerConfig{
			{
				Listener:              &inspectedStreams{Listener: streams, picker: picker},
				RelayAddressGenerator: relayed(), PermissionHandler: permit,
			},
			{
				Listener:              &inspectedStreams{Listener: tlsStreams, picker: picker},
				RelayAddressGenerator: relayed(), PermissionHandler: permit,
			},
		},
	})
	if err != nil {
		_ = packets.Close()    //nolint:errcheck // the start's error is the one to report
		_ = streams.Close()    //nolint:errcheck // the start's error is the one to report
		_ = tlsStreams.Close() //nolint:errcheck // the start's error is the one to report
		return nil, fmt.Errorf("start the relay: %w", err)
	}
	daysCtx, stopDays := context.WithCancel(ctx)
	relay := &Relay{
		server: server, address: packets.LocalAddr(), tlsAddress: secured.Addr(), stopDays: stopDays,
		daysStopped: make(chan struct{}), ledger: ledger, announced: make(chan struct{}),
	}
	go func() {
		defer close(relay.daysStopped)
		ledger.keep(daysCtx)
	}()
	go relay.announce(daysCtx, config, parts)
	parts.Logger.InfoContext(ctx, "the relay is serving", slog.String(slogx.EventKey, "relayStarted"),
		slog.String("name", config.Name), slog.String("listen", relay.address.String()),
		slog.String("listenTls", relay.tlsAddress.String()), slog.Int("secrets", len(config.Secrets)))
	return relay, nil
}

// checkStart refuses a configuration that is not whole, and one that announces with no HTTP client.
func checkStart(config Config, parts Parts) error {
	if err := config.check(); err != nil {
		return err
	}
	if len(config.Announce) > 0 && parts.HTTP == nil {
		return errors.New("the relay announces with no HTTP client")
	}
	return nil
}

// announce has the relay announce itself to each discovery config names until ctx ends (D567 §1).
func (relay *Relay) announce(ctx context.Context, config Config, parts Parts) {
	defer close(relay.announced)
	if len(config.Announce) == 0 {
		return
	}
	announcing := &announcer{
		name: config.Name, targets: config.Announce, parts: parts, states: make([]string, len(config.Announce)),
		target: &net.UDPAddr{IP: config.PublicAddress, Port: portOf(relay.address)},
	}
	announcing.keep(ctx)
}

// listeners are the relay's: UDP and TCP on one port, and TCP for TLS.
type listeners struct {
	packets          net.PacketConn
	streams, secured net.Listener
}

// listen listens over UDP and TCP on one port, and over TCP for TLS.
func listen(ctx context.Context, config Config) (listeners, error) {
	packets, streams, err := turnserver.ListenOnOnePort(ctx, config.Listen, (&net.ListenConfig{}).Listen)
	if err != nil {
		return listeners{}, fmt.Errorf("start the relay: %w", err)
	}
	secured, err := (&net.ListenConfig{}).Listen(ctx, "tcp", config.ListenTLS)
	if err != nil {
		_ = packets.Close() //nolint:errcheck // the listen's error is the one to report
		_ = streams.Close() //nolint:errcheck // the listen's error is the one to report
		return listeners{}, fmt.Errorf("start the relay's TLS: %w", err)
	}
	return listeners{packets: packets, streams: streams, secured: secured}, nil
}

// check is why config cannot start a relay, or nil.
func (config Config) check() error {
	switch {
	case config.PublicAddress.To4() == nil:
		return errors.New("the relay's public address must be an IPv4 address")
	case len(config.Secrets) == 0:
		return errors.New("the relay admits no credential: it has no secret")
	case config.Name == "" && len(config.Announce) > 0:
		return errors.New("the relay announces with no name: name it as the discovery names it")
	case config.RelayPortMin == 0 || config.RelayPortMax < config.RelayPortMin:
		return fmt.Errorf("the relayed ports %d to %d are no range", config.RelayPortMin, config.RelayPortMax)
	}
	for _, announce := range config.Announce {
		if err := announce.check(); err != nil {
			return err
		}
	}
	_, plain, plainErr := net.SplitHostPort(config.Listen)
	_, secured, securedErr := net.SplitHostPort(config.ListenTLS)
	if plainErr == nil && securedErr == nil && plain == secured && plain != "0" {
		return fmt.Errorf("TURN and TURN over TLS both on port %s; each needs its own", plain)
	}
	return nil
}

// portOf is address's port, or 0.
func portOf(address net.Addr) int {
	_, port, err := net.SplitHostPort(address.String())
	if err != nil {
		return 0
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return number
}

// Address is where TURN is served over UDP and TCP, its port chosen when Listen named none.
func (relay *Relay) Address() net.Addr { return relay.address }

// TLSAddress is where TURN over TLS is served.
func (relay *Relay) TLSAddress() net.Addr { return relay.tlsAddress }

// Dropped is how many datagrams the relay has dropped: to or from anything that is not the
// household's other end, or beyond its rate.
func (relay *Relay) Dropped() int64 { return relay.ledger.dropped.Load() }

// Close stops the relay and every allocation on it.
func (relay *Relay) Close() error {
	relay.stopDays()
	<-relay.daysStopped
	<-relay.announced
	if err := relay.server.Close(); err != nil {
		return fmt.Errorf("stop the relay: %w", err)
	}
	return nil
}
