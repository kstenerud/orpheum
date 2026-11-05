// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strconv"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/core/settings/settingstest"
)

// The throughput test's body is the size the page asks for, within its bounds, of bytes no gzip on
// the way shrinks, never cached, and each one sent is logged with how it came; only while the admin
// offers the device tests, and only to a signed-in page (webrtc-throughput-2026-10-28.md).
func TestLab_TheThroughputTestsBodyIsTheSizeAskedForAndLoggedAsSent(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)

	answer := harness.do(t, http.MethodGet, "/lab/throughput?bytes=1048576", user, nil)
	if answer.status != http.StatusNotFound {
		t.Errorf("the throughput test answered %d while the device tests are off", answer.status)
	}
	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.ShouldOfferDeviceTests, "true")

	const asked = 3<<20 + 5
	answer = harness.do(t, http.MethodGet, "/lab/throughput?leg=relay&bytes="+strconv.Itoa(asked), user, nil)
	if answer.status != http.StatusOK || len(answer.body) != asked {
		t.Fatalf("GET /lab/throughput = %d with %d bytes, want 200 with %d", answer.status, len(answer.body), asked)
	}
	if got := answer.header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := answer.header.Get("Orpheum-Over"); got != "http" {
		t.Errorf("Orpheum-Over = %q, want the path it came over, http", got)
	}
	var squeezed bytes.Buffer
	compressor := gzip.NewWriter(&squeezed)
	if _, err := compressor.Write([]byte(answer.body)); err != nil {
		t.Fatal(err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatal(err)
	}
	if squeezed.Len() < asked*99/100 {
		t.Errorf("gzip shrank the body from %d to %d bytes: a link that compresses would time nothing", asked,
			squeezed.Len())
	}
	harness.logs.AwaitLine(t, `"msg":"a throughput test's body was sent"`, `"leg":"relay"`, `"over":"http"`,
		`"bytes":`+strconv.Itoa(asked), `"isWhole":true`, `"userId":1`)

	for _, bounds := range []string{"1048575", "268435457"} {
		answer = harness.do(t, http.MethodGet, "/lab/throughput?bytes="+bounds, user, nil)
		if answer.status != http.StatusUnprocessableEntity {
			t.Errorf("a body of %s bytes was answered %d", bounds, answer.status)
		}
	}
	answer = harness.do(t, http.MethodGet, "/lab/throughput", "", nil)
	if answer.status != http.StatusUnauthorized {
		t.Errorf("a page signed in to nothing was sent the body: %d", answer.status)
	}
}

// A throughput test's figures are logged under their own words, which the lead finds a device's by.
func TestLab_AThroughputTestsFiguresAreLoggedUnderTheirOwnWords(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)
	user := harness.createAdminAndSignIn(t)
	settingstest.Set(t, harness.server.Dependencies().Load().Settings, settings.ShouldOfferDeviceTests, "true")
	sent := map[string]any{"probe": "throughput", "result": map[string]any{"transportMbps": 41.5}}
	answer := harness.do(t, http.MethodPost, "/lab/results", user, sent)
	if answer.status != http.StatusOK {
		t.Fatalf("POST /lab/results = %d %s", answer.status, answer.body)
	}
	harness.logs.AwaitLine(t, `"msg":"a throughput test ran"`, `"probe":"throughput"`,
		`"result":{"transportMbps":41.5}`)
}
