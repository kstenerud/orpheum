// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// default-config makes a core's instance, every directory private, whose full config names every
// key and leaves those with no starting value for the admin, so it refuses to start until they
// are set; and it never overwrites an instance (D242 §4).
func TestDefaultConfig_MakesAnInstanceThatWaitsForItsRequiredKeysAndIsNeverOverwritten(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "o") // a socket's path is short: t.TempDir's carries the test's name
	made := runCommand(t, "default-config", dir)
	if made.status != exitSuccess || !strings.Contains(made.stdout, "made a core instance") {
		t.Fatalf("default-config = %+v", made)
	}
	for _, name := range []string{"persistent", "cache", "backups", "run"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.IsDir() || info.Mode().Perm() != instanceDirectoryMode {
			t.Errorf("%s: %v, %v; want a directory with mode 0700", name, info, err)
		}
	}
	fullConfig := string(readFile(t, filepath.Join(dir, "persistent", fullConfigFileName)))
	for _, key := range configKeys(kindCore) {
		if !strings.Contains(fullConfig, key.name+" =") {
			t.Errorf("the full config does not name %s:\n%s", key.path(), fullConfig)
		}
	}
	refused := runCommand(t, "migrate", "--instance", dir)
	if refused.status == exitSuccess || !strings.Contains(refused.stderr, "server.listen is required") {
		t.Errorf("migrate on a new instance = %+v; want it to wait for server.listen", refused)
	}
	topLevel := readFile(t, filepath.Join(dir, instanceFileName))
	again := runCommand(t, "default-config", dir)
	if again.status == exitSuccess || !strings.Contains(again.stderr, "already holds an instance") {
		t.Errorf("default-config on an instance = %+v; want a refusal", again)
	}
	if got := readFile(t, filepath.Join(dir, instanceFileName)); string(got) != string(topLevel) {
		t.Error("default-config on an instance changed its top-level config")
	}
}

// A node's instance has no backups directory, and each kind of instance runs only its own kind
// (D242 §6).
func TestInstance_EachKindRunsOnlyItsOwnKind(t *testing.T) {
	t.Parallel()
	nodeDir := filepath.Join(t.TempDir(), "n") // a socket's path is short: t.TempDir's carries the test's name
	if made := runCommand(t, "default-config", "--node", nodeDir); made.status != exitSuccess {
		t.Fatalf("default-config --node = %+v", made)
	}
	if _, err := os.Stat(filepath.Join(nodeDir, "backups")); !os.IsNotExist(err) {
		t.Errorf("a node's instance has a backups directory: %v", err)
	}
	fullConfig := string(readFile(t, filepath.Join(nodeDir, "persistent", fullConfigFileName)))
	if !strings.Contains(fullConfig, "core_socket =") || strings.Contains(fullConfig, "[server]") {
		t.Errorf("a node's full config is not a node's:\n%s", fullConfig)
	}
	for _, subcommand := range []string{"core", "migrate", "doctor"} {
		refused := runCommand(t, subcommand, "--instance", nodeDir)
		if refused.status == exitSuccess || !strings.Contains(refused.stderr, `this is a "node" instance`) {
			t.Errorf("%s on a node's instance = %+v; want a refusal naming its kind", subcommand, refused)
		}
	}
	_, coreDir := testConfig(t)
	refused := runCommand(t, "node", "--instance", coreDir)
	if refused.status == exitSuccess || !strings.Contains(refused.stderr, `this is a "core" instance`) {
		t.Errorf("node on a core's instance = %+v; want a refusal naming its kind", refused)
	}
	notAnInstance := runCommand(t, "migrate", "--instance", t.TempDir())
	if notAnInstance.status == exitSuccess || !strings.Contains(notAnInstance.stderr, "orpheum default-config") {
		t.Errorf("migrate on a plain directory = %+v; want it to say how to make an instance", notAnInstance)
	}
}

// log.destination names a file the log is appended to (D242 §3).
func TestLogger_AppendsToTheFileLogDestinationNames(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	config.Log.Destination = filepath.Join(t.TempDir(), "orpheum.log")
	var stderr strings.Builder
	for _, line := range []string{"first", "second"} {
		logger, closeLog, err := invocation{stderr: &stderr}.logger(config)
		if err != nil {
			t.Fatal(err)
		}
		logger.Info("a line", "line", line)
		if err = closeLog(); err != nil {
			t.Fatal(err)
		}
	}
	written := string(readFile(t, config.Log.Destination))
	if !strings.Contains(written, `"line":"first"`) || !strings.Contains(written, `"line":"second"`) {
		t.Errorf("the log file holds %q; want both lines, appended", written)
	}
	if stderr.Len() != 0 {
		t.Errorf("standard error got %q; want the log in its file alone", stderr.String())
	}
}

// default-config refuses, before it makes anything, an instance whose socket's path would be too
// long for a Unix socket, and --run-dir puts the run directory somewhere shorter (D242).
func TestDefaultConfig_RefusesASocketPathTooLongAndTakesARunDirectoryElsewhere(t *testing.T) {
	t.Parallel()
	deep := filepath.Join(t.TempDir(), strings.Repeat("a-folder-whose-name-is-long/", 4), "orpheum")
	refused := runCommand(t, "default-config", deep)
	if refused.status == exitSuccess || !strings.Contains(refused.stderr, "--run-dir") {
		t.Errorf("default-config in a deep directory = %+v; want a refusal saying to use --run-dir", refused)
	}
	if _, err := os.Stat(deep); !os.IsNotExist(err) {
		t.Errorf("the refused default-config made %s: %v", deep, err)
	}
	runDir := filepath.Join(t.TempDir(), "r")
	if made := runCommand(t, "default-config", "--run-dir", runDir, deep); made.status != exitSuccess {
		t.Fatalf("default-config --run-dir = %+v", made)
	}
	instance, err := loadInstance(deep, kindCore)
	if err != nil || instance.RunDir != runDir {
		t.Errorf("the instance's run directory is %q, %v; want %s", instance.RunDir, err, runDir)
	}
}
