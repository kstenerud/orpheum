// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/kstenerud/orpheum/internal/deployment"
)

// fillValues fills in the values file a deployment began with.
func fillValues(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, deployment.ValuesFile)
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	filled := strings.NewReplacer(
		`contact = ""`, `contact = "owner@example.org"`,
		`endpoint = ""`, `endpoint = "[2001:db8::7]:51820"`,
		`public_ipv4 = ""`, `public_ipv4 = "203.0.113.7"`,
		`public_key = ""`, `public_key = "YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE="`,
		`roots = []`, `roots = ["/srv/media"]`,
	).Replace(string(text))
	if err = os.WriteFile(path, []byte(filled), 0o600); err != nil { //nolint:gosec // G703: a test's own temporary file
		t.Fatal(err)
	}
}

func TestWriteDeployment_BeginsThenMakesTheInstancesOnceAndWritesTheFiles(t *testing.T) {
	t.Parallel()
	dir, runDir := t.TempDir(), filepath.Join(t.TempDir(), "run")
	begun, err := writeDeployment(dir, runDir, deployment.Staging, "orpheum:v0.1.0", "nas", -1)
	if err != nil {
		t.Fatal(err)
	}
	if !begun.isBegun || !strings.Contains(begun.text, "public key") {
		t.Errorf("the first run said %q; want it begun, giving the tunnel's public key", begun.text)
	}
	if _, err = writeDeployment(dir, runDir, deployment.Staging, "orpheum:v0.1.0", "nas", -1); err == nil ||
		!strings.Contains(err.Error(), "contact") {
		t.Errorf("writing before the values are filled in = %v; want it refused, naming what they need", err)
	}
	fillValues(t, dir)
	written, err := writeDeployment(dir, runDir, deployment.Release, "orpheum:v0.1.0", "nas", -1)
	if err != nil {
		t.Fatal(err)
	}
	if written.isBegun || !strings.Contains(written.text, "docker compose up -d") {
		t.Errorf("the run after filling in said %q; want it written, saying how to start it", written.text)
	}
	core, err := loadInstance(filepath.Join(dir, "core"), kindCore)
	if err != nil {
		t.Fatal(err)
	}
	if core.RunDir != filepath.Join(runDir, "core") {
		t.Errorf("the core's run directory is %s, want %s, where the compose file mounts run/", core.RunDir,
			filepath.Join(runDir, "core"))
	}
	config := map[string]map[string]any{}
	if _, err = toml.DecodeFile(core.fullConfigPath(), &config); err != nil {
		t.Fatal(err)
	}
	if config["database"]["when_unmigratable"] != "start_over" {
		t.Errorf("the core config says %v when unmigratable; want staging's start_over, as the values file began",
			config["database"]["when_unmigratable"])
	}
	if _, err = writeDeployment(dir, runDir, deployment.Release, "orpheum:v0.2.0", "nas", -1); err != nil {
		t.Errorf("writing again over the instances made = %v; want them kept and the files written", err)
	}
}
