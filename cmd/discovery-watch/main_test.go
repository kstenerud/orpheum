// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/addressdoc"
	"github.com/kstenerud/orpheum/discovery/trust"
	"github.com/kstenerud/orpheum/discovery/trust/trusttest"
)

// A place serving only what the owner signed is quiet; a statement a stolen key signed, which the
// ledger does not hold, is reported by its key and counter, as is a place that serves nothing it
// should (the root key audit's F4).
func TestRun_ReportsEverySignatureTheLedgerDoesNotHold(t *testing.T) {
	t.Parallel()
	chain, everyday, _ := trusttest.NewDiscovery(t)
	directory := t.TempDir()
	files := trust.ChainFiles(addressdoc.RootPathPrefix, chain.Statements)
	ledgerFile := filepath.Join(t.TempDir(), "expected")
	for _, file := range files {
		expected, err := trust.ExpectedOf(file.Signed)
		if err != nil {
			t.Fatal(err)
		}
		if err = trust.AppendExpected(ledgerFile, expected); err != nil {
			t.Fatal(err)
		}
	}
	if err := trust.Publish(directory, files); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.StripPrefix(trust.WellKnownPrefix, http.FileServer(http.Dir(directory))))
	t.Cleanup(server.Close)
	watch := func() (int, string) {
		var stdout, stderr bytes.Buffer
		status := run(t.Context(), []string{"--expected", ledgerFile, "--place", server.URL}, server.Client(),
			&stdout, &stderr)
		return status, stdout.String() + stderr.String()
	}
	if status, said := watch(); status != exitExpected {
		t.Fatalf("only the owner's: %d %q; want quiet", status, said)
	}
	// The everyday key, stolen with its PIN, signs a statement the owner never made.
	thief := trusttest.HardwareKey(t, "x")
	theirs, _ := chain.Next(t, 2, trusttest.Change{AddEveryday: []trusttest.Key{thief}}, everyday)
	served := trust.ChainFiles(addressdoc.RootPathPrefix, append(chain.Statements, theirs))
	if err := trust.Publish(directory, served); err != nil {
		t.Fatal(err)
	}
	status, said := watch()
	if status != exitUnexpected || !strings.Contains(said, trust.Fingerprint(everyday.PublicKey())) ||
		!strings.Contains(said, "root/2") || !strings.Contains(said, "counter") {
		t.Errorf("a statement the owner never made: %d %q; want it reported by its key and counter", status, said)
	}
}
