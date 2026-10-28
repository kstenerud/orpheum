// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"context"
	"time"
)

// StartingSnapSeeksBeyond is how far a seek must jump to be snapped to a keyframe (D285 §1) before a
// device has brought the setting's, together_snap_seeks_beyond_seconds (Tuning): its starting value.
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
	case CommandStop:
		current.group.fileID, current.group.position = 0, 0
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
		if now.After(current.group.at) { // a start's lead is kept
			current.group.position, current.group.at = current.positionAt(now), now
		}
		current.group.rate = message.Rate
		current.change(now, current.group.state, EventChangedRate, sender.id)
	case CommandChangeItem, CommandStop:
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
// device that plays and puts it on is the screen that is the film, and leads (D371 §3): the group
// stands where the film is put on until its leader plays, and no one else is waited for (D371 §2).
func (current *session) changeItem(now time.Time, sender *member, from *device, fileID int64, position time.Duration) {
	if sender.isHost && from.role.plays() {
		current.lead(sender, from, leadPutOn)
	}
	current.group.fileID, current.group.position, current.group.rate = fileID, position, 1
	current.change(now, current.playingState(), EventChangedItem, sender.id)
}

// seekTarget is where a seek takes the group: where it was asked, or, for a jump of more than the
// session's snapSeeksBeyond whose keyframe is known, the run-up after the keyframe at or before it. Every
// player then loads from a chunk's start, the encoded qualities' too, and none needs a second chunk
// for what lies past where it aimed; a run-up runs on what is loaded (D285 §1). A jump along the
// timeline is imprecise, the owner said, and no one would notice it land a little early. The
// leader's own seek lands where it asked: the leader is already there, and is never sought to the
// group's place (D371 §1).
func (current *session) seekTarget(now time.Time, from *device, message Inbound) time.Duration {
	asked := time.Duration(message.PositionMs) * time.Millisecond
	jump := asked - current.positionAt(now)
	if from.id == current.leaderID || !message.isKeyframeKnown ||
		(jump <= current.snapSeeksBeyond && jump >= -current.snapSeeksBeyond) {
		return asked
	}
	return time.Duration(message.keyframeMs)*time.Millisecond + current.runUp
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
// the group's position is the leader's, which nothing steers (D371 §1). A group to play stands where
// its leader is while the leader loads, stalls or is paused by its browser (Waiting), and plays from
// where the leader plays once it does, at once, with no lead to start on; a playing leader is placed
// half its round trip on from where it said, and nothing is announced: the followers read where the
// group is from their next pong. A paused group is the commands', and moves by none of the leader's
// states. It takes the place of a group that followed its only player and corrected it (D88), whose
// waits sought a Fire TV back at each one (the owner's casts, staging, 2026-10-28).
func (current *session) followLeader(ctx context.Context, now time.Time, who *member, from *device, message Inbound) {
	state := current.group.state
	if current.group.fileID == 0 || (state != GroupPlaying && state != GroupWaiting) {
		return
	}
	isPlaying := message.Playing && !message.Buffering
	position := time.Duration(message.PositionMs) * time.Millisecond
	if isPlaying {
		rttMs := from.rttMs
		if message.RTTMs > 0 {
			rttMs = message.RTTMs
		}
		position += time.Duration(float64(time.Duration(rttMs)*time.Millisecond/2) * current.group.rate)
	}
	switch {
	case state == GroupPlaying && isPlaying:
		stepped := position - current.positionAt(now)
		current.group.position, current.group.at = position, now
		current.logger.DebugContext(ctx, "watch together: the group follows its leader",
			append(current.deviceAttributes(who, from), logEvent, eventFollowedLeader,
				positionMsAttribute, position.Milliseconds(), "steppedMs", stepped.Milliseconds())...)
	case state == GroupPlaying:
		current.group.position, current.group.at = position, now
		current.change(now, GroupWaiting, EventWaitingFor, who.id)
	case isPlaying: // a group waiting for its leader, which plays
		current.group.position, current.group.at = position, now
		current.change(now, GroupPlaying, EventLeaderPlays, who.id)
	}
}

// The reasons a session's leader changes, as its log says them.
const (
	// A host's device that plays attaching: the newest leads.
	leadAttached = "attached"
	// A host's device putting a film on: the screen that is the film.
	leadPutOn = "putOn"
	// The leader leaving: the host's newest other device that plays leads, or none.
	leadLeft = "leaderLeft"
)

// leader is the session's leader, the device that is the film (D371 §1); nil while none leads.
func (current *session) leader() *device {
	if current.leaderID == 0 {
		return nil
	}
	for _, each := range current.members {
		if attached, found := each.devices[current.leaderID]; found {
			return attached
		}
	}
	return nil
}

// lead makes the device the session's leader (D371 §3), logging who leads now and why.
func (current *session) lead(who *member, leading *device, why string) {
	if leading.id == current.leaderID {
		return
	}
	current.leaderID = leading.id
	current.logger.Info("watch together: the session's leader changed", logEvent, eventLeaderChanged,
		"sessionId", current.id, "leaderDeviceId", leading.id, "leaderMemberId", who.id, "why", why)
}

// leaderLeft takes the leader's leaving: the host's newest other device that plays leads (D371 §3),
// or none does, and a group that stood for the leader then plays on from where it stood for whoever
// plays still, as a session plays on as its host leaves the film (D364 §15). With no one to play to,
// it stands, until a player attaches and leads it.
func (current *session) leaderLeft(now time.Time, host *member) {
	current.leaderID = 0
	var newest *device
	for _, attached := range host.devices {
		if attached.role.plays() && (newest == nil || attached.id > newest.id) {
			newest = attached
		}
	}
	if newest != nil {
		current.lead(host, newest, leadLeft)
		return
	}
	current.logger.Info("watch together: the session's leader changed", logEvent, eventLeaderChanged,
		"sessionId", current.id, "leaderDeviceId", 0, "why", leadLeft)
	if current.group.state == GroupWaiting && current.isAnyPlayer() {
		current.change(now, GroupPlaying, EventLeaderLeft, host.id)
	}
}

// isAnyPlayer is whether some device attached plays the film.
func (current *session) isAnyPlayer() bool {
	for _, each := range current.members {
		if each.hasPlayer() {
			return true
		}
	}
	return false
}

// startOf is where and when a group that starts playing now starts moving. With a leader, that is
// where the group stands, now: the leader plays already (D371 §1). With none, the start's lead from
// now (D268 §3); while a player has not yet learned how late it starts, the lead is the run-up, which
// it plays through from before where the group starts, and a group at the film's first moments starts
// the run-up in, so there is film to run up through (D282).
func (current *session) startOf(now time.Time) (time.Duration, time.Time) {
	position := current.positionAt(now)
	if current.leader() != nil {
		return position, now
	}
	if current.runUp <= current.startLead || !current.isAnyStartUnlearned() {
		return position, now.Add(current.startLead)
	}
	return max(position, current.runUp), now.Add(current.runUp)
}

// isAnyStartUnlearned is whether a player has not yet learned how late it starts.
func (current *session) isAnyStartUnlearned() bool {
	for _, each := range current.members {
		for _, attached := range each.devices {
			if attached.role.plays() && attached.hasBeenReady && !attached.knowsStartLag {
				return true
			}
		}
	}
	return false
}

// change moves the group to state, naming the event and who caused it, and tells every device.
func (current *session) change(now time.Time, state GroupState, event EventKind, byMember int64) {
	current.announce(now, state, event, byMember, 0)
}

// announce is change, with where a seek jumped from. A group that starts playing starts moving as
// startOf says; one that plays on moves on as it was.
func (current *session) announce(
	now time.Time, state GroupState, event EventKind, byMember int64, seekedFrom time.Duration,
) {
	switch {
	case state == GroupPlaying && current.group.state != GroupPlaying:
		current.group.position, current.group.at = current.startOf(now)
	case state != GroupPlaying || current.group.state != GroupPlaying:
		current.group.position, current.group.at = current.positionAt(now), now
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

// logChange logs the group's state as announce just set it: what plays, where, what changed it and
// by whom, and who leads it, so a report can be read from the log (D371; the owner's test,
// 2026-10-06, where a group that would not play could only be guessed at).
func (current *session) logChange(now time.Time) {
	attributes := []any{
		logEvent, eventGroupChanged,
		"sessionId", current.id, "state", string(current.group.state), "event", string(current.group.event),
		"byMemberId", current.group.byMember, "fileId", current.group.fileID,
		positionMsAttribute, current.positionAt(now).Milliseconds(), "leaderDeviceId", current.leaderID,
	}
	if startsIn := current.startsIn(now); startsIn > 0 {
		attributes = append(attributes, "startsInMs", startsIn.Milliseconds())
	}
	current.logger.Info("watch together: the group changed", attributes...)
}

// positionAt is where the group is at now: moving at its rate while it plays, once its start's
// lead is over.
func (current *session) positionAt(now time.Time) time.Duration {
	if current.group.state != GroupPlaying || now.Before(current.group.at) {
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
		SeekedFromMs: current.group.seekedFrom.Milliseconds(), StartsInMs: current.startsIn(now).Milliseconds(),
	}
}

// startsIn is how long after now a playing group starts moving: what is left of its start's lead.
func (current *session) startsIn(now time.Time) time.Duration {
	if current.group.state != GroupPlaying || !now.Before(current.group.at) {
		return 0
	}
	return current.group.at.Sub(now)
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
