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

// Each release is written into the record with its digest and where it may be pulled from, dated and
// its sequence one more, a security release marking the older ones; an older release, and a version
// given another digest than it was signed with, are refused, writing nothing, as is a call without
// what it needs. Each written record says how to sign it (D393 §2).
func TestRun_WritesEachReleaseIntoTheRecordAndRefusesAnOlderOne(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "release")
	digest := "sha256:" + strings.Repeat("e", 64)
	later := "sha256:" + strings.Repeat("f", 64)
	now := time.UnixMilli(1_800_000_000_000)
	for _, arguments := range [][]string{
		{
			"--record", path, "--version", "v0.3.1", "--digest", digest, "--id", "op1-publisher", "--root-version", "1",
			"--pull", "registry.example/orpheum",
		},
		{"--record", path, "--version", "v0.3.2", "--digest", later, "--security", "a sign-in could be guessed"},
	} {
		var stdout, stderr bytes.Buffer
		if status := run(arguments, now, &stdout, &stderr); status != exitSuccess {
			t.Fatalf("run(%v) = %d, %s", arguments, status, stderr.String())
		}
		if !strings.Contains(stdout.String(), "ssh-keygen -Y sign -n orpheum-release-v1 -f <the key's stub> "+path) {
			t.Errorf("run said %q; want the command that signs the record", stdout.String())
		}
	}
	record, err := release.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if record.Latest != "v0.3.2" || record.Images["v0.3.2"] != later || record.Pending["v0.3.1"].FixedIn != "v0.3.2" ||
		record.Sequence != 2 || record.ID != "op1-publisher" || !record.Issued.Equal(now.UTC().Truncate(time.Second)) ||
		len(record.Pulls) != 1 {
		t.Errorf("the record is %+v; want v0.3.2 latest, v0.3.1 needing its fix, its place kept", record)
	}
	var stdout, stderr bytes.Buffer
	if status := run([]string{"--record", path}, now, &stdout, &stderr); status != exitUsage {
		t.Errorf("without a version: status %d; want a usage error", status)
	}
}

// A release older than the latest, and a version given another digest than it was released with,
// are refused, writing nothing (the root key audit's F11).
func TestRun_RefusesAnOlderReleaseOrAnotherDigestForAVersion(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "release")
	digest := "sha256:" + strings.Repeat("e", 64)
	now := time.UnixMilli(1_800_000_000_000)
	var stdout, stderr bytes.Buffer
	if status := run([]string{
		"--record", path, "--version", "v0.3.2", "--digest", "sha256:" + strings.Repeat("f", 64), "--id", "op1-x",
		"--root-version", "1",
	}, now, &stdout, &stderr); status != exitSuccess {
		t.Fatalf("the first release: %d %s", status, stderr.String())
	}
	for _, refused := range [][]string{
		{"--record", path, "--version", "v0.3.0", "--digest", digest},
		{"--record", path, "--version", "v0.3.2", "--digest", digest},
	} {
		var stdout, stderr bytes.Buffer
		if status := run(refused, now, &stdout, &stderr); status != exitFailure {
			t.Errorf("%v: status %d, %q; want refused", refused, status, stderr.String())
		}
	}
	if again, readErr := release.Read(path); readErr != nil || again.Latest != "v0.3.2" || again.Sequence != 1 {
		t.Errorf("a refused release left the record %+v, %v; want it as it was", again, readErr)
	}
}
