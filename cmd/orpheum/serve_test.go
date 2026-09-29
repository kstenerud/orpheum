// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
)

// unreachableURL is where tests that fake no provider (TMDB, OpenSubtitles) send it: a closed port
// on loopback, so a call that escapes fails on this machine and never reaches the network
// (standards/testing.md).
const unreachableURL = "http://127.0.0.1:9"

func TestMain(m *testing.M) {
	// The node under test starts FFmpeg through this binary, as the real one does through orpheum.
	if status, isShim := shimIfAsked(os.Args, os.Stderr); isShim {
		os.Exit(status)
	}
	api.UseErrorShape()
	m.Run()
}

// signallingHandler writes JSON records to a buffer and closes a channel when a record with a
// given message is logged, so a test waits for "ready" instead of sleeping.
type signallingHandler struct {
	slog.Handler
	mutex   *sync.Mutex
	buffer  *bytes.Buffer
	signals map[string]chan struct{}
}

func (handler signallingHandler) Handle(ctx context.Context, record slog.Record) error {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()
	if signal, isAwaited := handler.signals[record.Message]; isAwaited {
		close(signal)
		delete(handler.signals, record.Message)
	}
	return handler.Handler.Handle(ctx, record)
}

func (handler signallingHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	handler.Handler = handler.Handler.WithAttrs(attributes)
	return handler
}

// capturedLog is a debug-level JSON log in memory that can be waited on.
type capturedLog struct {
	mutex   *sync.Mutex
	buffer  *bytes.Buffer
	signals map[string]chan struct{}
	logger  *slog.Logger
}

func newCapturedLog(awaited ...string) capturedLog {
	captured := capturedLog{mutex: &sync.Mutex{}, buffer: &bytes.Buffer{}, signals: map[string]chan struct{}{}}
	for _, message := range awaited {
		captured.signals[message] = make(chan struct{})
	}
	waiting := maps.Clone(captured.signals)
	handler := signallingHandler{
		Handler: slog.NewJSONHandler(captured.buffer, &slog.HandlerOptions{Level: slog.LevelDebug}),
		mutex:   captured.mutex, buffer: captured.buffer, signals: waiting,
	}
	captured.logger = slog.New(handler)
	return captured
}

func (captured capturedLog) String() string {
	captured.mutex.Lock()
	defer captured.mutex.Unlock()
	return captured.buffer.String()
}

// testConfig is a config with its directories in a temporary directory, written to a file too.
func testConfig(t *testing.T) (Config, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "orpheum.toml")
	text := fmt.Sprintf(`[server]
listen = "127.0.0.1:0"
data_dir = %q
cache_dir = %q
read_header_timeout_s = 10
shutdown_grace_s = 10
socket_dir = %q

[database]
read_connections_minimum = 4
migrate_free_space_db_multiple = 3

[node]
name = "local"
root_paths = []
ffmpeg_dir = %q
confinement_policy = "report"
video_extensions = ["mkv"]
subtitle_extensions = ["srt"]
max_errors_per_root = 1000
probe_timeout_s = 60
status_interval_s = 10
prepared_dir = %q
prepared_budget_gib = 1
prepared_kept_after_play_h = 72
prepared_disk_used_limit_percent = 95

[log]
level = "debug"
format = "json"
`, filepath.Join(root, "state"), filepath.Join(root, "cache"), filepath.Join(root, "r"), filepath.Join(root, "ffmpeg"),
		filepath.Join(root, "prepared"))
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	config, report, err := loadConfig(path)
	if err != nil || len(report.Notes()) != 0 {
		t.Fatalf("the test config does not load cleanly: %v, %v", err, report.Notes())
	}
	return config, path
}

// runningServer is a server started by serve on a loopback listener.
type runningServer struct {
	baseURL string
	stop    func() error
}

// serveWaitLimit is how long startServing waits for the log line it awaits before failing with
// the log so far, rather than hanging until the test binary's timeout says nothing useful.
const serveWaitLimit = 5 * time.Minute

// startServing runs serve until the test stops it, and waits until it logs awaited. When startNode
// is not nil, the core starts its node with it: inProcessStarter runs one in this process.
func startServing(
	t *testing.T, config Config, captured capturedLog, awaited string, startNode nodes.Starter,
) runningServer {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		outside := wiring{
			startNode: startNode, tmdbBaseURL: unreachableURL, tmdbImagesBaseURL: unreachableURL,
			openSubtitlesBaseURL: unreachableURL,
		}
		done <- serve(ctx, config, ConfigReport{}, listener, outside, captured.logger)
	}()
	select {
	case <-captured.signals[awaited]:
	case err := <-done:
		cancel()
		t.Fatalf("serve returned before logging %q: %v\n%s", awaited, err, captured)
	case <-time.After(serveWaitLimit):
		cancel()
		t.Fatalf("serve did not log %q within %v:\n%s", awaited, serveWaitLimit, captured)
	}
	stopped := false
	stop := func() error {
		if stopped {
			return nil
		}
		stopped = true
		cancel()
		return <-done
	}
	t.Cleanup(func() {
		if err := stop(); err != nil {
			t.Logf("serve returned: %v", err)
		}
	})
	return runningServer{baseURL: "http://" + listener.Addr().String() + api.Prefix, stop: stop}
}

type apiReply struct {
	status int
	body   string
}

func requestAPI(t *testing.T, method, url, token string, body any) apiReply {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(t.Context(), method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client, err := outbound.NewClient(30*time.Second, version)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := answer.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	text, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatal(err)
	}
	return apiReply{status: answer.StatusCode, body: string(text)}
}

// signInDevice is the device the tests sign in on (D217 §8).
var signInDevice = api.NewDeviceBody{Name: "Firefox on Linux", Platform: "web"}

// ownerPassword is the password setUpOwner gives the admin it makes.
const ownerPassword = "pw"

// setUpOwner completes setup with the code the setup-code command prints, making name the admin
// with ownerPassword, and returns the code.
func setUpOwner(t *testing.T, server runningServer, configPath, name string) string {
	t.Helper()
	code := runCommand(t, "setup-code", "--config", configPath)
	setupCode := strings.TrimSpace(code.stdout)
	created := requestAPI(t, http.MethodPost, server.baseURL+"/setup", "",
		map[string]string{"setupCode": setupCode, "name": name, "password": ownerPassword})
	if created.status != http.StatusCreated {
		t.Fatalf("setup: %d %s", created.status, created.body)
	}
	return setupCode
}

// signInAs signs name in on signInDevice and returns the session's token.
func signInAs(t *testing.T, baseURL, name, password string) string {
	t.Helper()
	answer := requestAPI(t, http.MethodPost, baseURL+"/session", "", map[string]any{
		"name": name, "password": password, "device": signInDevice,
	})
	var session api.SessionOutput
	if err := json.Unmarshal([]byte(answer.body), &session.Body); err != nil ||
		answer.status != http.StatusCreated || session.Body.Token == "" {
		t.Fatalf("sign in as %s: %d %s", name, answer.status, answer.body)
	}
	return session.Body.Token
}

// signedInOwner sets up the server with "owner" as its admin and returns the owner's session token.
func signedInOwner(t *testing.T, server runningServer, configPath string) string {
	t.Helper()
	setUpOwner(t, server, configPath, "owner")
	return signInAs(t, server.baseURL, "owner", ownerPassword)
}

func TestServe_HealthSaysMigratingBeforeTheMigrationStarts(t *testing.T) {
	t.Parallel()
	server := api.NewServer(slog.New(slog.DiscardHandler))
	served := httptest.NewServer(server.Handler())
	t.Cleanup(served.Close)
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), storetest.OpenOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := database.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	report := reportMigration(server.SetStatus, slog.New(slog.DiscardHandler))
	var seen []string
	err = database.Migrate(t.Context(), store.MigrateOptions{
		SnapshotDirectory:         t.TempDir(),
		FreeSpaceDatabaseMultiple: 3,
		OnBeforeStep: func(step, total int) {
			report(step, total)
			health := requestAPI(t, http.MethodGet, served.URL+api.Prefix+"/health", "", nil)
			state, stateErr := database.State(t.Context())
			if stateErr != nil {
				t.Error(stateErr)
			}
			seen = append(seen, fmt.Sprintf("%d %s step %d at schema %d",
				health.status, health.body, step, state.Version))
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("no step ran; the test saw nothing")
	}
	for index, observed := range seen {
		step := index + 1
		if !strings.HasPrefix(observed, "200 ") || !strings.Contains(observed, `"status":"migrating"`) ||
			!strings.Contains(observed, fmt.Sprintf(`"detail":"step %d of`, step)) ||
			!strings.HasSuffix(observed, fmt.Sprintf("at schema %d", step-1)) {
			t.Errorf("before step %d, /health = %s; want 200, migrating, that step, and the schema not yet at it",
				step, observed)
		}
	}
}

func TestServe_ComesUpReadyAndStopsCleanly(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	captured := newCapturedLog("ready")
	server := startServing(t, config, captured, "ready", nil)
	health := requestAPI(t, http.MethodGet, server.baseURL+"/health", "", nil)
	if health.status != http.StatusOK || !strings.Contains(health.body, `"status":"ready"`) {
		t.Errorf("/health once ready = %d %s", health.status, health.body)
	}
	if !strings.Contains(captured.String(), `"setupCode"`) {
		t.Errorf("a server with no users logs no setup code:\n%s", captured)
	}
	if err := server.stop(); err != nil {
		t.Errorf("serve returned %v after a clean stop, want nil", err)
	}
}

func TestServe_AnswersTheClientsAddressesAtTheRoot(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	root := strings.TrimSuffix(server.baseURL, api.Prefix)
	// A test binary has no web client built in, so the root says so rather than 404ing silently.
	films := requestAPI(t, http.MethodGet, root+"/films", "", nil)
	if films.status != http.StatusNotFound || !strings.Contains(films.body, "built without the web client") {
		t.Errorf("/films = %d %q, want the web client's handler", films.status, films.body)
	}
	if health := requestAPI(t, http.MethodGet, server.baseURL+"/health", "", nil); health.status != http.StatusOK {
		t.Errorf("/health beside the web client = %d, want 200", health.status)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
}

func TestServe_ReportsAFailedStartInsteadOfExiting(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	if err := os.MkdirAll(config.Server.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(config.Server.DataDir, "library.db")
	if err := os.WriteFile(foreign, []byte("SQLite format 3\x00"+strings.Repeat("\x00", 84)), 0o600); err != nil {
		t.Fatal(err)
	}
	const failed = "the server could not start; it stays up to report why at /api/v1/health"
	server := startServing(t, config, newCapturedLog(failed), failed, nil)
	health := requestAPI(t, http.MethodGet, server.baseURL+"/health", "", nil)
	if health.status != http.StatusServiceUnavailable || !strings.Contains(health.body, `"status":"degraded"`) {
		t.Errorf("/health after a failed start = %d %s, want 503 degraded with the reason", health.status, health.body)
	}
	if err := server.stop(); err == nil {
		t.Error("serve returned nil after a failed start; the exit status must say it failed")
	}
}

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
	storetest.ExecRaw(t, filepath.Join(config.Server.DataDir, "library.db"),
		"UPDATE setting SET value_json = 'true', is_modified = 1 "+
			"WHERE scope = 'server' AND key = 'should_serve_debug_pprof'")
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
	storetest.ExecRaw(t, filepath.Join(config.Server.DataDir, "library.db"),
		"UPDATE setting SET value_json = '7', is_modified = 0 WHERE key = 'signin_failure_limit'",
		"UPDATE setting SET value_json = '6', is_modified = 1 WHERE key = 'secret_last_chars_shown'")
	captured := newCapturedLog("ready")
	server := startServing(t, config, captured, "ready", nil)
	logged := captured.String()
	sweep := strings.Index(logged, "this release changed the starting value of settings nobody had set")
	ready := strings.Index(logged, `"msg":"ready"`)
	if sweep < 0 || ready < 0 || sweep > ready || !strings.Contains(logged[sweep:ready], "signin_failure_limit") ||
		strings.Contains(logged[sweep:ready], "secret_last_chars_shown") {
		t.Errorf("the sweep was not logged before ready, or touched the setting a person set:\n%s", logged)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
	doctor := runCommand(t, "doctor", "--config", path)
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
	cachePath := filepath.Join(config.Server.CacheDir, "cache.db")
	storetest.ExecRaw(t, cachePath, "PRAGMA user_version = 999",
		"INSERT INTO job (kind, runnable_at, created_at) VALUES ('fetch', 0, 0)")
	captured := newCapturedLog("ready")
	server := startServing(t, config, captured, "ready", nil)
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(captured.String(), "discarding cache.db") {
		t.Errorf("no word of discarding the old cache:\n%s", captured)
	}
	cache, err := store.OpenCache(t.Context(), cachePath, storetest.OpenOptions())
	if err != nil {
		t.Fatalf("the rebuilt cache does not open: %v", err)
	}
	defer func() {
		if err := cache.Close(); err != nil {
			t.Error(err)
		}
	}()
	if jobs, err := cache.Read().ListJobs(t.Context(), 10); err != nil || len(jobs) != 0 {
		t.Errorf("the rebuilt cache holds %d jobs (%v); the old file's job should be gone", len(jobs), err)
	}
}

func TestServe_TheSystemPageShowsTheServersVersionDirectoriesAndDatabases(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	token := signedInOwner(t, server, path)
	system := requestAPI(t, http.MethodGet, server.baseURL+"/system", token, nil)
	var page struct {
		Server api.ServerBody `json:"server"`
	}
	if err := json.Unmarshal([]byte(system.body), &page); err != nil {
		t.Fatalf("/system: %d %s", system.status, system.body)
	}
	facts := page.Server
	if facts.Build.Name != version || facts.Host.Platform == "" || len(facts.Disks) != 2 ||
		facts.Disks[0].Path != config.Server.DataDir ||
		facts.Disks[1].Path != config.Server.CacheDir || facts.Disks[0].TotalBytes <= 0 ||
		len(facts.Databases) != 2 || facts.Databases[0].SizeBytes <= 0 || facts.StartedAt <= 0 {
		t.Errorf("the System page's server = %+v; want this build, its data and cache disks, and both databases "+
			"(D225)", facts)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
}
