// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"errors"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/discovery/discoveryproto"
)

// The exit path (D25; discovery-core K11; the paid spec §3.10): a discovery moves a claimed core
// through grace, frozen and released. In grace the core is published and keeps sending its
// addresses; frozen and released, it stops, waits the ended check between askings, and keeps its
// own id and zone, so a renewal before or after released restores it as it was with no new link.
func TestStep_TheExitPathWalksGraceFrozenAndReleasedAndARenewalRestoresTheCore(t *testing.T) {
	t.Parallel()
	built := claimedWorld(t)
	ctx := t.Context()
	claimed := built.service.Status()
	walk := []struct {
		state         discoveryproto.State
		isPublished   bool
		isHeartbeated bool
	}{
		{discoveryproto.StateGrace, true, true},
		{discoveryproto.StateFrozen, false, false},
		{discoveryproto.StateReleased, false, false},
		{discoveryproto.StateClaimed, true, true},
	}
	for _, step := range walk {
		built.fake.become(step.state)
		built.clock.Advance(heartbeatEvery)
		built.service.step(ctx)
		sent := len(built.fake.heartbeats)
		built.clock.Advance(heartbeatEvery)
		wait := built.service.step(ctx)
		status := built.service.Status()
		if status.State != step.state {
			t.Errorf("after the discovery said %s the core's state is %q", step.state, status.State)
		}
		if status.IsPublished() != step.isPublished {
			t.Errorf("%s: IsPublished = %t; want %t", step.state, status.IsPublished(), step.isPublished)
		}
		if isHeartbeated := len(built.fake.heartbeats) > sent; isHeartbeated != step.isHeartbeated {
			t.Errorf("%s: sent its addresses = %t; want %t", step.state, isHeartbeated, step.isHeartbeated)
		}
		if !step.isHeartbeated && wait != endedEvery {
			t.Errorf("%s: the core waits %s before asking again; want the ended check's %s", step.state, wait,
				endedEvery)
		}
		if status.CoreID != claimed.CoreID || status.CertificateName() != claimed.CertificateName() {
			t.Errorf("%s: the core is %q under %q; want its own %q under %q, kept", step.state, status.CoreID,
				status.CertificateName(), claimed.CoreID, claimed.CertificateName())
		}
	}
	if restarted := built.restart(t).Status(); restarted.CertificateName() != claimed.CertificateName() {
		t.Errorf("after the walk and a restart the core's names are %q; want %q", restarted.CertificateName(),
			claimed.CertificateName())
	}
}

// A lapse closes the core's signalling connection at once and refuses each redial (403). The refusal
// has the core ask how it stands at once, not at its next heartbeat up to five minutes later, so the
// System page says frozen within seconds (VQ371); a discovery refusing every redial while it says the
// core is claimed is asked at most once each standingAskEvery, however fast the redials come.
func TestSignal_ARefusedRedialAsksHowTheCoreStandsAtOnceAndAtMostOnceAWhile(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	refused := func(t *testing.T, built *world) bool {
		t.Helper()
		select { // a nudge left from connecting is no refusal's
		case <-built.service.wake:
		default:
		}
		_, err := built.service.holdSignalling(ctx, built.service.Status().BaseURL, &lostSignalling{})
		if !errors.Is(err, errSignallingRefused) {
			t.Fatalf("the refused dial said %v; want %v", err, errSignallingRefused)
		}
		select {
		case <-built.service.wake:
			return true
		default:
			return false
		}
	}
	heartbeats := func(built *world) int {
		built.fake.mutex.Lock()
		defer built.fake.mutex.Unlock()
		return len(built.fake.heartbeats)
	}

	lapsed := claimedWorld(t)
	lapsed.fake.become(discoveryproto.StateFrozen)
	if !refused(t, lapsed) {
		t.Error("a refused redial did not wake the step loop")
	}
	lapsed.service.step(ctx)
	if state := lapsed.service.Status().State; state != discoveryproto.StateFrozen {
		t.Errorf("the step after the refused redial left the core %q; want %q, asked at once", state,
			discoveryproto.StateFrozen)
	}

	looping := claimedWorld(t)
	looping.fake.mutex.Lock()
	looping.fake.refusesSignalling = true
	looping.fake.mutex.Unlock()
	for _, look := range []struct {
		after    time.Duration
		isAsking bool
	}{{0, true}, {time.Second, false}, {standingAskEvery - time.Second, true}} {
		looping.clock.Advance(look.after)
		sent := heartbeats(looping)
		isWoken := refused(t, looping)
		looping.service.step(ctx)
		if isAsked := heartbeats(looping) > sent; isWoken != look.isAsking || isAsked != look.isAsking {
			t.Errorf("a refusal %s after the last: woke the loop %t and asked %t; want %t", look.after, isWoken,
				isAsked, look.isAsking)
		}
	}
}
