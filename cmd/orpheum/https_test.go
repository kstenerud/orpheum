// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"crypto/rand"
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
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/jobqueue/jobqueuetest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/storetest"
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
		HTTP: httpClient,
		DNS: certificate.CloudflareDNS(
			cloudflare.New(httpClient, dns.Server.URL, cloudflaretest.Keys(cloudflaretest.Token))),
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

// renewalWorld is what a certificate's renewal is queued on: the job queue over cache.db on a fake
// clock, and the secrets a person sets the DNS provider's token in.
type renewalWorld struct {
	clock   *clocktest.Fake
	cache   *store.Cache
	queue   *jobs.Queue
	secrets *secrets.Store
}

func newRenewalWorld(t *testing.T, start time.Time) renewalWorld {
	t.Helper()
	database := storetest.OpenMigrated(t)
	config := settingstest.Seeded(t, database)
	key, err := secrets.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	world := renewalWorld{clock: clocktest.NewFake(start), cache: storetest.OpenCache(t)}
	world.secrets = secrets.NewStore(database, key, world.clock, rand.Reader, config)
	world.queue = jobs.NewQueue(jobs.QueueParts{
		Cache: world.cache, Clock: world.clock, Settings: config, Random: jobqueuetest.Unjittered(),
		Logger: slog.New(slog.DiscardHandler),
	})
	return world
}

// Setting the DNS provider's token asks for the certificate at once: the renewal queued at start,
// before any token, had failed and waited for the sweep's next daily look (staging, 2026-10-06).
// Clearing it asks for nothing.
func TestRenewWhenTokenSet_SettingTheTokenAsksForTheCertificateAtOnce(t *testing.T) {
	t.Parallel()
	world := newRenewalWorld(t, time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC))
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
		listed, listErr := world.cache.Read().ListJobs(t.Context(), cachegen.ListJobsParams{MaxRows: 100})
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

// A challenge's record is given as long as certificateDnsWaitSeconds says, read at each challenge;
// a wiring that waits for none, a test's, waits none (the constants sweep, 2026-10-20).
func TestPropagationWaitOf_IsTheSettingOrNoneForAFakeDNS(t *testing.T) {
	t.Parallel()
	config := settingstest.Seeded(t, storetest.OpenMigrated(t))
	settingstest.Set(t, config, settings.CertificateDNSWaitSeconds, "45")
	logger := slog.New(slog.DiscardHandler)
	wait := propagationWaitOf(true, config, logger)
	if wait == nil || wait(t.Context()) != 45*time.Second {
		t.Errorf("a server's wait is not the 45 s set")
	}
	if propagationWaitOf(false, config, logger) != nil {
		t.Errorf("a fake DNS's wiring waits; want none")
	}
}
