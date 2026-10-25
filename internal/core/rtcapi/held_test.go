// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"strconv"
	"testing"
	"time"
)

// A connection is named in the log by its number, and by whose it is once a request of it was
// signed in, the first sign-in naming it for good; a request over plain HTTP names nothing.
func TestHeld_AConnectionIsNamedByItsFirstSignedInRequest(t *testing.T) {
	t.Parallel()
	held := newHeldConnection(7, time.Unix(0, 0))
	ctx := withHeld(t.Context(), held)
	if got := held.name(); got != "c"+strconv.FormatInt(held.number, 10)+" (no sign-in yet)" {
		t.Errorf("before any sign-in: %q", got)
	}
	NoteClient(ctx, "user 1 device 18")
	NoteClient(withHeld(t.Context(), held), "user 2 device 3")
	if got, want := held.name(), "c"+strconv.FormatInt(held.number, 10)+" (user 1 device 18)"; got != want {
		t.Errorf("named %q; want %q", got, want)
	}
	if held.requests.Load() != 2 {
		t.Errorf("counted %d requests; want 2", held.requests.Load())
	}
	NoteClient(t.Context(), "user 9 device 9") // a request over HTTP: nothing to name
}

// The log lists a network's connections in the order they opened.
func TestHeld_ANetworksConnectionsAreListedInOrder(t *testing.T) {
	t.Parallel()
	first, second := newHeldConnection(1, time.Unix(0, 0)), newHeldConnection(2, time.Unix(0, 0))
	NoteClient(withHeld(t.Context(), second), "user 1 device 30")
	want := first.name() + ", " + second.name()
	if got := namesOf([]*heldConnection{second, first}); got != want {
		t.Errorf("listed %q; want %q", got, want)
	}
}
