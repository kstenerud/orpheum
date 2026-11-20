// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// coreHealthCheck is the core's health check in a published compose file: its bash script, as YAML
// quotes it, before the port it is given as $0.
var coreHealthCheck = regexp.MustCompile(`(?m)^\s+test: \["CMD", "bash", "-c", ("(?:[^"\\]|\\.)*"), `)

// healthCheckScript is the core's health check script in the compose file at name, as compose hands
// it to bash: unquoted, and each $$ a $.
func healthCheckScript(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	match := coreHealthCheck.FindSubmatch(raw)
	if match == nil {
		t.Fatalf("%s has no bash health check for the core", name)
	}
	script, err := strconv.Unquote(string(match[1]))
	if err != nil {
		t.Fatalf("%s's health check %s: %v", name, match[1], err)
	}
	return strings.ReplaceAll(script, "$$", "$")
}

// runHealthCheck runs script with bash, as the container does, its $0 the port of server, and
// returns what it printed, which Docker keeps, and how it exited.
func runHealthCheck(t *testing.T, script string, server runningServer) (string, error) {
	t.Helper()
	address, err := url.Parse(server.baseURL)
	if err != nil {
		t.Fatal(err)
	}
	output, err := testutil.RunShell(t, `exec bash -c "$1" "$2"`, []string{script, address.Port()},
		[]string{"PATH=/usr/local/bin:/usr/bin:/bin"})
	return string(output), err
}

// The core's health check prints what /api/v1/health answers, so `docker inspect` and a NAS's panel
// can show why the core is unhealthy, not the word alone; and it passes only while the server answers
// 200 (the walkthrough install on OpenMediaVault 7, 2026-11-20, W4).
func TestHouseholdHealthCheck_PrintsTheHealthEndpointsAnswerAndPassesOnlyOn200(t *testing.T) {
	t.Parallel()
	script := healthCheckScript(t, "../../deploy/docker/compose.yaml")

	config, _ := testConfig(t)
	if err := os.MkdirAll(config.Instance.PersistentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(config.Instance.PersistentDir, "persistent.db")
	if err := os.WriteFile(foreign, []byte("SQLite format 3\x00"+strings.Repeat("\x00", 84)), 0o600); err != nil {
		t.Fatal(err)
	}
	degraded := startServing(t, config, newCapturedLog(couldNotStart), couldNotStart, nil)
	output, err := runHealthCheck(t, script, degraded)
	if err == nil {
		t.Errorf("the health check passed a server that could not start: %s", output)
	}
	for _, says := range []string{`"status":"degraded"`, "the server could not start", "file is not a database"} {
		if !strings.Contains(output, says) {
			t.Errorf("the health check of a server that could not start printed %q; want it to say %q", output, says)
		}
	}
	if stopErr := degraded.stop(); stopErr == nil {
		t.Error("serve returned nil after a failed start")
	}

	readyConfig, _ := testConfig(t)
	ready := startServing(t, readyConfig, newCapturedLog(eventReady), eventReady, nil)
	output, err = runHealthCheck(t, script, ready)
	if err != nil || !strings.Contains(output, `"status":"ready"`) {
		t.Errorf("the health check of a ready server: %v, %q; want it passed, saying ready", err, output)
	}
	if stopErr := ready.stop(); stopErr != nil {
		t.Fatal(stopErr)
	}
}
