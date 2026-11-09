// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A locked node says who it is and what it lacks at once and at each tick, and says no hello, until
// the core answers that an admin lets it run unconfined (D450 §1).
func TestWaitForLeave_ReportsEachTickUntilTheCoreLetsItRunUnconfined(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	lacking := []nodeproto.Confinement{{Mechanism: "landlock", CauseCode: "disabledAtBoot"}}
	tick := make(chan time.Time)
	allowed := make(chan bool, 1)
	go func() {
		allowed <- WaitForLeave(t.Context(), socket, nodeproto.LockedReport{Name: "nas", Confinement: lacking}, tick,
			slog.New(slog.DiscardHandler))
	}()
	first := testutil.Receive(t, core.lockedReports, "the first locked report")
	if first.Name != "nas" || len(first.Confinement) != 1 || first.ProtocolVersion != nodeproto.ProtocolVersion {
		t.Errorf("the locked report = %+v; want the node's name, what it lacks and its protocol", first)
	}
	tick <- time.Time{}
	testutil.Receive(t, core.lockedReports, "a locked report at the tick")
	core.mutex.Lock()
	core.mayRunUnconfined = true
	core.mutex.Unlock()
	tick <- time.Time{}
	testutil.Receive(t, core.lockedReports, "the report the core lets run")
	if !testutil.Receive(t, allowed, "the wait to end") {
		t.Error("WaitForLeave = false; want true once the core lets the node run unconfined")
	}
	if count := core.helloCount(); count != 0 {
		t.Errorf("%d hellos while locked; want none", count)
	}
}

// A locked node whose wait is ended reports it was not let run.
func TestWaitForLeave_IsFalseWhenTheNodeIsStopped(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	ctx, cancel := context.WithCancel(t.Context())
	allowed := make(chan bool, 1)
	go func() {
		allowed <- WaitForLeave(ctx, socket, nodeproto.LockedReport{Name: "nas"}, make(chan time.Time),
			slog.New(slog.DiscardHandler))
	}()
	testutil.Receive(t, core.lockedReports, "the first locked report")
	cancel()
	if testutil.Receive(t, allowed, "the wait to end") {
		t.Error("WaitForLeave = true after the node stopped; want false")
	}
}
