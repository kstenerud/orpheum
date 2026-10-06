// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/certificate"
	"github.com/kstenerud/orpheum/internal/core/certificate/certificatetest"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare"
	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare/cloudflaretest"
	"github.com/kstenerud/orpheum/internal/core/secrets"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// The server's public name is served over HTTPS with the certificate obtained for it, which a
// browser trusting the CA accepts for that name (D243 §6).
func TestServeHTTPS_ServesTheNamesCertificate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
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
	certificates, err := certificate.New(certificate.Parts{
		Domain: "media.orpheum-app.com", Contact: "owner@example.org", DirectoryURL: authority.Directory(),
		HTTP: httpClient, DNS: cloudflare.New(httpClient, dns.Server.URL, cloudflaretest.Keys(cloudflaretest.Token)),
		Files: directories, Clock: clocktest.NewFake(now), Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = certificates.Obtain(t.Context()); err != nil {
		t.Fatal(err)
	}
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	config, _ := testConfig(t)
	stop := serveHTTPS(t.Context(), config, listener, certificates,
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte("hello")) //nolint:errcheck // the test reads it, or fails
		}),
		slog.New(slog.DiscardHandler))
	t.Cleanup(func() {
		if stopErr := stop(); stopErr != nil {
			t.Error(stopErr)
		}
	})
	browser := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: authority.Roots(), ServerName: "media.orpheum-app.com", MinVersion: tls.VersionTLS12,
		Time: func() time.Time { return now }, // the test's time, never the machine's
	}}}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+listener.Addr().String()+"/",
		http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := browser.Do(request)
	if err != nil {
		t.Fatalf("a browser trusting the CA, for media.orpheum-app.com: %v", err)
	}
	defer func() { _ = answer.Body.Close() }() //nolint:errcheck // read to the end below
	body, err := io.ReadAll(answer.Body)
	if err != nil || string(body) != "hello" {
		t.Errorf("served %q, %v; want the handler's answer", body, err)
	}
}

// Setting the DNS provider's token asks for the certificate at once: the renewal queued at start,
// before any token, had failed and waited for the sweep's next daily look (staging, 2026-10-06).
// Clearing it asks for nothing.
func TestRenewWhenTokenSet_SettingTheTokenAsksForTheCertificateAtOnce(t *testing.T) {
	t.Parallel()
	world := newJobsWorld(t, time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC))
	directories, err := instancedir.Open(instancedir.PathsIn(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	certificates, err := certificate.New(certificate.Parts{
		Domain: "media.orpheum-app.com", Files: directories, Queue: world.queue, Clock: world.clock,
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	renewWhenTokenSet(world.secrets, certificates, slog.New(slog.DiscardHandler))
	renewals := func() int {
		listed, listErr := world.cache.Read().ListJobs(t.Context(), 100)
		if listErr != nil {
			t.Fatal(listErr)
		}
		count := 0
		for _, job := range listed {
			if job.Kind == string(jobs.KindRenewCertificate) {
				count++
			}
		}
		return count
	}
	ctx := t.Context()
	if err = world.secrets.Clear(ctx, secrets.DNSProviderToken); err != nil {
		t.Fatal(err)
	}
	if got := renewals(); got != 0 {
		t.Fatalf("clearing the token queued %d renewals; want none", got)
	}
	if err = world.secrets.Set(ctx, secrets.DNSProviderToken, secrets.NewValue("cloudflare-token"), nil); err != nil {
		t.Fatal(err)
	}
	if got := renewals(); got != 1 {
		t.Errorf("setting the token queued %d renewals; want one, at once", got)
	}
}
