// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"context"
	"time"
)

// Handle takes one message from an attached device. A change of what plays is checked against
// what the host may play, a next's or a previous's film found, whether anything plays after the
// film its leader's player ended, and a seek's keyframe found, before the hub is held, since they
// read the store. Each cast a next or a previous moved along with its session is told once the hub
// is let go (OnCastMovedAlong).
func (hub *Hub) Handle(ctx context.Context, attachment Attachment, message Inbound) {
	if message.Type == MessageCommand && message.Command == CommandChangeItem &&
		!hub.mayPlay(ctx, hub.hostOf(attachment.SessionID), message.FileID) {
		message.FileID = 0 // refused below, as one the host may not play
	}
	hub.findNeighbour(ctx, attachment, &message)
	hub.findNothingNext(ctx, attachment, &message)
	hub.findKeyframe(ctx, attachment, &message)
	movedAlong := hub.handleHeld(ctx, attachment, message)
	if hub.onCastMovedAlong != nil {
		for _, each := range movedAlong {
			hub.onCastMovedAlong(each)
		}
	}
}

// handleHeld takes the message as Handle says, holding the hub: the casts it moved along with their
// session, a next's or a previous's film put on (castsMoveAlong).
func (hub *Hub) handleHeld(ctx context.Context, attachment Attachment, message Inbound) []EndedCast {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	handled, found := hub.sessions[attachment.SessionID]
	if !found {
		return nil
	}
	sender, found := handled.members[attachment.MemberID]
	if !found {
		return nil
	}
	from, found := sender.devices[attachment.DeviceID]
	if !found {
		return nil
	}
	now := hub.clock.Now()
	hub.heardFrom(sender, from, now)
	switch message.Type {
	case MessagePing:
		// The pong says where the group is as it answers, so a follower can place the group by the
		// pong whose round trip it measured shortest, not by a message whose wait it cannot know (D264).
		from.send(Outbound{
			V: ProtocolVersion, Type: MessagePong, TMono: message.TMono, Seq: handled.group.seq,
			Group: handled.groupView(now),
		})
		handled.passStalledLead(now)
	case MessageState, MessageSuspend, MessageResume:
		handled.takeState(ctx, now, sender, from, message)
		handled.passStalledLead(now)
	case MessageCommand:
		handled.touch(now)
		wasOn := handled.group.fileID
		handled.command(now, sender, from, message)
		return hub.castsMoveAlong(handled, wasOn, message)
	case MessageRTC:
		handled.relay(from, message)
	case MessageNote, MessageStanding, MessageSound, MessagePicture, MessagePlayback:
		handled.takeAside(ctx, now, sender, from, message)
	case MessageCastState:
		handled.takeCastState(ctx, sender, from, message.CastState)
	case MessageHello, MessageWelcome, MessagePong, MessageGroup, MessageRoster, MessageRejected, MessageEnded:
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, Code: codeInvalid})
	default:
		// A type this server does not know, from a page of another build: refused, so the sender is
		// not left waiting on an answer that never comes, and logged, so a report says why.
		handled.logger.DebugContext(ctx, "watch together: a message of an unknown type is refused",
			append(handled.deviceAttributes(sender, from), "type", string(message.Type))...)
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, Code: codeInvalid})
	}
	return nil
}

// takeState takes what a device says of itself, and everyone is told: whether its page is hidden
// and which film its player has, by which it contends for the lead or no longer does, a leader that
// no longer does handing it on (takeContention, D372 §5, D374 §1); a host's device in step taking
// the lead back from a guest's (D372 §3); the leader's state places the group (followLeader), and no
// one else's moves it, a follower's trouble its own to catch up on (D371 §2); no one's stall is
// waited for while another plays the film (takeStall, D373 §1, D374 §3); the leader's player at the
// end of a film nothing plays after stops the group (stopsAtItsLeadersEnd, D516). Its first word that
// even its lowest quality is beyond its link is logged.
func (current *session) takeState(ctx context.Context, now time.Time, who *member, from *device, message Inbound) {
	couldKeepUp := from.cannotKeepUp == nil
	wasContender := current.contends(from)
	current.logPlayer(ctx, now, who, from, message)
	from.takeState(message)
	if message.Type == MessageState {
		from.place = devicePlace{
			fileID: message.FileID, position: current.reportedPosition(from, message), at: now,
			isMoving: message.IsPlaying && !message.IsBuffering, isRemembered: message.RemembersPlace,
		}
		current.takeVisibility(who, from, message.IsPageHidden)
		current.takeRun(now, who, from, message)
		current.takeStall(ctx, now, who, from, message)
		current.takeContention(now, who, from, wasContender)
		current.takeLeadInStep(now, who, from, message)
	}
	switch {
	case current.stopsAtItsLeadersEnd(now, from, message):
	case message.Type == MessageState && from.id == current.leaderID:
		current.followLeader(ctx, now, who, from, message)
	}
	if couldKeepUp && from.cannotKeepUp != nil {
		current.logger.InfoContext(ctx, "watch together: a device cannot keep up even at its lowest quality",
			append(current.deviceAttributes(who, from),
				"linkKbps", from.cannotKeepUp.LinkKbps, "lowestKbps", from.cannotKeepUp.LowestKbps)...)
	}
	current.sendRosterIfChanged()
}

// relay passes voice signalling to the device it names, in the same session (D193 §4).
func (current *session) relay(from *device, message Inbound) {
	for _, each := range current.members {
		if to, found := each.devices[message.ToDeviceID]; found {
			to.send(Outbound{
				V: ProtocolVersion, Type: MessageRTC, FromDeviceID: from.id,
				SDP: message.SDP, Candidate: message.Candidate,
			})
			return
		}
	}
}
