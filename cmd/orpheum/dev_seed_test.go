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
	template, err := os.ReadFile(filepath.Join("..", "..", "deploy", "dev", "seed.toml"))
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
	runSeed := func() (int, string) {
		var stdout, stderr bytes.Buffer
		arguments := []string{"dev", "seed", "--instance", configPath, "--seed", seedPath}
		status := dispatch(t.Context(), arguments, lookupEnv, &stdout, &stderr)
		return status, stdout.String() + stderr.String()
	}
	status, output := runSeed()
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

	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	token := signInAs(t, server.baseURL, "owner", "owner")
	settings := requestAPI(t, http.MethodGet, server.baseURL+"/settings", token, nil)
	if !strings.Contains(settings.body, `"key":"should_check_for_updates","kind":"bool","value":false`) {
		t.Errorf("the seeded setting is not in force: %s", settings.body)
	}
	raw := requestAPI(t, http.MethodGet, server.baseURL+"/config", token, nil)
	const pprofOn = `"key":"should_serve_debug_pprof","kind":"bool","value":true,"startingValue":false,` +
		`"isModified":true`
	if !strings.Contains(raw.body, pprofOn) ||
		strings.Contains(settings.body, "should_serve_debug_pprof") {
		t.Errorf("the seed did not turn should_serve_debug_pprof on through /config, "+
			"or the settings page shows it:\n%s", raw.body)
	}
	serverSecrets := requestAPI(t, http.MethodGet, server.baseURL+"/secrets", token, nil)
	const tmdbSet = `"name":"tmdb_api_key","description":"the TMDB API key metadata is fetched with","isSet":true`
	const loginSet = `"description":"the password of that OpenSubtitles account","isSet":true`
	if !strings.Contains(serverSecrets.body, tmdbSet) || !strings.Contains(serverSecrets.body, loginSet) {
		t.Errorf("the seeded secrets are not set:\n%s", serverSecrets.body)
	}
	partner := signInAs(t, server.baseURL, "partner", "partner")
	me := requestAPI(t, http.MethodGet, server.baseURL+"/me", partner, nil)
	if !strings.Contains(me.body, `"isOutsider":true`) {
		t.Errorf("partner is not an outsider: %s", me.body)
	}
	if err = server.stop(); err != nil {
		t.Fatal(err)
	}

	checkSeedsAgain(t, runSeed)
}

// checkSeedsAgain runs the seed a second time: what exists is left as it is.
func checkSeedsAgain(t *testing.T, runSeed func() (int, string)) {
	t.Helper()
	status, output := runSeed()
	if status != exitSuccess || !strings.Contains(output, "user owner exists") ||
		!strings.Contains(output, `library "Dev films" exists; left as it is`) {
		t.Errorf("seeding again = %d, want it to leave existing users and libraries alone:\n%s", status, output)
	}
}
