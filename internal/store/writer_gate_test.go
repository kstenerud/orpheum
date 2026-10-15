// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/testutil"
)

// waitForWaiters waits until the gate holds count waiting writes.
func waitForWaiters(t *testing.T, gate *writerGate, count int) {
	t.Helper()
	for range 10_000 {
		gate.mutex.Lock()
		waiting := len(gate.people) + len(gate.background)
		gate.mutex.Unlock()
		if waiting == count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the gate never had %d waiting writes", count)
}

// gateOrder holds the writer, queues writes one at a time in the order given (a name starting
// "person" a person's, any other background work), lets the writer go, and returns the order the
// gate handed it out.
func gateOrder(t *testing.T, names ...string) []string {
	t.Helper()
	var gate writerGate
	if err := gate.acquire(t.Context(), false, "first"); err != nil {
		t.Fatal(err)
	}
	var (
		mutex   sync.Mutex
		order   []string
		running sync.WaitGroup
	)
	for i, name := range names {
		running.Go(func() {
			if err := gate.acquire(t.Context(), strings.HasPrefix(name, "person"), name); err != nil {
				t.Error(err)
				return
			}
			mutex.Lock()
			order = append(order, name)
			mutex.Unlock()
			gate.release()
		})
		waitForWaiters(t, &gate, i+1)
	}
	gate.release()
	running.Wait()
	return order
}

func TestWriterGate_HandsTheWriterToAPersonBeforeBackgroundWork(t *testing.T) {
	t.Parallel()
	got := gateOrder(t, "pass", "fetch", "person")
	if want := []string{"person", "pass", "fetch"}; !slices.Equal(got, want) {
		t.Errorf("the gate handed the writer out as %v; want %v", got, want)
	}
}

func TestWriterGate_BackgroundWorkGoesNextOncePassedOverEnough(t *testing.T) {
	t.Parallel()
	got := gateOrder(t, "pass", "person1", "person2", "person3", "person4", "person5", "person6")
	want := []string{"person1", "person2", "person3", "person4", "pass", "person5", "person6"}
	if !slices.Equal(got, want) {
		t.Errorf("the gate handed the writer out as %v; want %v (background work after %d people)",
			got, want, backgroundPassedOverAtMost)
	}
}

func TestWriterGate_AWriteThatGivesUpLeavesTheQueue(t *testing.T) {
	t.Parallel()
	var gate writerGate
	if err := gate.acquire(t.Context(), false, "first"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	gaveUp := make(chan error, 1)
	go func() { gaveUp <- gate.acquire(ctx, true, "impatient") }()
	waitForWaiters(t, &gate, 1)
	cancel()
	if err := testutil.Receive(t, gaveUp, "the impatient write to give up"); err == nil {
		t.Fatal("a write whose context ended got the writer")
	}
	waitForWaiters(t, &gate, 0)
	gate.release()
	if err := gate.acquire(t.Context(), false, "next"); err != nil {
		t.Errorf("after the impatient write gave up, the writer was not free: %v", err)
	}
}
