// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/sha256"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/storetest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// countingFlusher counts flushes, and whether each one's context was still live.
type countingFlusher struct {
	mutex      sync.Mutex
	flushes    int
	liveAtStop bool
	flushed    chan struct{}
}

func (flusher *countingFlusher) FlushSessionUses(ctx context.Context) error {
	flusher.mutex.Lock()
	defer flusher.mutex.Unlock()
	flusher.flushes++
	flusher.liveAtStop = ctx.Err() == nil
	select {
	case flusher.flushed <- struct{}{}:
	default:
	}
	return nil
}

// fixedLengths answers every duration setting with one length.
type fixedLengths time.Duration

func (length fixedLengths) Duration(context.Context, settings.Key) (time.Duration, error) {
	return time.Duration(length), nil
}

// The sessions' uses are written on the setting's interval, and once more as the server stops,
// with a context that is still live, so the last uses reach the database before it closes.
func TestWriteSessionUses_FlushesOnItsIntervalAndAtStop(t *testing.T) {
	t.Parallel()
	flusher := &countingFlusher{flushed: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		writeSessionUses(ctx, flusher, fixedLengths(time.Millisecond), slog.New(slog.DiscardHandler))
		close(stopped)
	}()
	testutil.Receive(t, flusher.flushed, "a flush on the interval")
	cancel()
	testutil.Receive(t, stopped, "the loop to stop")
	flusher.mutex.Lock()
	defer flusher.mutex.Unlock()
	if flusher.flushes < 2 || !flusher.liveAtStop {
		t.Errorf("%d flushes, the last with a live context: %t; want one on the interval and one at stop, live",
			flusher.flushes, flusher.liveAtStop)
	}
}

// A served request only remembers its session's use, and the server writes it at the latest as it
// stops (D303 §4).
func TestServe_WritesASessionsUseByTheTimeItStops(t *testing.T) {
	t.Parallel()
	config, configPath := testConfig(t)
	server := startServing(t, config, newCapturedLog("ready"), "ready", nil)
	token := signedInOwner(t, server, configPath)
	time.Sleep(5 * time.Millisecond) // the use must come after the sign-in's own stamp
	if me := requestAPI(t, http.MethodGet, server.baseURL+"/me", token, nil); me.status != http.StatusOK {
		t.Fatalf("GET /me = %d %s", me.status, me.body)
	}
	if err := server.stop(); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(config.Instance.PersistentDir, "persistent.db"),
		storetest.OpenOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() }) //nolint:errcheck // the test is over
	digest := sha256.Sum256([]byte(token))
	state, err := database.Read().GetGrantState(t.Context(), digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if state.LastUsedAt == nil || *state.LastUsedAt <= state.IssuedAt {
		t.Errorf("after the server stopped the session was last used at %v, issued at %d; want its use written",
			state.LastUsedAt, state.IssuedAt)
	}
}
