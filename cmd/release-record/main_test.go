// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/release"
)

// Each release is written into the record with its digest and where it may be pulled from, dated
// when it was written, a security release marking the older ones; an older release is refused,
// writing nothing, and so is a call without what it needs. Each written record says how to sign it
// (D391 §6).
func TestRun_WritesEachReleaseIntoTheRecordAndRefusesAnOlderOne(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "release.json")
	digest := "sha256:" + strings.Repeat("e", 64)
	later := "sha256:" + strings.Repeat("f", 64)
	now := time.UnixMilli(1_800_000_000_000)
	for _, arguments := range [][]string{
		{"--record", path, "--version", "v0.3.1", "--digest", digest, "--location", "registry.example/orpheum"},
		{"--record", path, "--version", "v0.3.2", "--digest", later, "--security", "a sign-in could be guessed"},
	} {
		var stdout, stderr bytes.Buffer
		if status := run(arguments, now, &stdout, &stderr); status != exitSuccess {
			t.Fatalf("run(%v) = %d, %s", arguments, status, stderr.String())
		}
		if !strings.Contains(stdout.String(), "ssh-keygen -Y sign -n orpheum-release-record-v1 -f <key> "+path) {
			t.Errorf("run said %q; want the command that signs the record", stdout.String())
		}
	}
	record, err := release.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	want := release.Note{Digest: digest, Security: true, Summary: "a sign-in could be guessed", FixedIn: "v0.3.2"}
	if record.Latest != "v0.3.2" || record.Releases["v0.3.2"].Digest != later || record.Releases["v0.3.1"] != want ||
		record.IssuedAt != now.UnixMilli() || len(record.Locations) != 1 {
		t.Errorf("the record is %+v; want v0.3.2 latest, v0.3.1 needing its fix, its location kept", record)
	}
	var stdout, stderr bytes.Buffer
	if status := run([]string{"--record", path, "--version", "v0.3.0", "--digest", digest}, now, &stdout,
		&stderr); status != exitFailure || !strings.Contains(stderr.String(), "older") {
		t.Errorf("an older release: status %d, %q; want refused saying it is older", status, stderr.String())
	}
	if again, readErr := release.Read(path); readErr != nil || again.Latest != "v0.3.2" {
		t.Errorf("a refused release left the record %+v, %v; want it as it was", again, readErr)
	}
	if status := run([]string{"--record", path}, now, &stdout, &stderr); status != exitUsage {
		t.Errorf("without a version: status %d; want a usage error", status)
	}
}
