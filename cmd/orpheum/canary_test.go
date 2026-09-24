// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/secrets"
)

// canaryPrefix starts every secret's canary; nothing but a secret contains it.
const canaryPrefix = "CANARY-7c2e91"

// channel is one place a secret could leak to, and a marker the test knows is in it, so the check
// is seen to find a planted string in every channel before it is trusted to find none (D190).
type channel struct {
	name    string
	content string
	control string
}

// TestCanary_ASecretAppearsNowhereButAsCiphertext is M1's acceptance test for D194 §5. Every
// secret is set to a canary through the real API of a real server, then the API's responses, the
// log at debug level, doctor's output, a backup and the database file itself are searched.
// Bug reports do not exist in Cut A yet; they join this test when they land.
func TestCanary_ASecretAppearsNowhereButAsCiphertext(t *testing.T) {
	t.Parallel()
	config, configPath := testConfig(t)
	captured := newCapturedLog("ready", "stopping")
	server := startServing(t, config, captured, "ready")
	setupCode := runCommand(t, "setup-code", "--config", configPath)
	const adminName = "CONTROL-admin-name"
	created := call(t, http.MethodPost, server.baseURL+"/setup", "",
		map[string]string{"setupCode": strings.TrimSpace(setupCode.stdout), "name": adminName, "password": "pw"})
	if created.status != http.StatusCreated {
		t.Fatalf("setup: %d %s", created.status, created.body)
	}
	session := call(t, http.MethodPost, server.baseURL+"/session", "",
		map[string]string{"name": adminName, "password": "pw"})
	var signedIn struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(session.body), &signedIn); err != nil || signedIn.Token == "" {
		t.Fatalf("sign in: %d %s", session.status, session.body)
	}

	var responses strings.Builder
	record := func(answer reply) { responses.WriteString(answer.body + "\n") }
	for _, definition := range secrets.Registry() {
		path := "/secrets/"
		if definition.Scope == secrets.ScopeUser {
			path = "/me/secrets/"
		}
		value := canaryPrefix + "-" + string(definition.Name) + "-0123456789"
		answer := call(t, http.MethodPut, server.baseURL+path+string(definition.Name), signedIn.Token,
			map[string]string{"value": value})
		if answer.status != http.StatusNoContent {
			t.Fatalf("PUT %s%s: %d %s", path, definition.Name, answer.status, answer.body)
		}
		record(answer)
		record(call(t, http.MethodPut, server.baseURL+path+string(definition.Name), signedIn.Token,
			map[string]any{"value": []string{value}}))
	}
	paths := []string{
		"/health", "/me", "/settings", "/me/settings", "/config", "/secrets", "/me/secrets", "/openapi.json",
	}
	for _, path := range paths {
		record(call(t, http.MethodGet, server.baseURL+path, signedIn.Token, nil))
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}

	doctor := runCommand(t, "doctor", "--config", configPath)
	written := runCommand(t, "backup", "--config", configPath)
	backupFile := strings.Fields(written.stdout)[1]
	backupBytes := readFile(t, backupFile)
	databaseBytes := readFile(t, filepath.Join(config.Server.DataDir, "library.db"))
	channels := []channel{
		{"API responses", responses.String(), adminName},
		{"the log", captured.String(), strings.TrimSpace(setupCode.stdout)},
		{"doctor's output", doctor.stdout + doctor.stderr, config.Server.DataDir},
		{"a backup", string(backupBytes), adminName},
		{"library.db", string(databaseBytes), adminName},
	}
	for _, place := range channels {
		if !strings.Contains(place.content, place.control) {
			t.Errorf("%s does not contain its control marker %q; the search there proves nothing", place.name,
				place.control)
		}
		if strings.Contains(place.content, canaryPrefix) {
			t.Errorf("a secret leaked into %s", place.name)
		}
	}
	if !strings.Contains(doctor.stdout, "every sealed secret opens") {
		t.Errorf("doctor did not check the sealed secrets:\n%s", doctor.stdout)
	}
}

// commandOutput is what a subcommand wrote and returned.
type commandOutput struct {
	status int
	stdout string
	stderr string
}

func runCommand(t *testing.T, arguments ...string) commandOutput {
	t.Helper()
	var stdout, stderr bytes.Buffer
	status := run(t.Context(), arguments, noEnvironment, &stdout, &stderr)
	return commandOutput{status: status, stdout: stdout.String(), stderr: stderr.String()}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}
