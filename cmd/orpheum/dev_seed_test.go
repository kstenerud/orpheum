// SPDX-License-Identifier: GPL-3.0-or-later

//go:build dev

package main

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevSeed_CreatesTheSeedsUsersSettingsAndSecretsAndCanRunAgain(t *testing.T) {
	t.Parallel()
	config, configPath := testConfig(t)
	template, err := os.ReadFile(filepath.Join("..", "..", "dev", "seed.toml"))
	if err != nil {
		t.Fatal(err)
	}
	seedPath := filepath.Join(t.TempDir(), "seed.toml")
	rendered := strings.ReplaceAll(string(template), "{root}", t.TempDir())
	if err = os.WriteFile(seedPath, []byte(rendered), 0o600); err != nil { //nolint:gosec // G703: under t.TempDir()
		t.Fatal(err)
	}
	environment := map[string]string{
		"ORPHEUM_DEV_TMDB_KEY":               "dev-tmdb-0123456789abcd",
		"ORPHEUM_DEV_OPENSUBTITLES_PASSWORD": "dev-password",
	}
	lookupEnv := func(name string) (string, bool) {
		value, isSet := environment[name]
		return value, isSet
	}
	seed := func() (int, string) {
		var stdout, stderr bytes.Buffer
		status := run(t.Context(), []string{"dev", "seed", "--config", configPath, "--seed", seedPath}, lookupEnv,
			&stdout, &stderr)
		return status, stdout.String() + stderr.String()
	}
	status, output := seed()
	if status != exitSuccess {
		t.Fatalf("dev seed = %d:\n%s", status, output)
	}
	for _, want := range []string{
		"user owner (admin)", "user partner (user)", "sealed tmdb_api_key",
		"sealed opensubtitles_password", `library "Dev films" covers`, `library "Dev shows" covers`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("the seed's report does not say %q:\n%s", want, output)
		}
	}

	server := startServing(t, config, newCapturedLog("ready"), "ready")
	token := signInAs(t, server.baseURL, "owner", "owner")
	settings := call(t, http.MethodGet, server.baseURL+"/settings", token, nil)
	if !strings.Contains(settings.body, `"key":"update_check","kind":"bool","value":false`) {
		t.Errorf("the seeded setting is not in force: %s", settings.body)
	}
	raw := call(t, http.MethodGet, server.baseURL+"/config", token, nil)
	if !strings.Contains(raw.body, `"key":"debug_pprof","kind":"bool","value":true,"start":false,"isModified":true`) ||
		strings.Contains(settings.body, "debug_pprof") {
		t.Errorf("the seed did not turn debug_pprof on through /config, or the settings page shows it:\n%s", raw.body)
	}
	serverSecrets := call(t, http.MethodGet, server.baseURL+"/secrets", token, nil)
	mySecrets := call(t, http.MethodGet, server.baseURL+"/me/secrets", token, nil)
	const tmdbSet = `"name":"tmdb_api_key","description":"the TMDB API key metadata is fetched with","isSet":true`
	if !strings.Contains(serverSecrets.body, tmdbSet) ||
		!strings.Contains(mySecrets.body, `"name":"opensubtitles_password"`) {
		t.Errorf("the seeded secrets are not set:\n%s\n%s", serverSecrets.body, mySecrets.body)
	}
	partner := signInAs(t, server.baseURL, "partner", "partner")
	me := call(t, http.MethodGet, server.baseURL+"/me", partner, nil)
	if !strings.Contains(me.body, `"isOutsider":true`) {
		t.Errorf("partner is not an outsider: %s", me.body)
	}
	if err = server.stop(); err != nil {
		t.Fatal(err)
	}

	checkSeedsAgain(t, seed)
}

// checkSeedsAgain runs the seed a second time: what exists is left as it is.
func checkSeedsAgain(t *testing.T, seed func() (int, string)) {
	t.Helper()
	status, output := seed()
	if status != exitSuccess || !strings.Contains(output, "user owner exists") ||
		!strings.Contains(output, `library "Dev films" exists; left as it is`) {
		t.Errorf("seeding again = %d, want it to leave existing users and libraries alone:\n%s", status, output)
	}
}

func signInAs(t *testing.T, baseURL, name, password string) string {
	t.Helper()
	answer := call(t, http.MethodPost, baseURL+"/session", "", map[string]string{"name": name, "password": password})
	const marker = `"token":"`
	start := strings.Index(answer.body, marker)
	if answer.status != http.StatusCreated || start < 0 {
		t.Fatalf("sign in as %s: %d %s", name, answer.status, answer.body)
	}
	token, _, found := strings.Cut(answer.body[start+len(marker):], `"`)
	if !found {
		t.Fatalf("sign in as %s: no end to the token in %s", name, answer.body)
	}
	return token
}
