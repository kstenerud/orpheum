// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"testing"
	"time"
)

// endStage is a session whose host's ordering neighbourOneAlong gives, nothing after film 20, its
// film played by the host's player and a guest's, the host's leading.
type endStage struct {
	sessionStage
	host, guest Attachment
	guestBox    *inbox
	// guestCredential is the guest's membership credential.
	guestCredential string
}

// filmEndStage is film onFilm on in an endStage.
func filmEndStage(t *testing.T, onFilm int64) endStage {
	t.Helper()
	stage := newStage(t)
	stage.hub.MovesAlongBy(neighbourOneAlong(stage.owner.UserID))
	host, _ := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	stage.command(t, host, CommandChangeItem, onFilm)
	stage.ready(t, host, false)
	stage.clock.Advance(time.Second)
	joined := stage.join(t)
	guest, guestBox := stage.attach(t, joined.MembershipCredential, RoleVideo)
	stage.ready(t, guest, false)
	return endStage{
		sessionStage: stage, host: host, guest: guest, guestBox: guestBox,
		guestCredential: joined.MembershipCredential,
	}
}

// atTheEnd is attachment's state that its player played the session's film to its end, paused there.
func (stage sessionStage) atTheEnd(t *testing.T, attachment Attachment) {
	t.Helper()
	stage.hub.Handle(t.Context(), attachment, Inbound{
		Type: MessageState, PositionMs: 2_400_000, RTTMs: 40, IsAtTheEnd: true,
		FileID: stage.filmOn(attachment.SessionID),
	})
}

// A group's film that ends on its leader's player with nothing next stops the group as the host's
// Stop does, for the host, each guest told so and sent back to what they had before the film: a
// guest's leading player's as the host's own, its group playing or paused by the end (D516 §1, §2).
// Where an episode plays next, its end stops nothing.
func TestHub_AFilmEndingOnItsLeadersPlayerWithNothingNextStopsTheGroup(t *testing.T) {
	t.Parallel()
	for _, isGuestLeading := range []bool{false, true} {
		stage := filmEndStage(t, 20)
		leader := stage.host
		if isGuestLeading {
			stage.hub.Detach(stage.host, GoneClosed) // the host left the player before the end
			leader = stage.guest
		} else {
			stage.command(t, stage.host, CommandPause, 0) // its end paused its group first
		}
		stage.atTheEnd(t, leader)
		group := stage.guestBox.last(MessageGroup).Group
		if group.State != GroupIdle || group.FileID != 0 || group.Event != EventStopped ||
			group.ByMemberID != stage.host.MemberID {
			t.Errorf("guest leading %v, the guest after its leader's end: %+v; want Idle, nothing on, "+
				"stopped by the host, member %d", isGuestLeading, group, stage.host.MemberID)
		}
		if viewer, isViewer := stage.hub.ViewerOf(stage.guestCredential); !isViewer || viewer.FileID != 0 {
			t.Errorf("guest leading %v, the viewer after the end = %+v (%v); want nothing to play",
				isGuestLeading, viewer, isViewer)
		}
	}
	stage := filmEndStage(t, 7) // film 8 plays after it
	stage.hub.Detach(stage.host, GoneClosed)
	stage.atTheEnd(t, stage.guest)
	if film := stage.filmOn(stage.opened.SessionID); film != 7 {
		t.Errorf("the film on after its leader's end with film 8 next: %d; want 7 still on", film)
	}
}

// A player that does not lead saying it played the group's film to its end changes nothing, though
// nothing plays after it: only the leader's end is the group's (D516 §1).
func TestHub_AFollowersEndOfTheFilmChangesNothing(t *testing.T) {
	t.Parallel()
	stage := filmEndStage(t, 20)
	groupsBefore := stage.guestBox.count(MessageGroup)
	stage.atTheEnd(t, stage.guest)
	if film := stage.filmOn(stage.opened.SessionID); film != 20 || stage.guestBox.count(MessageGroup) != groupsBefore {
		t.Errorf("after a follower's end: film %d on, %d group messages more; want film 20 on, none",
			film, stage.guestBox.count(MessageGroup)-groupsBefore)
	}
}
