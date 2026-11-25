// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"testing"
	"time"
)

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

// A cast whose castee says nothing for CastSilentAfter ends by its own deadline, the castee told
// receiverGone and the session going on; its remote saying something keeps nothing on, and a cast
// whose castee has not yet said hello waits for it (D555 §3).
func TestCast_ACastWhoseCasteeIsSilentForItsWaitEnds(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	waiting, err := stage.hub.OpenCast(alone, "")
	if err != nil {
		t.Fatal(err)
	}
	cast, err := stage.hub.OpenCast(stage.owner, "")
	if err != nil {
		t.Fatal(err)
	}
	casteeAt, castee := stage.attach(t, cast.ReceiverCredential, RoleVideo)
	remote, _ := stage.attach(t, cast.SenderCredential, RolePanel)
	stage.clock.Advance(10 * time.Second)
	stage.ready(t, casteeAt, false)
	stage.clock.Advance(CastSilentAfter / 2)
	stage.command(t, remote, CommandPause, 0)
	stage.clock.Advance(CastSilentAfter/2 - time.Second)
	if ended := stage.hub.EndSilentCasts(); len(ended) != 0 {
		t.Fatalf("a castee heard %s ago had its cast ended: %+v", CastSilentAfter-time.Second, ended)
	}
	stage.clock.Advance(time.Second)
	ended := stage.hub.EndSilentCasts()
	if len(ended) != 1 || ended[0] != (EndedCast{CastID: cast.CastID, UserID: stage.owner.UserID}) {
		t.Fatalf("a castee silent for %s ended %+v; want cast %d alone", CastSilentAfter, ended, cast.CastID)
	}
	if told := castee.last(MessageEnded).Reason; told != CastReceiverGone {
		t.Errorf("the silent castee was told %q; want %s", told, CastReceiverGone)
	}
	if _, err = stage.hub.Mine(stage.owner.UserID); err != nil {
		t.Errorf("the host's session after its cast ended silent: %v; want it going on", err)
	}
	if _, err = stage.hub.CastCredential(waiting.CastID, alone.UserID); err != nil {
		t.Errorf("a cast whose castee has not said hello: %v; want it waiting on", err)
	}
	stage.hub.CastHeard(waiting.CastID, alone.UserID)
	stage.clock.Advance(CastSilentAfter)
	if ended = stage.hub.EndSilentCasts(); len(ended) != 1 || ended[0].CastID != waiting.CastID {
		t.Errorf("a cast taken %s ago, its castee silent since, ended %+v; want it", CastSilentAfter, ended)
	}
}
