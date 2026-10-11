// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"crypto/tls"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/certificate"
	"github.com/kstenerud/orpheum/internal/core/certificate/certificatetest"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare"
	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare/cloudflaretest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// discoveryName is the wildcard the tests' discovery publishes the core's names under.
const discoveryName = "*.k7xq2.orpheum-app.com"

// certificateParts are a certificate's parts over a fake CA and a fake Cloudflare, but for its name.
func certificateParts(t *testing.T) certificate.Parts {
	t.Helper()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	dns := cloudflaretest.New(t, "orpheum-app.com")
	authority := certificatetest.NewCA(t, dns, now, 90*24*time.Hour)
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	directories, err := instancedir.Open(instancedir.PathsIn(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return certificate.Parts{
		DirectoryURL: authority.Directory(), HTTP: httpClient,
		DNS: certificate.CloudflareDNS(
			cloudflare.New(httpClient, dns.Server.URL, cloudflaretest.Keys(cloudflaretest.Token))),
		Files: directories, Clock: clocktest.NewFake(now), Logger: slog.New(slog.DiscardHandler),
	}
}

// servedName is the first name of the certificate served for serverName.
func servedName(t *testing.T, served *servedCertificates, serverName string) string {
	t.Helper()
	chosen, err := served.GetCertificate(&tls.ClientHelloInfo{ServerName: serverName})
	if err != nil {
		t.Fatalf("GetCertificate(%s) = %v", serverName, err)
	}
	return chosen.Leaf.DNSNames[0]
}

// A browser is served the discovery's wildcard for a name one label under it, and the owner's own
// name's certificate for any other (D273).
func TestServedCertificates_ServesEachNameTheCertificateForIt(t *testing.T) {
	t.Parallel()
	parts := certificateParts(t)
	ownParts := parts
	ownParts.Domain = "media.orpheum-app.com"
	own, err := certificate.New(ownParts)
	if err != nil {
		t.Fatal(err)
	}
	served := &servedCertificates{own: own, discoveryParts: parts}
	if err = served.useDiscoveryName(discoveryName); err != nil {
		t.Fatal(err)
	}
	if err = served.HandleRenew(t.Context(), jobs.Job{Kind: jobs.KindRenewCertificate}); err != nil {
		t.Fatal(err)
	}
	for serverName, want := range map[string]string{
		"media.orpheum-app.com":                       "media.orpheum-app.com",
		"2a02-8070-0-0-0-0-0-1.k7xq2.orpheum-app.com": discoveryName,
		"192-168-1-20.K7XQ2.orpheum-app.com":          discoveryName,
		"a.b.k7xq2.orpheum-app.com":                   "media.orpheum-app.com",
		"k7xq2.orpheum-app.com":                       "media.orpheum-app.com",
	} {
		if got := servedName(t, served, serverName); got != want {
			t.Errorf("for %s the certificate served is %s's; want %s's", serverName, got, want)
		}
	}
}

// A server with no public name opens HTTPS once its discovery's certificate is obtained, and only
// once; a door closed before then is never opened.
func TestServedCertificates_HTTPSOpensOnceTheDiscoverysCertificateCanBeServed(t *testing.T) {
	t.Parallel()
	served := &servedCertificates{discoveryParts: certificateParts(t)}
	opened := 0
	served.whenServable(func() { opened++ })
	if err := served.useDiscoveryName(discoveryName); err != nil {
		t.Fatal(err)
	}
	if opened != 0 {
		t.Fatalf("HTTPS opened %d times before there was a certificate; want none", opened)
	}
	for range 2 {
		if err := served.HandleRenew(t.Context(), jobs.Job{Kind: jobs.KindRenewCertificate}); err != nil {
			t.Fatal(err)
		}
	}
	if opened != 1 {
		t.Errorf("HTTPS opened %d times once the certificate was obtained; want once", opened)
	}
	door := &lateDoor{}
	if err := door.shut(); err != nil {
		t.Fatal(err)
	}
	listened := false
	listen := func() (func() error, error) {
		listened = true
		return func() error { return nil }, nil
	}
	if err := door.open(listen); err != nil || listened {
		t.Errorf("a door closed first opened = %v, listened %t; want it left shut", err, listened)
	}
}

// A server with neither a public name nor a discovery has no certificate to renew.
func TestServedCertificates_NoNameAndNoDiscoveryHaveNothingToRenew(t *testing.T) {
	t.Parallel()
	served := &servedCertificates{}
	err := served.HandleRenew(t.Context(), jobs.Job{Kind: jobs.KindRenewCertificate})
	if !errors.Is(err, errNoPublicName) || !jobs.IsMarkedPermanent(err) {
		t.Errorf("a renewal with nothing to renew = %v; want errNoPublicName, asking no retry", err)
	}
}
