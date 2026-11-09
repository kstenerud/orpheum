// SPDX-License-Identifier: AGPL-3.0-or-later

package certificate

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/certificate/certificatetest"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare"
	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare/cloudflaretest"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// domain is the name the tests' certificate is for.
const domain = "media.orpheum-app.com"

// memoryFiles is a persistent directory in memory.
type memoryFiles struct {
	mutex sync.Mutex
	files map[string][]byte
}

func (files *memoryFiles) ReadPersistentFile(name string) ([]byte, error) {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	contents, found := files.files[name]
	if !found {
		return nil, instancedir.ErrNotFound
	}
	return contents, nil
}

func (files *memoryFiles) WritePersistentFile(name string, contents []byte) error {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	files.files[name] = contents
	return nil
}

// heardQueue keeps the renewals asked for.
type heardQueue struct{ requests []jobs.Request }

func (queue *heardQueue) Enqueue(_ context.Context, request jobs.Request) (bool, error) {
	queue.requests = append(queue.requests, request)
	return true, nil
}

// bench is a certificate service over a fake CA and a fake Cloudflare, its files and its queue.
type bench struct {
	service   *Service
	authority *certificatetest.CA
	dns       *cloudflaretest.Fake
	files     *memoryFiles
	queue     *heardQueue
	clock     *clocktest.Fake
	parts     Parts
}

func newBench(t *testing.T, token cloudflaretest.Keys) bench {
	t.Helper()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	dns := cloudflaretest.New(t, "orpheum-app.com")
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	fakeClock := clocktest.NewFake(now)
	files, queue := &memoryFiles{files: map[string][]byte{}}, &heardQueue{}
	authority := certificatetest.NewCA(t, dns, now, 90*24*time.Hour)
	parts := Parts{
		Domain: domain, Contact: "owner@example.org", DirectoryURL: authority.Directory(), HTTP: httpClient,
		DNS:   CloudflareDNS(cloudflare.New(httpClient, dns.Server.URL, token)),
		Files: files, Queue: queue, Clock: fakeClock,
		Logger: slog.New(slog.DiscardHandler),
	}
	service, err := New(parts)
	if err != nil {
		t.Fatal(err)
	}
	return bench{
		service: service, authority: authority, dns: dns, files: files, queue: queue, clock: fakeClock, parts: parts,
	}
}

// The certificate is obtained by proving the name with a DNS-01 record set through Cloudflare, which
// is removed again; it is served at once, kept, and served again by a server that starts later
// (D243 §6).
func TestObtain_ProvesTheNameByDNSServesTheCertificateAndKeepsIt(t *testing.T) {
	t.Parallel()
	bench := newBench(t, cloudflaretest.Token)
	if _, err := bench.service.GetCertificate(nil); !errors.Is(err, ErrNoCertificate) {
		t.Errorf("before any is obtained, GetCertificate = %v; want ErrNoCertificate", err)
	}
	if err := bench.service.Obtain(t.Context()); err != nil {
		t.Fatal(err)
	}
	served, err := bench.service.GetCertificate(nil)
	if err != nil || served.Leaf == nil || len(served.Leaf.DNSNames) != 1 || served.Leaf.DNSNames[0] != domain {
		t.Fatalf("GetCertificate = %+v, %v; want a certificate for %s", served, err, domain)
	}
	if set := bench.dns.EverSet("_acme-challenge." + domain); len(set) != 1 || bench.dns.RecordCount() != 0 {
		t.Errorf("the challenge's records: set %v, %d left; want one set and removed", set, bench.dns.RecordCount())
	}
	again, err := New(bench.parts)
	if err != nil {
		t.Fatal(err)
	}
	kept, keptErr := again.GetCertificate(nil)
	if keptErr != nil || kept.Leaf.SerialNumber.Cmp(served.Leaf.SerialNumber) != 0 {
		t.Errorf("a server started later serves %v, %v; want the kept certificate", kept, keptErr)
	}
}

// A certificate kept for another name, before the configured domain changed, is not served, and a
// new one is asked for at once (M4's audit): it was served until a month before it expired.
func TestNew_ACertificateKeptForAnotherNameIsNotServed(t *testing.T) {
	t.Parallel()
	bench := newBench(t, cloudflaretest.Token)
	if err := bench.service.Obtain(t.Context()); err != nil {
		t.Fatal(err)
	}
	renamed := bench.parts
	renamed.Domain = "films.orpheum-app.com"
	service, err := New(renamed)
	if err != nil {
		t.Fatal(err)
	}
	if served, err := service.GetCertificate(nil); !errors.Is(err, ErrNoCertificate) {
		t.Errorf("after the domain changed, GetCertificate = %v, %v; want ErrNoCertificate", served, err)
	}
	if asked, err := service.QueueRenewal(t.Context()); err != nil || asked != 1 {
		t.Errorf("QueueRenewal asked for %d (%v); want one, for the new name", asked, err)
	}
}

// A renewal is asked for when there is no certificate, and when the one there expires within a
// month, and not before.
func TestQueueRenewal_AsksWhenThereIsNoneOrItExpiresWithinAMonth(t *testing.T) {
	t.Parallel()
	bench := newBench(t, cloudflaretest.Token)
	if asked, err := bench.service.QueueRenewal(t.Context()); err != nil || asked != 1 {
		t.Errorf("with no certificate, QueueRenewal asked for %d (%v); want one", asked, err)
	}
	if err := bench.service.Obtain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if asked, err := bench.service.QueueRenewal(t.Context()); err != nil || asked != 0 {
		t.Errorf("with a new certificate, QueueRenewal asked for %d (%v); want none", asked, err)
	}
	bench.clock.Advance(61 * 24 * time.Hour)
	if asked, err := bench.service.QueueRenewal(t.Context()); err != nil || asked != 1 {
		t.Errorf("29 days before it expires, QueueRenewal asked for %d (%v); want one", asked, err)
	}
	if len(bench.queue.requests) != 2 || bench.queue.requests[1].Kind != jobs.KindRenewCertificate {
		t.Errorf("asked for %+v; want two renewals", bench.queue.requests)
	}
}

// renewSettings answers certificateRenewBeforeDays with its days.
type renewSettings struct{ days int }

func (renew renewSettings) Duration(_ context.Context, key settings.Key) (time.Duration, error) {
	if key != settings.CertificateRenewBeforeDays {
		return 0, errors.New("not asked for")
	}
	return time.Duration(renew.days) * 24 * time.Hour, nil
}

// How early a certificate is renewed is a setting (the constants sweep, 2026-10-20): at five days, one
// 29 days from expiring is not yet due, and one four days from it is.
func TestQueueRenewal_AsksWithinTheSettingsDays(t *testing.T) {
	t.Parallel()
	bench := newBench(t, cloudflaretest.Token)
	bench.service.parts.Settings = renewSettings{days: 5}
	if err := bench.service.Obtain(t.Context()); err != nil {
		t.Fatal(err)
	}
	bench.clock.Advance(61 * 24 * time.Hour)
	if asked, err := bench.service.QueueRenewal(t.Context()); err != nil || asked != 0 {
		t.Errorf("29 days before it expires, renewing 5 days before, asked for %d (%v); want none", asked, err)
	}
	bench.clock.Advance(25 * 24 * time.Hour)
	if asked, err := bench.service.QueueRenewal(t.Context()); err != nil || asked != 1 {
		t.Errorf("4 days before it expires, renewing 5 days before, asked for %d (%v); want one", asked, err)
	}
}

// A token not set, or refused, fails the renewal for the admin to fix, and no certificate is asked
// of the CA.
func TestHandleRenew_ARefusedTokenFailsForTheAdminToFix(t *testing.T) {
	t.Parallel()
	bench := newBench(t, "")
	err := bench.service.HandleRenew(t.Context(), jobs.Job{Kind: jobs.KindRenewCertificate})
	if !errors.Is(err, cloudflare.ErrTokenRefused) || bench.authority.Issued() != 0 {
		t.Errorf("a renewal with no token = %v, %d issued; want ErrTokenRefused and none", err,
			bench.authority.Issued())
	}
}

// A wildcard is obtained by proving the name under it, and is kept under its own prefix, apart from
// the owner's own name's: each is served again after a restart, and neither is taken for the other
// (D273).
func TestObtain_AWildcardIsProvenByTheNameUnderItAndKeptApart(t *testing.T) {
	t.Parallel()
	bench := newBench(t, cloudflaretest.Token)
	wildcard := bench.parts
	wildcard.Domain, wildcard.FilePrefix = "*.k7xq2.orpheum-app.com", "discovery-"
	service, err := New(wildcard)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Obtain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if set := bench.dns.EverSet("_acme-challenge.k7xq2.orpheum-app.com"); len(set) != 1 {
		t.Errorf("the wildcard's challenge set %v; want one record, for the name under it", set)
	}
	again, err := New(wildcard)
	if err != nil {
		t.Fatal(err)
	}
	if served, servedErr := again.GetCertificate(nil); servedErr != nil || served.Leaf.DNSNames[0] != wildcard.Domain {
		t.Errorf("after a restart the wildcard serves %v, %v; want the kept wildcard", served, servedErr)
	}
	own, err := New(bench.parts)
	if err != nil {
		t.Fatal(err)
	}
	if served, servedErr := own.GetCertificate(nil); !errors.Is(servedErr, ErrNoCertificate) {
		t.Errorf("the own name's service serves %v, %v; want none, the wildcard kept apart", served, servedErr)
	}
}

// reportingDNS is a DNS that wants the ACME account, as a discovery does, and keeps in order what it
// was told and asked.
type reportingDNS struct {
	DNS
	mutex sync.Mutex
	heard []string
}

func (dns *reportingDNS) ReportACMEAccount(_ context.Context, accountURI string) error {
	dns.mutex.Lock()
	defer dns.mutex.Unlock()
	dns.heard = append(dns.heard, "account "+accountURI)
	return nil
}

func (dns *reportingDNS) SetTXT(ctx context.Context, name, value string) (func(context.Context) error, error) {
	dns.mutex.Lock()
	dns.heard = append(dns.heard, "challenge "+name)
	dns.mutex.Unlock()
	return dns.DNS.SetTXT(ctx, name, value)
}

// A DNS that holds the zone's CAA is told the order's ACME account before any challenge is set, so
// the CAA can name it (the discovery protocol's §2.7; RFC 8657).
func TestObtain_TellsADNSThatWantsItTheAccountBeforeTheChallenge(t *testing.T) {
	t.Parallel()
	bench := newBench(t, cloudflaretest.Token)
	reporting := &reportingDNS{DNS: bench.parts.DNS}
	parts := bench.parts
	parts.DNS = reporting
	service, err := New(parts)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Obtain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(reporting.heard) != 2 || !strings.HasSuffix(reporting.heard[0], "/account/1") ||
		reporting.heard[1] != "challenge _acme-challenge."+domain {
		t.Errorf("the DNS heard %q; want the account's URI, then the challenge", reporting.heard)
	}
}
