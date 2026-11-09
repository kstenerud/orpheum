// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// cutOff is a transport that reaches nothing while isCut, as a house whose network is down, and the
// discovery otherwise.
type cutOff struct {
	isCut   atomic.Bool
	through *http.Client
}

func (cut *cutOff) RoundTrip(request *http.Request) (*http.Response, error) {
	if cut.isCut.Load() {
		return nil, errors.New("the network is down")
	}
	return cut.through.Do(request) //nolint:gosec // the test's own fake discovery, on this machine
}

// A core backing off after its network went down looks every reachProbeEvery whether the discovery
// answers again, and asks at once when it does, rather than up to discoveryRetryMostSeconds later:
// the last holder of a realm must not miss its window (D437 §5).
func TestRun_ACoreWhoseNetworkComesBackAsksAtOnce(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	cut := &cutOff{through: built.service.parts.HTTP}
	built.service.parts.HTTP = &http.Client{Transport: cut}
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(ctx)
	cut.isCut.Store(true)
	built.service.mutex.Lock()
	built.service.token = ""
	built.service.mutex.Unlock()
	built.service.step(ctx)
	if !built.service.isWaitingForTheNetwork() {
		t.Fatalf("a core that reached no discovery does not wait for the network; status %+v", built.service.Status())
	}
	cut.isCut.Store(false)
	select { // a nudge left from connecting is no answer from the discovery
	case <-built.service.wake:
	default:
	}
	done := make(chan bool, 1)
	go func() { done <- built.service.waitForNextStep(ctx, 1000*time.Hour) }()
	for isDone := false; !isDone; {
		select {
		case isGoingOn := <-done:
			if !isGoingOn {
				t.Fatal("the wait ended as though the core stopped")
			}
			isDone = true
		case <-time.After(5 * time.Millisecond):
			built.clock.Advance(reachProbeEvery)
		}
	}
	if built.service.isWaitingForTheNetwork() {
		t.Error("the discovery answered again, and the core still waits for the network")
	}
	testutil.Receive(t, built.service.reachedAgain, "the signalling loop woken as the discovery answers again")
}

// A core whose own health check fails holds no signalling connection, so the discovery's ping of it
// goes unanswered; one whose check fails while it holds the connection closes it (D437 §2).
func TestSignal_AnUnhealthyCoreHoldsNoSignallingConnection(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	var isSick atomic.Bool
	isSick.Store(true)
	built.service.parts.Answerer = &answering{}
	built.service.parts.Healthy = func(context.Context) error {
		if isSick.Load() {
			return errors.New("the database cannot be read")
		}
		return nil
	}
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(ctx)
	var dials atomic.Int32
	built.fake.mutex.Lock()
	built.fake.answersPings = true
	built.fake.onSignal = func() { dials.Add(1) }
	built.fake.mutex.Unlock()
	if _, err := built.service.holdSignalling(ctx, built.service.Status().BaseURL, &lostSignalling{}); err == nil ||
		dials.Load() != 0 {
		t.Errorf("an unhealthy core dialled the signalling %d times, with %v; want none, and its health the error",
			dials.Load(), err)
	}
	isSick.Store(false)
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Its end is what is awaited.
		base := built.service.Status().BaseURL
		_, _ = built.service.holdSignalling(ctx, base, &lostSignalling{}) //nolint:errcheck // as above
	}()
	for dials.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	isSick.Store(true)
	for isDone := false; !isDone; {
		select {
		case <-done:
			isDone = true
		case <-time.After(5 * time.Millisecond):
			built.clock.Advance(healthCheckEvery)
		}
	}
}

// The serving core keeps and logs what the discovery says of a copy of it that it refused: where the
// copy asked from, and when (D437 §4).
func TestSignal_TheServingCoreSaysACopyWasRefused(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	log := &testutil.Log{}
	built.service.parts.Logger = slog.New(slog.NewTextHandler(log, nil))
	built.service.parts.Answerer = &answering{}
	if _, err := built.service.Connect(t.Context(), built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(t.Context())
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		built.service.signal(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		testutil.Receive(t, done, "the signalling loop to end")
	})
	refusedAt := time.Date(2026, 11, 6, 9, 30, 0, 0, time.UTC)
	built.fake.signals <- discoveryproto.SignalMessage{
		Type: discoveryproto.SignalCopyRefused, Address: "203.0.113.7", At: refusedAt.UnixMilli(),
	}
	line := log.AwaitEvent(t, "copyRefused")
	status := built.service.Status()
	if status.CopyRefusedFrom != "203.0.113.7" || !status.CopyRefusedAt.Equal(refusedAt) ||
		!strings.Contains(line, "203.0.113.7") {
		t.Errorf("the refused copy was kept as from %q at %s, logged %q; want 203.0.113.7 at %s",
			status.CopyRefusedFrom, status.CopyRefusedAt, line, refusedAt)
	}
}
