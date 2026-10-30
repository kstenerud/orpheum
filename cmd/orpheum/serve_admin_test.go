// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

func TestServe_ServesPProfOnlyWhenTheSettingSaysSo(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	pprofURL := strings.TrimSuffix(server.baseURL, api.Prefix) + "/debug/pprof/"
	if off := requestAPI(t, http.MethodGet, pprofURL, "", nil); off.status != http.StatusNotFound {
		t.Errorf("with should_serve_debug_pprof at its starting value, /debug/pprof/ = %d, want 404", off.status)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
	storetest.ExecRaw(t, filepath.Join(config.Instance.PersistentDir, "persistent.db"),
		"UPDATE setting SET value_json = 'true', is_modified = 1 WHERE key = 'should_serve_debug_pprof'")
	restarted := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	pprofURL = strings.TrimSuffix(restarted.baseURL, api.Prefix) + "/debug/pprof/"
	if on := requestAPI(t, http.MethodGet, pprofURL, "", nil); on.status != http.StatusOK {
		t.Errorf("with should_serve_debug_pprof on and a loopback listener, /debug/pprof/ = %d, want 200", on.status)
	}
}

func TestServe_SweepsUnmodifiedSettingsToTheirStartingValuesBeforeItIsReady(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	first := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	if err := first.stop(); err != nil {
		t.Fatal(err)
	}
	// As a database looks after an upgrade that changed two starting values: one row nobody
	// touched still holds the old value; the other was set by a person.
	storetest.ExecRaw(t, filepath.Join(config.Instance.PersistentDir, "persistent.db"),
		"UPDATE setting SET value_json = '7', is_modified = 0 WHERE key = 'signin_failure_limit'",
		"UPDATE setting SET value_json = '6', is_modified = 1 WHERE key = 'secret_last_chars_shown'")
	captured := newCapturedLog("ready")
	server := startServing(t, config, captured, "ready", nil)
	logged := captured.String()
	sweep := strings.Index(logged, "this release changed the starting value of settings nobody had set")
	ready := strings.Index(logged, `"logEvent":"ready"`)
	if sweep < 0 || ready < 0 || sweep > ready || !strings.Contains(logged[sweep:ready], "signin_failure_limit") ||
		strings.Contains(logged[sweep:ready], "secret_last_chars_shown") {
		t.Errorf("the sweep was not logged before ready, or touched the setting a person set:\n%s", logged)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
	doctor := runCommand(t, "doctor", "--instance", path)
	if strings.Contains(doctor.stdout, "is unused") || strings.Contains(doctor.stderr, "had no row") {
		t.Errorf("after the start swept, doctor found work to do:\n%s%s", doctor.stdout, doctor.stderr)
	}
}

func TestServe_DiscardsACacheFromAnotherVersionAndBuildsItAgain(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	first := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	if err := first.stop(); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(config.Instance.CacheDir, "cache.db")
	storetest.ExecRaw(t, cachePath, "PRAGMA user_version = 999",
		"INSERT INTO job (kind, runnable_at, created_at) VALUES ('fetch_metadata', 0, 0)")
	captured := newCapturedLog("ready")
	server := startServing(t, config, captured, "ready", nil)
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(captured.String(), "discarding cache.db") {
		t.Errorf("no word of discarding the old cache:\n%s", captured)
	}
	// ExecRaw's connection to cache.db, cached from the insert above, must not survive the discard
	// and rebuild: a write through it now must land in the file the server just reopened, not an
	// orphaned connection to the one it discarded (finding 5, 2026-10-26).
	storetest.ExecRaw(t, cachePath,
		"INSERT INTO job (kind, runnable_at, created_at) VALUES ('backup', 0, 2)")
	cache, err := store.OpenCache(t.Context(), cachePath, storetest.OpenOptions())
	if err != nil {
		t.Fatalf("the rebuilt cache does not open: %v", err)
	}
	defer func() {
		if closeErr := cache.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	// The rebuilt cache may hold what the start's sweeps queued; never the old file's job, the one
	// created at 0.
	jobs, err := cache.Read().ListJobs(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	sawProbe := false
	for _, job := range jobs {
		if job.CreatedAt == 0 {
			t.Errorf("the rebuilt cache holds the old file's %s job; it should be gone", job.Kind)
		}
		if job.CreatedAt == 2 {
			sawProbe = true
		}
	}
	if !sawProbe {
		t.Error("ExecRaw's insert after the discard did not reach the rebuilt cache.db; its connection was stale")
	}
}

func TestServe_TheSystemPageShowsTheServersVersionDirectoriesAndDatabases(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	token := signedInOwner(t, server, path)
	system := requestAPI(t, http.MethodGet, server.baseURL+"/system", token, nil)
	var page struct {
		Server    api.ServerBody     `json:"server"`
		Providers []api.ProviderBody `json:"providers"`
	}
	if err := json.Unmarshal([]byte(system.body), &page); err != nil {
		t.Fatalf("/system: %d %s", system.status, system.body)
	}
	names := make([]string, 0, len(page.Providers))
	for _, provider := range page.Providers {
		names = append(names, provider.Name)
	}
	if want := []string{"TMDB", "TMDB's image server", "OpenSubtitles"}; !slices.Equal(names, want) {
		t.Errorf("the System page's providers are %q, want each outside host's: %q (D231 §5)", names, want)
	}
	facts := page.Server
	if facts.Build.Name != version || facts.Host.Platform == "" || len(facts.Disks) != 2 ||
		facts.Disks[0].Path != config.Instance.PersistentDir ||
		facts.Disks[1].Path != config.Instance.CacheDir || facts.Disks[0].TotalBytes <= 0 ||
		len(facts.Databases) != 2 || facts.Databases[0].SizeBytes <= 0 || facts.StartedAt <= 0 {
		t.Errorf("the System page's server = %+v; want this build, its data and cache disks, and both databases "+
			"(D225)", facts)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
}

// A server with no discovery says so, and that it asks its router for nothing: the System page's
// Discovery section is wired to the discovery and the port mapping (D273 §7).
func TestServe_TheDiscoverySectionSaysNoDiscoveryAndNoPortMapping(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	token := signedInOwner(t, server, path)
	answered := requestAPI(t, http.MethodGet, server.baseURL+"/system/discovery", token, nil)
	var standing api.DiscoveryStatusBody
	if err := json.Unmarshal([]byte(answered.body), &standing); err != nil || answered.status != http.StatusOK {
		t.Fatalf("/system/discovery: %d %s", answered.status, answered.body)
	}
	if standing.BaseURL != "" || standing.PortMapping.IsWanted || standing.PortMapping.Mappings == nil {
		t.Errorf("/system/discovery = %+v; want no discovery and no port mapping asked for", standing)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
}

// The served server keeps each link's secret sealed, so a person who registered by an invitation is
// shown it again as their own link (discovery-core.md K7).
func TestServe_APersonIsShownTheLinkTheyRegisteredBy(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	token := signedInOwner(t, server, path)
	invited := requestAPI(t, http.MethodPost, server.baseURL+"/invitations", token,
		map[string]string{"role": "user", "adminNote": "the partner"})
	var invitation api.InviteOutput
	err := json.Unmarshal([]byte(invited.body), &invitation.Body)
	if err != nil || invited.status != http.StatusCreated {
		t.Fatalf("POST /invitations: %d %s", invited.status, invited.body)
	}
	registered := requestAPI(t, http.MethodPost, server.baseURL+"/registration", "", map[string]any{
		"secret": invitation.Body.Link.Secret, "name": "partner", "password": "partner-password",
		"device": signInDevice,
	})
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(registered.body), &session); err != nil || session.Token == "" {
		t.Fatalf("POST /registration: %d %s", registered.status, registered.body)
	}
	own := requestAPI(t, http.MethodGet, server.baseURL+"/me/own-link", session.Token, nil)
	if own.status != http.StatusOK || !strings.Contains(own.body, invitation.Body.Link.Secret) {
		t.Errorf("GET /me/own-link: %d %s; want the link the partner registered by", own.status, own.body)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
}

// The served plain listener refuses a request naming a public host it was not given, as a page that
// rebound its own name to the core would send, and answers its address (S15).
func TestServe_ThePlainListenerRefusesANameItWasNotGiven(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.baseURL+"/health", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "rebind.attacker.example"
	answer, err := http.DefaultClient.Do(request) //nolint:forbidigo // a test's own request to its loopback server
	if err != nil {
		t.Fatal(err)
	}
	if err = answer.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if answer.StatusCode != http.StatusMisdirectedRequest {
		t.Errorf("a request naming another host answered %d; want 421", answer.StatusCode)
	}
	if health := requestAPI(t, http.MethodGet, server.baseURL+"/health", "", nil); health.status != http.StatusOK {
		t.Errorf("a request to the listener's address answered %d", health.status)
	}
	if err = server.stop(); err != nil {
		t.Fatal(err)
	}
}

// A core whose config names a UDP port for WebRTC takes WebRTC there at the house's addresses, which
// its LAN addresses name (D300 §1).
func TestServe_TakesWebRTCAtHomeOnTheConfiguredPort(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	held, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		t.Fatal(err)
	}
	address, isUDP := held.LocalAddr().(*net.UDPAddr)
	if !isUDP {
		t.Fatalf("a UDP listener's address is %T", held.LocalAddr())
	}
	if err = held.Close(); err != nil {
		t.Fatal(err)
	}
	config.WebRTC.UDPPort = address.Port
	config.Discovery.LANAddresses = []string{"192.168.1.20:8443"}
	captured := newCapturedLog("taking WebRTC at home")
	startServing(t, config, captured, "taking WebRTC at home", nil)
	if want := fmt.Sprintf(`"udpPort":%d`, address.Port); !strings.Contains(captured.String(), want) ||
		!strings.Contains(captured.String(), "192.168.1.20") {
		t.Errorf("the log does not say WebRTC is taken on %s at 192.168.1.20:\n%s", want, captured)
	}
}
