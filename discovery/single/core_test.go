// SPDX-License-Identifier: AGPL-3.0-or-later

package single_test

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/discovery/single"
	"github.com/kstenerud/orpheum/internal/core/discovery"
	"github.com/kstenerud/orpheum/internal/core/identity"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/core/rtcapi"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// memoryFiles is the core's persistent directory, in memory.
type memoryFiles struct {
	mutex sync.Mutex
	files map[string][]byte
}

func (files *memoryFiles) ReadPersistentFile(name string) ([]byte, error) {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	contents, found := files.files[name]
	if !found {
		return nil, instancedir.ErrNotFound
	}
	return contents, nil
}

func (files *memoryFiles) WritePersistentFile(name string, contents []byte) error {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	files.files[name] = contents
	return nil
}

func (files *memoryFiles) RemovePersistentFile(name string) error {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	delete(files.files, name)
	return nil
}

// answering answers every offer as the core's WebRTC server would.
type answering struct{}

func (answering) Answer(_ context.Context, offer rtcapi.Description) (rtcapi.Description, error) {
	return rtcapi.Description{Type: "answer", SDP: "answer to " + offer.SDP}, nil
}

// The core's own discovery client, unchanged, enrols with a single-target discovery, is claimed by
// the code it shows, sends its addresses, and answers the offer the discovery passes it (W4b.2).
func TestCore_ACoreEnrolsIsClaimedAndSendsItsAddresses(t *testing.T) {
	t.Parallel()
	coreClock := clocktest.NewFake(time.Date(2026, 10, 11, 20, 0, 0, 0, time.UTC))
	logs := &testutil.Log{}
	// The discovery keeps a clock of its own, which stands still: the core's sleeps move the core's
	// fake clock on by hours, which would expire every nonce the discovery issued meanwhile.
	discoveryNow := coreClock.Now()
	held, err := single.New(single.Parts{
		Zone: "home.example", Store: single.FileStore{Path: filepath.Join(t.TempDir(), "discovery.json")},
		Now: func() time.Time { return discoveryNow }, Random: rand.Reader,
		Logger: slog.New(slog.NewTextHandler(logs, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(held.Handler())
	t.Cleanup(server.Close)
	key, err := identity.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	core, err := discovery.New(discovery.Parts{
		Files: &memoryFiles{files: map[string][]byte{}}, Key: key, HTTP: &http.Client{}, Clock: coreClock,
		Logger: slog.New(slog.NewTextHandler(logs, nil)), Version: "v0.2.0", APIMajor: 1,
		Candidates: func() []discoveryproto.Candidate {
			return []discoveryproto.Candidate{{Address: "2a0f:1000::1", Port: 443, Source: "interface"}}
		},
		Answerer: answering{}, OnClaimed: func(context.Context) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := core.Connect(t.Context(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	// The owner types the code the core's System page shows: `orpheum discovery claim`.
	if err = held.Claim(status.ClaimCode); err != nil {
		t.Fatalf("claiming with the core's own code %q: %v", status.ClaimCode, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		core.Run(ctx)
	}()
	// The core's loop waits on its clock: it is moved on until the core has done its part.
	ticker := time.NewTicker(20 * time.Millisecond)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				coreClock.Advance(time.Second)
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		testutil.Receive(t, done, "the core's discovery loop to end")
	})
	logs.AwaitLine(t, "the core sent its addresses", "candidates=1", "orpheumVersion=v0.2.0")
	logs.AwaitLine(t, "the core's signalling connected")
	answer, err := held.Offer(t.Context(), "a browser's offer")
	if err != nil || answer != "answer to a browser's offer" {
		t.Errorf("the offer passed through the discovery was answered %q, %v", answer, err)
	}
}
