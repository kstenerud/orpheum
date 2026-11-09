// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
	"github.com/kstenerud/orpheum/internal/core/instancedir"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// A Leave while a step is in flight, even one enrolling the core again with a discovery that dropped
// it, waits for the step and then takes out what it left: no enrolment stays, in the core or at the
// discovery (the owner, 2026-11-09).
func TestLeave_WhileAStepIsInFlightLeavesNoEnrolment(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	logged := &testutil.Log{}
	built.service.parts.Logger = slog.New(slog.NewTextHandler(logged, nil))
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(ctx)
	built.fake.refuse(discoveryproto.RefusalEnrolAgain)
	built.service.step(ctx) // its session refused: the next step enrols again
	enrolling := built.fake.pause(t, "POST /enrol")
	stepped := make(chan struct{})
	go func() {
		defer close(stepped)
		built.service.step(ctx)
	}()
	testutil.Receive(t, enrolling.arrived, "the step's enrolment")
	left := make(chan error, 1)
	go func() { left <- built.service.Leave(ctx) }()
	logged.AwaitEvent(t, "leaveWaitsForStep")
	enrolling.release()
	testutil.Receive(t, stepped, "the step's end")
	if err := testutil.Receive(t, left, "the Leave's end"); err != nil {
		t.Fatal(err)
	}
	assertLeft(t, built)
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	if !built.fake.isLeft {
		t.Error("the core the step enrolled again was not taken out of the discovery")
	}
}

// A step that comes while a Leave is under way does nothing, so the discovery that has already let
// the core go is not asked to enrol it again (the owner, 2026-11-09).
func TestStep_DuringALeaveDoesNothing(t *testing.T) {
	t.Parallel()
	built := newWorld(t)
	ctx := t.Context()
	if _, err := built.service.Connect(ctx, built.issuer, built.fake.chain.ID()); err != nil {
		t.Fatal(err)
	}
	built.fake.claim()
	built.service.step(ctx)
	deleting := built.fake.pause(t, "DELETE /core")
	left := make(chan error, 1)
	go func() { left <- built.service.Leave(ctx) }()
	testutil.Receive(t, deleting.arrived, "the Leave's delete")
	// The discovery has let the core go while the Leave waits for its answer.
	built.fake.refuse(discoveryproto.RefusalEnrolAgain)
	built.service.step(ctx)
	built.service.step(ctx)
	deleting.release()
	if err := testutil.Receive(t, left, "the Leave's end"); err != nil {
		t.Fatal(err)
	}
	assertLeft(t, built)
	built.fake.mutex.Lock()
	defer built.fake.mutex.Unlock()
	if built.fake.enrolCount != 1 {
		t.Errorf("the core enrolled %d times; want once, steps during the Leave enrolling it no more",
			built.fake.enrolCount)
	}
}

// assertLeft fails the test unless the core names no discovery and keeps no state file.
func assertLeft(t *testing.T, built *world) {
	t.Helper()
	if status := built.service.Status(); status.BaseURL != "" || status.CoreID != "" || status.State != "" {
		t.Errorf("after the Leave the status is %+v; want enrolled with no discovery", status)
	}
	if _, err := built.files.ReadPersistentFile(StateFileName); !errors.Is(err, instancedir.ErrNotFound) {
		t.Errorf("%s is there after the Leave: %v", StateFileName, err)
	}
}
