// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/discovery/release"
)

// Each release is written into the record, a security release marking the older ones; an older
// release is refused, writing nothing, and so is a call without what it needs.
func TestRun_WritesEachReleaseIntoTheRecordAndRefusesAnOlderOne(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "release.json")
	digest := "sha256:" + strings.Repeat("e", 64)
	for _, arguments := range [][]string{
		{"--record", path, "--version", "v0.3.1", "--digest", digest},
		{"--record", path, "--version", "v0.3.2", "--digest", digest, "--security", "a sign-in could be guessed"},
	} {
		var stdout, stderr bytes.Buffer
		if status := run(arguments, &stdout, &stderr); status != exitSuccess {
			t.Fatalf("run(%v) = %d, %s", arguments, status, stderr.String())
		}
	}
	record, err := release.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	want := release.Note{Security: true, Summary: "a sign-in could be guessed", FixedIn: "v0.3.2"}
	if record.Latest != "v0.3.2" || record.Digest != digest || record.Versions["v0.3.1"] != want {
		t.Errorf("the record is %+v; want v0.3.2 latest and v0.3.1 needing its fix", record)
	}
	var stdout, stderr bytes.Buffer
	if status := run([]string{"--record", path, "--version", "v0.3.0", "--digest", digest}, &stdout,
		&stderr); status != exitFailure || !strings.Contains(stderr.String(), "older") {
		t.Errorf("an older release: status %d, %q; want refused saying it is older", status, stderr.String())
	}
	if again, readErr := release.Read(path); readErr != nil || again.Latest != "v0.3.2" {
		t.Errorf("a refused release left the record %+v, %v; want it as it was", again, readErr)
	}
	if status := run([]string{"--record", path}, &stdout, &stderr); status != exitUsage {
		t.Errorf("without a version: status %d; want a usage error", status)
	}
}
