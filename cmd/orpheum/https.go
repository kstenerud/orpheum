// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/certificate"
	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// dnsPropagationWait is how long a challenge's TXT record is given to reach the name's servers
// before the CA looks: Cloudflare's own servers answer within seconds, and a margin over that costs
// nothing once every two months (D243 §6).
const dnsPropagationWait = 30 * time.Second

// openCertificates makes the certificate service of the server's public name, and asks for a
// certificate at once when there is none; nothing for a server with no https.domain (D243 §6).
func openCertificates(
	ctx context.Context, config Config, opened openedServices, outside wiring, logger *slog.Logger,
) (openedServices, error) {
	if !config.HTTPS.isOn() {
		return opened, nil
	}
	service, err := certificate.New(certificate.Parts{
		Domain: config.HTTPS.Domain, Contact: config.HTTPS.Contact, DirectoryURL: outside.acmeDirectoryURL,
		HTTP:  opened.jobServices.outboundClient,
		DNS:   cloudflare.New(opened.jobServices.outboundClient, outside.cloudflareBaseURL, opened.secrets),
		Files: opened.directories, Queue: opened.jobServices.queue, Clock: clock.System{},
		PropagationWait: outside.dnsPropagationWait, Logger: logger,
	})
	if err != nil {
		return opened, err
	}
	opened.jobServices.certificates = service
	if _, err = service.QueueRenewal(ctx); err != nil {
		return opened, err
	}
	return opened, nil
}

// listenAndServeHTTPS serves handler over HTTPS on https.listen, as serveHTTPS says; nothing for a
// server with no public name. It returns what stops it.
func listenAndServeHTTPS(
	ctx context.Context, config Config, opened openedServices, handler http.Handler, logger *slog.Logger,
) (func() error, error) {
	if opened.jobServices.certificates == nil {
		return func() error { return nil }, nil
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", config.HTTPS.Listen)
	if err != nil {
		return nil, fmt.Errorf("listen for HTTPS on %s: %w", config.HTTPS.Listen, err)
	}
	return serveHTTPS(ctx, config, listener, opened.jobServices.certificates, handler, logger), nil
}

// serveHTTPS serves handler over HTTPS on listener with the certificate service's certificate, a
// renewed one served as soon as it is kept, until the returned function stops it.
func serveHTTPS(
	ctx context.Context, config Config, listener net.Listener, certificates *certificate.Service,
	handler http.Handler, logger *slog.Logger,
) func() error {
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: config.readHeaderTimeout(),
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

// certificateExpiry is the System page's view of the certificate service: nil for a server with no
// public name, never a nil service in a non-nil interface.
func certificateExpiry(certificates *certificate.Service) api.CertificateExpiry {
	if certificates == nil {
		return nil
	}
	return certificates
}
