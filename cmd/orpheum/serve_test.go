// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/outbound"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
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
// given message, or a record carrying logEvent with a given value, is logged, so a test waits for
// "ready" instead of sleeping: a wait by logEvent survives a reword of the message it names (test
// architecture review, 2026-10-26, finding 12).
type signallingHandler struct {
	slog.Handler
	mutex   *sync.Mutex
	buffer  *bytes.Buffer
	signals map[string]chan struct{}
}

func (handler signallingHandler) Handle(ctx context.Context, record slog.Record) error {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()
	for _, key := range awaitKeysOf(record) {
		if signal, isAwaited := handler.signals[key]; isAwaited {
			close(signal)
			delete(handler.signals, key)
		}
	}
	return handler.Handler.Handle(ctx, record)
}

// awaitKeysOf are the names a record can be awaited by: its message, and its logEvent attribute's
// value if it carries one.
func awaitKeysOf(record slog.Record) []string {
	keys := []string{record.Message}
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == logEvent {
			keys = append(keys, attr.Value.String())
		}
		return true
	})
	return keys
}

func (handler signallingHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	handler.Handler = handler.Handler.WithAttrs(attributes)
	return handler
}

// couldNotStart is logEvent's value for a server that could not start; a test waiting for anything
// else fails at it rather than at its wait's limit.
const couldNotStart = eventCouldNotStart

// capturedLog is a debug-level JSON log in memory that can be waited on.
type capturedLog struct {
	mutex   *sync.Mutex
	buffer  *bytes.Buffer
	signals map[string]chan struct{}
	logger  *slog.Logger
}

func newCapturedLog(awaited ...string) capturedLog {
	captured := capturedLog{mutex: &sync.Mutex{}, buffer: &bytes.Buffer{}, signals: map[string]chan struct{}{}}
	for _, message := range append(awaited, couldNotStart) {
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

// testConfig is a core instance in a temporary directory, made by default-config with its full
// config written for a test, loaded as the core loads it, and returned with its directory. The
// returned config also carries a node's table, for a test that runs a node beside the core in this
// process: its sockets and prepared directory are then the core instance's run and cache
// directories, and it dials the core's socket there.
func testConfig(t *testing.T) (Config, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "i")
	// A Unix socket's path is at most 103 bytes, and t.TempDir's carries the test's name: the run
	// directory is put outside the instance, as default-config's --run-dir puts it, at the shortest
	// path there is.
	instance, err := makeInstance(dir, kindCore, filepath.Join(root, "r"), "test")
	if err != nil {
		t.Fatal(err)
	}
	text := `[server]
listen = "127.0.0.1:0"
read_header_timeout_seconds = 10
shutdown_grace_seconds = 10
host_names = []
home_networks = ["127.0.0.0/8", "::1/128"]
this_machine_listen = ""

[https]
domain = ""
contact = ""
listen = ":443"
acme_directory = "https://acme-v02.api.letsencrypt.org/directory"

[turn]
public_address = ""
listen = ":3478"
relay_port_min = 49160
relay_port_max = 49199

[discovery]
configured_addresses = []
lan_addresses = []
send_interface_addresses = true
house_ipv6_networks = []

[webrtc]
udp_port = 0
stun_servers = []
max_connections = 64

[database]
read_connections_minimum = 4
migrate_free_space_db_multiple = 3
when_unmigratable = "refuse"

[log]
level = "debug"
format = "json"
destination = "stderr"
`
	if err = os.WriteFile(instance.fullConfigPath(), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	config, report, err := loadInstanceConfig(dir, kindCore)
	if err != nil || len(report.Notes()) != 0 {
		t.Fatalf("the test config does not load cleanly: %v, %v", err, report.Notes())
	}
	config.Node = NodeConfig{
		Name: "local", CoreSocket: filepath.Join(config.Instance.RunDir, coreSocketName), RootPaths: []string{},
		FFmpegDir: filepath.Join(dir, "ffmpeg"), ConfinementPolicy: "report",
		ProbeTimeoutSeconds: 60, EndedWorkKeptHours: 168,
		ReadHeaderTimeoutSeconds: 10, PreparedBudgetGiB: 1, PreparedKeptAfterPlayHours: 72,
		PreparedDiskUsedLimitPercent: 95, PreparedDiskMinFreeMiB: 100, EncodeSlotCount: 1, RemuxSlotCount: 2,
		SlowestReadMiBPerSecond: 20,
	}
	return config, dir
}

// runningServer is a server started by serve on a loopback listener.
type runningServer struct {
	baseURL string
	stop    func() error
}

// serveWaitLimit is how long startServing waits for the log line it awaits before failing with
// the log so far, rather than hanging until the test binary's timeout says nothing useful.
const serveWaitLimit = 5 * time.Minute

// startServing runs serve until the test stops it, and waits until it logs awaited. When node is not
// nil, it runs beside the core until the test stops both, as a node's own service runs beside the
// core's: runNodeWith in this process.
func startServing(
	t *testing.T, config Config, captured capturedLog, awaited string, node func(ctx context.Context) error,
) runningServer {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return startServingOn(t, listener, config, captured, awaited, node)
}

// startServingOn is startServing on the test's own listener. An instance with no persistent.db yet
// starts from a seeded one (seedPersistentDatabase); one the test made or served before starts
// from what it holds.
func startServingOn(
	t *testing.T, listener net.Listener, config Config, captured capturedLog, awaited string,
	node func(ctx context.Context) error,
) runningServer {
	t.Helper()
	seedPersistentDatabase(t, config)
	return serveOn(t, listener, config, captured, awaited, node)
}

// serveOn runs serve on listener until the test stops it, and waits until it logs awaited, as
// startServing says.
func serveOn(
	t *testing.T, listener net.Listener, config Config, captured capturedLog, awaited string,
	node func(ctx context.Context) error,
) runningServer {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		outside := wiring{
			tmdbBaseURL: unreachableURL, tmdbImagesBaseURL: unreachableURL, openSubtitlesBaseURL: unreachableURL,
		}
		done <- serve(ctx, config, ConfigReport{}, listener, outside, captured.logger)
	}()
	nodeDone := make(chan error, 1)
	if node == nil {
		nodeDone <- nil
	} else {
		go func() { nodeDone <- node(ctx) }()
	}
	failedToStart := captured.signals[couldNotStart]
	if awaited == couldNotStart {
		failedToStart = nil
	}
	select {
	case <-captured.signals[awaited]:
	case <-failedToStart:
		cancel()
		t.Fatalf("the server could not start, waiting for %q:\n%s", awaited, captured)
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
		served := testutil.Receive(t, done, "serve to stop")
		return errors.Join(served, testutil.Receive(t, nodeDone, "the node to stop"))
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
	code := runCommand(t, "setup-code", "--instance", configPath)
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
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "persistent.db"), storetest.OpenOptions())
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
	steps, err := store.Steps()
	if err != nil {
		t.Fatal(err)
	}
	for index, observed := range seen {
		step, before := index+1, 0 // a new file is at schema 0; then at each step's version (a baseline's is its own)
		if index > 0 {
			before = steps[index-1].Version
		}
		if !strings.HasPrefix(observed, "200 ") || !strings.Contains(observed, `"status":"migrating"`) ||
			!strings.Contains(observed, fmt.Sprintf(`"detail":"step %d of`, step)) ||
			!strings.HasSuffix(observed, fmt.Sprintf("at schema %d", before)) {
			t.Errorf("before step %d, /health = %s; want 200, migrating, that step, and the schema not yet at it",
				step, observed)
		}
	}
}

// The one serving test that starts cold, as a new installation does: every other starts from a
// seeded persistent.db (seedPersistentDatabase).
func TestServe_ComesUpReadyAndStopsCleanly(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	captured := newCapturedLog("ready")
	server := startServingCold(t, config, captured, "ready")
	health := requestAPI(t, http.MethodGet, server.baseURL+"/health", "", nil)
	if health.status != http.StatusOK || !strings.Contains(health.body, `"status":"ready"`) {
		t.Errorf("/health once ready = %d %s", health.status, health.body)
	}
	if logged := captured.String(); !strings.Contains(logged, `"msg":"migrating"`) ||
		!strings.Contains(logged, "wrote the starting value of settings that had none") {
		t.Errorf("a cold start neither migrated from empty nor wrote the settings' starting values:\n%s", logged)
	}
	if !strings.Contains(captured.String(), `"setupCode"`) {
		t.Errorf("a server with no users logs no setup code:\n%s", captured)
	}
	if err := server.stop(); err != nil {
		t.Errorf("serve returned %v after a clean stop, want nil", err)
	}
}

// A new installation hashes a password at argon2id's production costs, RFC 9106's second option
// (D194 §2): the costs every other serving test lowers (seedPersistentDatabase) are its own.
func TestServe_ANewInstallationHashesPasswordsAtTheProductionCosts(t *testing.T) {
	t.Parallel()
	config, path := testConfig(t)
	server := startServingCold(t, config, newCapturedLog("ready"), "ready")
	setUpOwner(t, server, path, "owner")
	hashes := storetest.QueryRaw(t, filepath.Join(config.Instance.PersistentDir, "persistent.db"),
		"SELECT CAST(password_hash AS TEXT) FROM user WHERE name = 'owner'")
	production := regexp.MustCompile(`^\$argon2id\$v=19\$m=65536,t=3,p=4\$`)
	if len(hashes) != 1 || !production.MatchString(hashes[0]) {
		t.Errorf("the owner's password hash = %q; want argon2id at m=65536, t=3, p=4", hashes)
	}
}

// The core's node socket takes a hello only from the holder of the key it keeps beside itself: a
// process of the same user without it, a GPU run that may open a Unix socket among them, cannot say
// hello as a node (D331 §1, the Plex-chain review's N1).
func TestServe_TheNodeSocketTakesNothingFromAProcessWithoutItsKey(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	defer func() { _ = server.stop() }() //nolint:errcheck // the test's own server
	socket := filepath.Join(config.Instance.RunDir, coreSocketName)
	hello := func(client *http.Client) int {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://core"+nodeproto.PathHello,
			strings.NewReader(`{"name":"impostor"}`))
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close() //nolint:errcheck // only the status matters
		return response.StatusCode
	}
	stranger := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}
	if got := hello(stranger); got != http.StatusUnauthorized {
		t.Errorf("a hello without the socket's key: %d; want 401", got)
	}
	if got := hello(unixsocket.Client(socket, 0)); got == http.StatusUnauthorized {
		t.Errorf("a hello with the socket's key: %d; want it taken past the key", got)
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
	if err := os.MkdirAll(config.Instance.PersistentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(config.Instance.PersistentDir, "persistent.db")
	if err := os.WriteFile(foreign, []byte("SQLite format 3\x00"+strings.Repeat("\x00", 84)), 0o600); err != nil {
		t.Fatal(err)
	}
	server := startServing(t, config, newCapturedLog(couldNotStart), couldNotStart, nil)
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
