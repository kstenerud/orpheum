// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
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
	captured := newCapturedLog("ready")
	server := startServing(t, config, captured, "ready", nil)
	const adminName = "CONTROL-admin-name"
	setupCode := setUpOwner(t, server, configPath, adminName)
	token := signInAs(t, server.baseURL, adminName, ownerPassword)

	var responses strings.Builder
	record := func(answer apiReply) { responses.WriteString(answer.body + "\n") }
	for _, definition := range secrets.Registry() {
		if definition.IsMadeByServer {
			// No person sets it, and this server, with no relay, makes none (startRelay).
			continue
		}
		path := "/secrets/"
		value := canaryPrefix + "-" + string(definition.Name) + "-0123456789"
		answer := requestAPI(t, http.MethodPut, server.baseURL+path+string(definition.Name), token,
			map[string]string{"value": value})
		if answer.status != http.StatusNoContent {
			t.Fatalf("PUT %s%s: %d %s", path, definition.Name, answer.status, answer.body)
		}
		record(answer)
		record(requestAPI(t, http.MethodPut, server.baseURL+path+string(definition.Name), token,
			map[string]any{"value": []string{value}}))
	}
	paths := []string{
		"/health", "/me", "/config", "/secrets", "/openapi.json",
	}
	for _, path := range paths {
		record(requestAPI(t, http.MethodGet, server.baseURL+path, token, nil))
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}

	doctor := runCommand(t, "doctor", "--instance", configPath)
	written := runCommand(t, "backup", "--instance", configPath)
	backupFile := strings.Fields(written.stdout)[1]
	backupContents := archiveContents(t, backupFile)
	databaseBytes := readFile(t, filepath.Join(config.Instance.PersistentDir, "persistent.db"))
	channels := []channel{
		{"API responses", responses.String(), adminName},
		{"the log", captured.String(), setupCode},
		{"doctor's output", doctor.stdout + doctor.stderr, config.Instance.PersistentDir},
		{"a backup", backupContents, adminName},
		{"persistent.db", string(databaseBytes), adminName},
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
	status := dispatch(t.Context(), arguments, noEnvironment, nil, &stdout, &stderr)
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

// archiveContents is every file a backup archive holds, uncompressed, one after another: what a
// search of the backup's contents searches.
func archiveContents(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // read-only
	zipped, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	archive := tar.NewReader(zipped)
	var contents strings.Builder
	for {
		if _, err = archive.Next(); errors.Is(err, io.EOF) {
			return contents.String()
		} else if err != nil {
			t.Fatal(err)
		}
		// The backup the test made is a few megabytes; a gibibyte bounds a broken one.
		if _, err = io.Copy(&contents, io.LimitReader(archive, 1<<30)); err != nil {
			t.Fatal(err)
		}
	}
}
