// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import "testing"

// A cast ended, by its receiver's Stop casting here, its sender's Stop casting or its receiver gone,
// leaves the Watch Together session its person is in going on: only the cast's own session of one
// ends, and the host's own device and the receiver's part stay members (D446 §2).
func TestCast_AnEndedCastLeavesTheWatchTogetherSessionGoingOn(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{CastTakenBack, CastStopped, CastReceiverGone} {
		stage := newStage(t)
		part, _, _ := stage.hub.TogetherPart(stage.owner.UserID, "")
		cast, err := stage.hub.OpenCast(stage.owner)
		if err != nil {
			t.Fatal(err)
		}
		if err = stage.hub.EndCast(cast.SessionID, stage.owner.UserID, reason); err != nil {
			t.Fatal(err)
		}
		for _, credential := range []string{stage.opened.MembershipCredential, part} {
			if sessionID, _, isMember := stage.hub.MemberOf(credential); !isMember ||
				sessionID != stage.opened.SessionID {
				t.Errorf("after a cast ended %s, a credential of the session is of %d (member %v); want %d",
					reason, sessionID, isMember, stage.opened.SessionID)
			}
		}
		if _, err = stage.hub.Mine(stage.owner.UserID); err != nil {
			t.Errorf("after a cast ended %s, the host's session: %v; want it going on", reason, err)
		}
	}
}
