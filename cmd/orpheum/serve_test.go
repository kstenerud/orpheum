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

	"github.com/kstenerud/orpheum/internal/core/api"
	"github.com/kstenerud/orpheum/internal/store"
)

func TestMain(m *testing.M) {
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
	config := Config{
		Server: ServerConfig{
			Listen: "127.0.0.1:0", DataDir: filepath.Join(root, "state"), CacheDir: filepath.Join(root, "cache"),
		},
		Node: NodeConfig{Confinement: "report"},
		Log:  LogConfig{Level: "debug", Format: "json"},
	}
	path := filepath.Join(root, "orpheum.toml")
	text := fmt.Sprintf("[server]\nlisten = %q\ndata_dir = %q\ncache_dir = %q\n\n[node]\nconfinement = \"report\"\n\n"+
		"[log]\nlevel = \"debug\"\nformat = \"json\"\n",
		config.Server.Listen, config.Server.DataDir, config.Server.CacheDir)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return config, path
}

// running is a server started by serve on a loopback listener.
type running struct {
	baseURL string
	stop    func() error
}

// startServing runs serve until the test stops it, and waits until it logs awaited.
func startServing(t *testing.T, config Config, captured capturedLog, awaited string) running {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, config, listener, captured.logger) }()
	select {
	case <-captured.signals[awaited]:
	case err := <-done:
		cancel()
		t.Fatalf("serve returned before logging %q: %v\n%s", awaited, err, captured)
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
	return running{baseURL: "http://" + listener.Addr().String() + api.Prefix, stop: stop}
}

type reply struct {
	status int
	body   string
}

func call(t *testing.T, method, url, token string, body any) reply {
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
	answer, err := http.DefaultClient.Do(request)
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
	return reply{status: answer.StatusCode, body: string(text)}
}

func TestServe_HealthSaysMigratingBeforeTheMigrationStarts(t *testing.T) {
	t.Parallel()
	server := api.NewServer(slog.New(slog.DiscardHandler))
	served := httptest.NewServer(server.Handler())
	t.Cleanup(served.Close)
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"))
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
		SnapshotDirectory: t.TempDir(),
		BeforeStep: func(step, total int) {
			report(step, total)
			health := call(t, http.MethodGet, served.URL+api.Prefix+"/health", "", nil)
			state, stateErr := database.State(t.Context())
			if stateErr != nil {
				t.Error(stateErr)
			}
			seen = append(seen, fmt.Sprintf("%d %s at schema %d", health.status, health.body, state.Version))
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("no step ran; the test saw nothing")
	}
	for _, observed := range seen {
		if !strings.HasPrefix(observed, "200 ") || !strings.Contains(observed, `"status":"migrating"`) ||
			!strings.Contains(observed, "step 1 of") || !strings.HasSuffix(observed, "at schema 0") {
			t.Errorf("before the first step, /health = %s; want 200, migrating, step 1, and the schema untouched",
				observed)
		}
	}
}

func TestServe_ComesUpReadyAndStopsCleanly(t *testing.T) {
	t.Parallel()
	config, _ := testConfig(t)
	captured := newCapturedLog("ready")
	server := startServing(t, config, captured, "ready")
	health := call(t, http.MethodGet, server.baseURL+"/health", "", nil)
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
	server := startServing(t, config, newCapturedLog(failed), failed)
	health := call(t, http.MethodGet, server.baseURL+"/health", "", nil)
	if health.status != http.StatusServiceUnavailable || !strings.Contains(health.body, `"status":"degraded"`) {
		t.Errorf("/health after a failed start = %d %s, want 503 degraded with the reason", health.status, health.body)
	}
	if err := server.stop(); err == nil {
		t.Error("serve returned nil after a failed start; the exit status must say it failed")
	}
}
