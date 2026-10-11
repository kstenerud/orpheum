// SPDX-License-Identifier: AGPL-3.0-or-later

package cloudflare_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare"
	"github.com/kstenerud/orpheum/internal/core/providers/cloudflare/cloudflaretest"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
)

// newClient is a client of the fake at base, opening keys.
func newClient(t *testing.T, base string, keys cloudflaretest.Keys) *cloudflare.Client {
	t.Helper()
	httpClient, err := outbound.NewClient(5*time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	return cloudflare.New(httpClient, base, keys)
}

// A TXT record is set in the zone the token reaches that holds its name, the longest such, and
// removed again (D243 §6).
func TestClient_SetsAndRemovesATXTRecordInTheNamesZone(t *testing.T) {
	t.Parallel()
	fake := cloudflaretest.New(t, "orpheum-app.com", "example.org")
	client := newClient(t, fake.Server.URL, cloudflaretest.Token)
	name := "_acme-challenge.media.orpheum-app.com"
	record, err := client.SetTXT(t.Context(), name, "proof")
	if err != nil || fake.TXT(name) != "proof" || record.ZoneID != "zone1" {
		t.Fatalf("SetTXT = %+v, %v; the fake holds %q; want it set in orpheum-app.com's zone", record, err,
			fake.TXT(name))
	}
	if err = client.RemoveTXT(t.Context(), record); err != nil || fake.RecordCount() != 0 {
		t.Errorf("RemoveTXT = %v, %d records left; want none", err, fake.RecordCount())
	}
}

// The name's zone is found however many zones the token reaches: Cloudflare lists them a page at a
// time, and the zone was missed beyond the first (M4's audit).
func TestClient_FindsTheNamesZoneAmongMoreThanAPageOfZones(t *testing.T) {
	t.Parallel()
	zones := make([]string, 0, 60)
	for index := range 59 {
		zones = append(zones, fmt.Sprintf("zone-%02d.example.org", index))
	}
	fake := cloudflaretest.New(t, append(zones, "orpheum-app.com")...)
	client := newClient(t, fake.Server.URL, cloudflaretest.Token)
	name := "_acme-challenge.media.orpheum-app.com"
	if record, err := client.SetTXT(t.Context(), name, "proof"); err != nil || record.ZoneID != "zone60" {
		t.Errorf("SetTXT = %+v, %v; want it set in orpheum-app.com's zone, the 60th", record, err)
	}
}

// What stops a record being set is said (go.md §21): a token not set or refused, a name no zone
// holds, a host down, an answer not Cloudflare's.
func TestClient_SaysWhyItCannotSetARecord(t *testing.T) {
	t.Parallel()
	fake := cloudflaretest.New(t, "orpheum-app.com")
	down := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(down.Close)
	odd := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"success": false}`)) //nolint:errcheck // a test's fake
	}))
	t.Cleanup(odd.Close)
	for _, check := range []struct {
		what   string
		client *cloudflare.Client
		name   string
		want   error
	}{
		{
			"no token", newClient(t, fake.Server.URL, ""), "_acme-challenge.media.orpheum-app.com",
			cloudflare.ErrTokenRefused,
		},
		{
			"a refused token", newClient(t, fake.Server.URL, "wrong"), "_acme-challenge.media.orpheum-app.com",
			cloudflare.ErrTokenRefused,
		},
		{
			"a name in no zone", newClient(t, fake.Server.URL, cloudflaretest.Token), "_acme-challenge.media.other.net",
			cloudflare.ErrNoZone,
		},
		{"Cloudflare down", newClient(t, down.URL, cloudflaretest.Token), "x.orpheum-app.com", outbound.ErrUnavailable},
		{
			"an answer not Cloudflare's", newClient(t, odd.URL, cloudflaretest.Token), "x.orpheum-app.com",
			cloudflare.ErrMalformed,
		},
	} {
		if _, err := check.client.SetTXT(t.Context(), check.name, "proof"); !errors.Is(err, check.want) {
			t.Errorf("%s: SetTXT = %v, want %v", check.what, err, check.want)
		}
	}
}
