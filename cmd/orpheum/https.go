// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/certificate"
	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/platform/pacing"
)

// propagationWaitOf is how long a DNS-01 challenge's record is given to reach the name's servers:
// certificate_dns_wait_seconds as reader holds it, read at each challenge; nil, no wait, for a wiring
// that waits for none, a test's fake DNS (D243 §6).
func propagationWaitOf(
	waitsForDNS bool, reader settings.DurationReader, logger *slog.Logger,
) func(ctx context.Context) time.Duration {
	if !waitsForDNS {
		return nil
	}
	return func(ctx context.Context) time.Duration {
		return settings.DurationOrStarting(ctx, reader, settings.CertificateDNSWaitSeconds, logger)
	}
}

// openCertificates makes the certificates HTTPS serves: the server's public name's, through
// Cloudflare, for a server with an https.domain (D243 §6); the discovery's is made once the
// discovery names the core (openReachedFromOutside).
func openCertificates(
	config Config, opened openedServices, outside wiring, logger *slog.Logger,
) (openedServices, error) {
	parts := certificate.Parts{
		Contact: config.HTTPS.Contact, DirectoryURL: config.HTTPS.ACMEDirectory,
		HTTP: opened.jobServices.outboundClient, Files: opened.directories, Queue: opened.jobServices.queue,
		Clock: clock.System{}, Settings: opened.settings, Logger: logger,
	}
	parts.PropagationWait = propagationWaitOf(outside.waitsForDNS, opened.settings, logger)
	served := &servedCertificates{discoveryParts: parts}
	opened.jobServices.certificates = served
	if !config.HTTPS.isOn() {
		return opened, nil
	}
	own := parts
	own.Domain = config.HTTPS.Domain
	own.DNS = certificate.CloudflareDNS(
		cloudflare.New(opened.jobServices.outboundClient, outside.cloudflareBaseURL, opened.secrets))
	service, err := certificate.New(own)
	if err != nil {
		return opened, err
	}
	served.own = service
	renewWhenTokenSet(opened.secrets, service, logger)
	return opened, nil
}

// renewWhenTokenSet has an admin's setting of the DNS provider's token ask for the certificate at
// once, by the renewal the sweep asks for, rather than at the sweep's next daily look: the renewal
// queued at start, before any token, failed for want of one and was not tried again, so staging
// served no certificate until the core restarted (2026-10-06). A person's action tries again what
// failed before it (D248). Clearing the token asks for nothing.
func renewWhenTokenSet(secretStore *secrets.Store, certificates *certificate.Service, logger *slog.Logger) {
	secretStore.OnChange(func(ctx context.Context, name secrets.Name) {
		if name != secrets.DNSProviderToken {
			return
		}
		if _, err := secretStore.Open(ctx, name); errors.Is(err, secrets.ErrNotSet) {
			return
		}
		if _, err := certificates.QueueRenewal(ctx); err != nil {
			logger.ErrorContext(ctx,
				"the DNS provider's token was set, but the certificate's renewal could not be queued", "err", err)
		}
	})
}

// listenForMedia listens on address for the core's clients, its connections sending with BBR, as
// the films they carry should on a lossy link (D263 §3); a kernel without it is said so once, and its
// default kept. The host loads tcp_bbr (deploy/staging/README.md).
func listenForMedia(ctx context.Context, address string, logger *slog.Logger) (net.Listener, error) {
	listener, err := pacing.ListenConfig(func(refused error) {
		logger.WarnContext(ctx, "films are sent with the kernel's default congestion control, not BBR; "+
			"load the tcp_bbr module on the host", "address", address, "err", refused)
	}).Listen(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}
	return listener, nil
}

// listenForClients listens where the server's plain HTTP is served: its own address, and the port
// only the server's own machine reaches beside it when the config names one (D334 §4), one listener.
func listenForClients(ctx context.Context, server ServerConfig, logger *slog.Logger) (net.Listener, error) {
	listener, err := listenForMedia(ctx, server.Listen, logger)
	if err != nil || server.ThisMachineListen == "" {
		return listener, err
	}
	thisMachine, err := listenForMedia(ctx, server.ThisMachineListen, logger)
	if err != nil {
		return nil, errors.Join(err, listener.Close())
	}
	return joinListeners(listener, thisMachine), nil
}

// listenAndServeHTTPS serves handler over HTTPS on https.listen, as serveHTTPS says: at once for a
// server with a public name, and for one without, once its discovery's certificate can be served
// (D273); nothing before. It returns what stops it.
func listenAndServeHTTPS(
	ctx context.Context, config Config, opened openedServices, handler http.Handler, logger *slog.Logger,
) (func() error, error) {
	served := opened.jobServices.certificates
	listen := func() (func() error, error) {
		listener, err := listenForMedia(ctx, config.HTTPS.Listen, logger)
		if err != nil {
			return nil, fmt.Errorf("listen for HTTPS on %s: %w", config.HTTPS.Listen, err)
		}
		return serveHTTPS(ctx, config, listener, served, handler, logger), nil
	}
	if served.own != nil {
		return listen()
	}
	door := &lateDoor{}
	served.whenServable(func() {
		if err := door.open(listen); err != nil {
			logger.ErrorContext(ctx, "HTTPS could not start for the discovery's names", "err", err)
		}
	})
	return door.shut, nil
}

// lateDoor is a front door opened once something can be served through it, if it has not been
// closed first.
type lateDoor struct {
	mutex    sync.Mutex
	stop     func() error
	isClosed bool
}

// open opens the door with listen, once, unless it is closed.
func (door *lateDoor) open(listen func() (func() error, error)) error {
	door.mutex.Lock()
	defer door.mutex.Unlock()
	if door.isClosed || door.stop != nil {
		return nil
	}
	stop, err := listen()
	if err != nil {
		return err
	}
	door.stop = stop
	return nil
}

// shut closes the door, stopping what open started.
func (door *lateDoor) shut() error {
	door.mutex.Lock()
	defer door.mutex.Unlock()
	door.isClosed = true
	if door.stop == nil {
		return nil
	}
	return door.stop()
}

// certificateSource is what serves a browser the certificate for the name it asks for.
type certificateSource interface {
	GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
}

// serveHTTPS serves handler over HTTPS on listener with the certificate service's certificate, a
// renewed one served as soon as it is kept, until the returned function stops it.
func serveHTTPS(
	ctx context.Context, config Config, listener net.Listener, certificates certificateSource,
	handler http.Handler, logger *slog.Logger,
) func() error {
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: config.readHeaderTimeout(), ConnContext: pacing.ConnContext,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: certificates.GetCertificate},
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if serveErr := server.ServeTLS(listener, "", ""); !errors.Is(serveErr, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "the HTTPS server stopped", "err", serveErr)
		}
	}()
	logger.InfoContext(ctx, "serving HTTPS", "domain", config.HTTPS.Domain, "address", listener.Addr().String())
	return func() error {
		shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.shutdownGrace())
		defer cancel()
		err := server.Shutdown(shutdownContext)
		<-done
		return err
	}
}

// certificateExpiry is the System page's view of the certificates: nil for a server with no public
// name, never a nil service in a non-nil interface.
func certificateExpiry(certificates *servedCertificates) api.CertificateExpiry {
	if certificates.own == nil {
		return nil
	}
	return certificates
}
