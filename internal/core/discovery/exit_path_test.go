// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"testing"

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
