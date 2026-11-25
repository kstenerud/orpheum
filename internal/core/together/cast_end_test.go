// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import "testing"

// A cast ended, by its receiver's Stop casting here, its sender's Stop casting or its receiver gone,
// leaves the Watch Together session its person is in going on, the host's own device a member (D446
// §2), and takes its castee's part back: the castee is told why and let go, and the cast's
// credentials admit nothing more (D555 §2).
func TestCast_AnEndedCastLeavesTheWatchTogetherSessionGoingOn(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{CastTakenBack, CastStopped, CastReceiverGone} {
		stage := newStage(t)
		cast, err := stage.hub.OpenCast(stage.owner, "")
		if err != nil {
			t.Fatal(err)
		}
		if cast.TogetherSessionID != stage.opened.SessionID {
			t.Fatalf("the host's cast is in session %d; want their part in %d", cast.TogetherSessionID,
				stage.opened.SessionID)
		}
		_, castee := stage.attach(t, cast.ReceiverCredential, RoleVideo)
		if err = stage.hub.EndCast(cast.CastID, stage.owner.UserID, reason); err != nil {
			t.Fatal(err)
		}
		if sessionID, _, isMember := stage.hub.MemberOf(stage.opened.MembershipCredential); !isMember ||
			sessionID != stage.opened.SessionID {
			t.Errorf("after a cast ended %s, the host's credential is of %d (member %v); want %d",
				reason, sessionID, isMember, stage.opened.SessionID)
		}
		for _, credential := range []string{cast.ReceiverCredential, cast.SenderCredential} {
			if _, _, isMember := stage.hub.MemberOf(credential); isMember {
				t.Errorf("after a cast ended %s, its credential still admits; want it taken back", reason)
			}
		}
		if told := castee.last(MessageEnded).Reason; told != reason {
			t.Errorf("after a cast ended %s, its castee was told %q", reason, told)
		}
		if _, err = stage.hub.Mine(stage.owner.UserID); err != nil {
			t.Errorf("after a cast ended %s, the host's session: %v; want it going on", reason, err)
		}
	}
}
