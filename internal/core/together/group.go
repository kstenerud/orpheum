// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// StartingSnapSeeksBeyond is how far a seek must jump to be snapped to a keyframe (D285 §1) before a
// device has brought the setting's, togetherSnapSeeksBeyondSeconds (Tuning): its starting value.
// This package keeps nothing past the process and reads no settings, so it is a copy, which
// internal/core/api's TestTogether_TheStartingWaitsAreTheSettingsStarts holds equal.
const StartingSnapSeeksBeyond = 30 * time.Second

// command applies a member's command in arrival order (cut-a §5.1): the host's, or a guest's their
// link allows (D271 §4); a seek based on a group another seek has changed since is refused as stale.
func (current *session) command(now time.Time, sender *member, from *device, message Inbound) {
	refuse := func(code string) {
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, BasedOnSeq: message.BasedOnSeq, Code: code})
	}
	if !sender.may(message.Command) {
		refuse(codeForbidden)
		return
	}
	isPlaying := current.group.fileID != 0
	switch message.Command {
	case CommandChangeItem:
		if message.FileID == 0 {
			refuse(codeNotFound) // one the host may not play was given as 0 (Handle)
			return
		}
		current.changeItem(now, sender, from, message.FileID, time.Duration(message.PositionMs)*time.Millisecond)
	case CommandNext, CommandPrevious:
		current.moveAlong(now, sender, from, message, refuse)
	case CommandStop:
		current.group.fileID, current.group.position = 0, 0
		current.filmChanged(now) // nothing on, no one leads (D374 §1)
		current.change(now, GroupIdle, EventStopped, sender.id)
	case CommandPlay, CommandPause, CommandSeek, CommandRate:
		if !isPlaying {
			refuse(codeInvalid)
			return
		}
		current.transport(now, sender, from, message, refuse)
	default:
		refuse(codeInvalid)
	}
}

// may is whether the member may give command: the host any, a guest what their link allows, a command
// carrying no privilege of its own (D268 §5, D271 §4).
func (each *member) may(command CommandKind) bool {
	rights := each.rightsHeld()
	switch command {
	case CommandPlay, CommandPause:
		return rights.CanPlayPause
	case CommandSeek:
		return rights.CanSeek
	case CommandNext, CommandPrevious:
		return rights.CanNext
	case CommandChangeItem, CommandStop, CommandRate:
		return each.isHost
	default:
		return false
	}
}

// rightsHeld is what the member may do to playback: every right for the host.
func (each *member) rightsHeld() Rights {
	if each.isHost {
		return everyRight()
	}
	return each.rights
}

// transport plays, pauses, seeks or changes the rate of what plays.
func (current *session) transport(
	now time.Time, sender *member, from *device, message Inbound, refuse func(code string),
) {
	switch message.Command {
	case CommandPlay:
		current.change(now, current.playingState(), EventPlayed, sender.id)
	case CommandPause:
		current.group.isHeldPlaying = false // a person's pause, which a player's return keeps (D375 §2)
		current.change(now, GroupPaused, EventPaused, sender.id)
	case CommandSeek:
		if message.BasedOnSeq < current.group.lastSeek {
			refuse(codeStale)
			return
		}
		current.seek(now, sender, current.seekTarget(now, from, message))
	case CommandRate:
		if message.Rate <= 0 || !current.everyPlayerSetsRate() {
			refuse(codeInvalid)
			return
		}
		current.group.position, current.group.at = current.positionAt(now), now
		current.group.rate = message.Rate
		current.change(now, current.group.state, EventChangedRate, sender.id)
	case CommandChangeItem, CommandStop, CommandNext, CommandPrevious:
	}
}

// moveAlong puts on the episode after or before the session's film that the hub found (Handle), as
// the host's Up next or ⏮ would (D471 §2): refused where nothing is on, as stale where another film
// was put on since it was found, and where none plays next or before.
func (current *session) moveAlong(
	now time.Time, sender *member, from *device, message Inbound, refuse func(code string),
) {
	switch {
	case current.group.fileID == 0:
		refuse(codeInvalid)
	case message.movedFrom != current.group.fileID:
		refuse(codeStale)
	case message.FileID == 0:
		refuse(codeNotFound)
	default:
		current.changeItem(now, sender, from, message.FileID, time.Duration(message.movedToMs)*time.Millisecond)
	}
}

// playingState is the state a group that is to play takes: Waiting while it has a leader, which it
// stands for, where it is, until the leader says it plays, as the leader plays as a device plays a
// film alone (D371 §1); with none, Playing at once, every player catching up (D371 §2).
func (current *session) playingState() GroupState {
	if current.leader() != nil {
		return GroupWaiting
	}
	return GroupPlaying
}

// changeItem puts fileID on for everyone, from position, as the host played it (D246 §5). A host's
// device that plays and puts it on puts on its own player's film (useLeadingPlayer, the one page
// that sends it): it is the screen that is the film, a contender while it loads, and leads (D371 §3,
// D374 §1), so a film put on still stands for its leader's loading, the start rule (D373 §4). The
// group stands where the film is put on until its leader plays, and no one else is waited for (D371
// §2). A leader that does not play the new film hands the lead on (filmChanged).
func (current *session) changeItem(now time.Time, sender *member, from *device, fileID int64, position time.Duration) {
	current.group.fileID, current.group.position, current.group.rate = fileID, position, 1
	if sender.isHost && from.role.plays() {
		from.playerFileID = fileID
		if current.contends(from) {
			from.hasContended = true
			current.lead(now, sender, from, leadPutOn)
		}
	}
	current.filmChanged(now)
	current.change(now, current.playingState(), EventChangedItem, sender.id)
}

// seekTarget is where a seek takes the group: where it was asked, or, for a jump of more than the
// session's snapSeeksBeyond whose keyframe is known, the keyframe at or before it. Every player then
// loads from a chunk's start, the encoded qualities' too, and none needs a second chunk for what lies
// past where it aimed (D285 §1; there is no run-up after it since D371). A jump along the timeline is
// imprecise, the owner said, and no one would notice it land a little early. The leader's own seek
// lands where it asked: the leader is already there, and is never sought to the group's place
// (D371 §1).
func (current *session) seekTarget(now time.Time, from *device, message Inbound) time.Duration {
	asked := time.Duration(message.PositionMs) * time.Millisecond
	jump := asked - current.positionAt(now)
	if from.id == current.leaderID || !message.isKeyframeKnown ||
		(jump <= current.snapSeeksBeyond && jump >= -current.snapSeeksBeyond) {
		return asked
	}
	return time.Duration(message.keyframeMs) * time.Millisecond
}

// seek moves the group to position. A playing group with a leader stands there until its leader
// plays from there (D371 §1); with none it plays on from there. Every player loads the new place, and
// says it buffers until it has.
func (current *session) seek(now time.Time, sender *member, position time.Duration) {
	for _, each := range current.members {
		for _, attached := range each.devices {
			if attached.role.plays() && attached.hasBeenReady {
				attached.buffering = true
			}
		}
	}
	state := current.group.state
	if state == GroupPlaying {
		state = current.playingState()
	}
	from := current.positionAt(now)
	current.group.position, current.group.at = position, now
	current.announce(now, state, EventSeeked, sender.id, from)
	current.group.lastSeek = current.group.seq
}

// followLeader places the group where its leader says it is, as the leader's state reaches the server:
// the group's position is the leader's, which nothing steers (D371 §1). A group to play (a film put
// on, a play, a seek) stands where its leader is while the leader loads (Waiting), and plays from
// where the leader plays once it does, at once, with no lead to start on; a playing leader is placed
// half its round trip on from where it said, and nothing is announced: the followers read where the
// group is from their next pong. A Watch Together group that plays never waits for its leader's
// stall while another contender plays the film: it plays on, and a leader stalled past
// stalledLeaderGrace hands on the lead (passStalledLead, D373 §1, §2); one that plays again within
// the grace catches up to the group, its states placing it only once in step (leaderCatchesUp,
// D374). The only contender's stall, a cast's receiver's among them, its group stands for, as a
// film played alone waits (D374 §3), and any group for a leader its browser paused. A paused group
// is the commands', and moves by none of the leader's states. It takes the place of a group that
// followed its only player and corrected it (D88), whose waits sought a Fire TV back at each one (the
// owner's casts, staging, 2026-10-28).
func (current *session) followLeader(ctx context.Context, now time.Time, who *member, from *device, message Inbound) {
	state := current.group.state
	if current.group.fileID == 0 || (state != GroupPlaying && state != GroupWaiting) {
		return
	}
	isPlaying := message.IsPlaying && !message.IsBuffering
	position := current.reportedPosition(from, message)
	switch {
	case state == GroupPlaying && !message.IsBuffering && !from.catchingUpSince.IsZero():
		current.leaderCatchesUp(ctx, now, who, from, message)
	case state == GroupPlaying && isPlaying:
		stepped := position - current.positionAt(now)
		current.group.position, current.group.at = position, now
		current.logger.DebugContext(ctx, "watch together: the group follows its leader",
			append(current.deviceAttributes(who, from), slogx.EventKey, eventFollowedLeader,
				positionMsAttribute, position.Milliseconds(), "steppedMs", stepped.Milliseconds())...)
	case state == GroupPlaying && !current.isOnlyContender(from):
		// no one's stall is waited for while another plays the film (D373 §1, D374 §3), nor the
		// leader's own pause, its browser's, not a person's: play or pause is the session's (D377 §5)
	case state == GroupPlaying:
		current.group.position, current.group.at = position, now
		current.change(now, GroupWaiting, EventWaitingFor, who.id)
	case isPlaying: // a group waiting for its leader, which plays
		current.group.position, current.group.at = position, now
		current.change(now, GroupPlaying, EventLeaderPlays, who.id)
	}
}

// leaderCatchesUp takes the state of a leader catching up to its group after a stall the group played
// on through (D374): out of step, behind the group or ahead of it, playing or waiting paused ahead
// for it as a follower does, it moves the group not at all, so the group never steps back for the
// stall and no one sees a stretch twice; once in step, within the drift a follower leaves alone
// (inStep), it leads from there, its states placing the group again, and the log says it caught up,
// how far off and after how long.
func (current *session) leaderCatchesUp(
	ctx context.Context, now time.Time, who *member, from *device, message Inbound,
) {
	offMs, isInStep := current.inStep(now, from, message)
	if !isInStep {
		return
	}
	catchingUpFor := now.Sub(from.catchingUpSince)
	from.catchingUpSince = time.Time{}
	position := current.reportedPosition(from, message)
	current.group.position, current.group.at = position, now
	current.logger.InfoContext(ctx, "watch together: the leader caught up to its group after its stall",
		append(current.deviceAttributes(who, from), slogx.EventKey, eventLeaderCaughtUp, "offGroupMs", offMs,
			"catchingUpMs", catchingUpFor.Milliseconds(), positionMsAttribute, position.Milliseconds())...)
}

// takeStall notes a player's stall while its Watch Together group plays, which the group does not
// wait for while another plays the film (D373 §1, D374 §3): when its state first says it buffers,
// which passStalledLead reads for a leader; and, as it plays on again, a line saying who stalled and
// for how long, so the log shows the group played on through it, a leader's saying how far behind the
// group it is as it catches up to it (leaderCatchesUp, D374). A group that stops playing, as one does
// for its only contender's stall, ends every stall (announce): what loads then is the start's to wait
// for (D373 §4).
func (current *session) takeStall(ctx context.Context, now time.Time, who *member, from *device, message Inbound) {
	switch {
	case message.IsBuffering && current.group.state == GroupPlaying && from.stalledSince.IsZero():
		from.stalledSince = now
	case !message.IsBuffering && !from.stalledSince.IsZero():
		isLeader := from.id == current.leaderID
		attributes := append(current.deviceAttributes(who, from), slogx.EventKey, eventStallNotWaitedFor,
			"isLeader", isLeader, "stalledMs", now.Sub(from.stalledSince).Milliseconds(),
			positionMsAttribute, message.PositionMs)
		if isLeader {
			from.catchingUpSince = now
			behind := current.positionAt(now) - current.reportedPosition(from, message)
			attributes = append(attributes, "catchesUp", true, "behindGroupMs", behind.Milliseconds())
		}
		current.logger.InfoContext(ctx, "watch together: the group played on through a player's stall",
			attributes...)
		from.stalledSince = time.Time{}
	}
}

// reportedPosition is where a device's state says its player is as the state reaches the server: a
// playing one half its round trip on from where it said.
func (current *session) reportedPosition(from *device, message Inbound) time.Duration {
	position := time.Duration(message.PositionMs) * time.Millisecond
	if message.IsPlaying && !message.IsBuffering {
		rttMs := from.rttMs
		if message.RTTMs > 0 {
			rttMs = message.RTTMs
		}
		position += time.Duration(float64(time.Duration(rttMs)*time.Millisecond/2) * current.group.rate)
	}
	return position
}

// change moves the group to state, naming the event and who caused it, and tells every device.
func (current *session) change(now time.Time, state GroupState, event EventKind, byMember int64) {
	current.announce(now, state, event, byMember, 0)
}

// announce is change, with where a seek jumped from. A group that starts playing moves on from where
// it stands at once, with no lead and no run-up: its leader plays already, or, with none, the
// players catch up as they can (D371 §1, §2, §6). One that plays on moves on as it was. A group held
// for want of a player stays held, to play on as one returns, only while it stays Paused (D375).
func (current *session) announce(
	now time.Time, state GroupState, event EventKind, byMember int64, seekedFrom time.Duration,
) {
	if state != GroupPlaying || current.group.state != GroupPlaying {
		current.group.position, current.group.at = current.positionAt(now), now
	}
	if state != GroupPlaying {
		current.endStalls()
	}
	if state != GroupPaused {
		current.group.isHeldPlaying = false
	}
	current.group.state = state
	current.group.event, current.group.byMember, current.group.seekedFrom = event, byMember, seekedFrom
	current.group.seq++
	current.group.changedAt = now
	current.logChange(now)
	current.broadcast(Outbound{
		V: ProtocolVersion, Type: MessageGroup, Seq: current.group.seq, Group: current.groupView(now),
	})
	// Each player's status moves with the group: a new film loads, a seek loads the new place.
	current.sendRoster()
}

// endStalls forgets every player's stall, and a leader's catching up after one, as the group stops
// playing: what it loads then is the start's, which waits for its leader (D373 §4), not a stall
// during playing, and the group goes on from where its leader is.
func (current *session) endStalls() {
	for _, each := range current.members {
		for _, attached := range each.devices {
			attached.stalledSince, attached.catchingUpSince = time.Time{}, time.Time{}
		}
	}
}

// logChange logs the group's state as announce just set it: what plays, where, what changed it and
// by whom, and who leads it, so a report can be read from the log (D371; the owner's test,
// 2026-10-06, where a group that would not play could only be guessed at).
func (current *session) logChange(now time.Time) {
	attributes := []any{
		slogx.EventKey, eventGroupChanged,
		sessionIDAttribute, current.id, "state", string(current.group.state), "event", string(current.group.event),
		"byMemberId", current.group.byMember, "fileId", current.group.fileID,
		positionMsAttribute, current.positionAt(now).Milliseconds(), leaderDeviceIDAttribute, current.leaderID,
	}
	if current.group.event == EventEveryoneLeft {
		attributes = append(attributes, "heldPlaying", current.group.isHeldPlaying) // D375 §1
	}
	current.logger.Info("watch together: the group changed", attributes...)
}

// positionAt is where the group is at now: moving at its rate while it plays.
func (current *session) positionAt(now time.Time) time.Duration {
	if current.group.state != GroupPlaying {
		return current.group.position
	}
	elapsed := now.Sub(current.group.at)
	return current.group.position + time.Duration(float64(elapsed)*current.group.rate)
}

// groupView is the group as a client is told it.
func (current *session) groupView(now time.Time) *GroupView {
	return &GroupView{
		State: current.group.state, PositionMs: current.positionAt(now).Milliseconds(), Rate: current.group.rate,
		FileID: current.group.fileID, Event: current.group.event, ByMemberID: current.group.byMember,
		SeekedFromMs: current.group.seekedFrom.Milliseconds(), IsHeldPlaying: current.group.isHeldPlaying,
	}
}

// everyPlayerSetsRate is whether every device that plays can set its rate: a group's rate stays
// 1.0 unless every one can honour it (watch-together.md, Sync).
func (current *session) everyPlayerSetsRate() bool {
	for _, each := range current.members {
		for _, attached := range each.devices {
			if attached.role.plays() && !attached.canSetRate {
				return false
			}
		}
	}
	return true
}
