// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"slices"
	"time"
)

// StartingSyncWait and StartingSnapSeeksBeyond are how long a sync point waits for the players
// (D250 §3), and how far a seek must jump to be snapped to a keyframe (D285 §1), before a device has
// brought the settings', together_sync_wait_seconds and together_snap_seeks_beyond_seconds (Tuning):
// their starting values. This package keeps nothing past the process and reads no settings, so they
// are copies, which internal/core/api's TestTogether_TheStartingWaitsAreTheSettingsStarts holds equal.
const (
	StartingSyncWait        = 20 * time.Second
	StartingSnapSeeksBeyond = 30 * time.Second
)

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
		current.transport(now, sender, message, refuse)
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
func (current *session) transport(now time.Time, sender *member, message Inbound, refuse func(code string)) {
	switch message.Command {
	case CommandPlay:
		current.waitForEveryone()
		current.change(now, GroupWaiting, EventPlayed, sender.id)
		current.settle(now)
	case CommandPause:
		current.change(now, GroupPaused, EventPaused, sender.id)
	case CommandSeek:
		if message.BasedOnSeq < current.group.lastSeek {
			refuse(codeStale)
			return
		}
		current.seek(now, sender, current.seekTarget(now, message))
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

// waitForEveryone makes a sync point wait for every player again, those catching up alone too
// (D250 §2): a film put on, the host's play after a pause, and the host's seek.
func (current *session) waitForEveryone() {
	for _, each := range current.members {
		for _, attached := range each.devices {
			attached.isCatchingUp = false
		}
	}
}

// changeItem puts fileID on for everyone, from position, as the host played it: the group waits
// until every player that has been ready loads it, those catching up too, then plays (D246 §5,
// D247 §4). The host's player that put it on is waited for though it has not yet once said it is
// ready: the host's player page connects as it opens, and the host chose the film (the owner's
// test, 2026-10-05, where the group played on as the host's player still loaded).
func (current *session) changeItem(now time.Time, sender *member, from *device, fileID int64, position time.Duration) {
	current.waitForEveryone()
	if from.role.plays() && !from.hasBeenReady {
		from.hasBeenReady, from.buffering = true, true
	}
	current.group.fileID, current.group.position, current.group.rate = fileID, position, 1
	current.change(now, GroupWaiting, EventChangedItem, sender.id)
	current.settle(now)
}

// seekTarget is where a seek takes the group: where it was asked, or, for a jump of more than the
// session's snapSeeksBeyond whose keyframe is known, the run-up after the keyframe at or before it. Every
// player then loads from a chunk's start, the encoded qualities' too, and none needs a second chunk
// for what lies past where it aimed; a run-up runs on what is loaded (D285 §1). A jump along the
// timeline is imprecise, the owner said, and no one would notice it land a little early.
func (current *session) seekTarget(now time.Time, message Inbound) time.Duration {
	asked := time.Duration(message.PositionMs) * time.Millisecond
	jump := asked - current.positionAt(now)
	if !message.isKeyframeKnown || (jump <= current.snapSeeksBeyond && jump >= -current.snapSeeksBeyond) {
		return asked
	}
	return time.Duration(message.keyframeMs)*time.Millisecond + current.runUp
}

// seek moves the group to position; a playing group waits there until every player is ready. Each
// player is ready again only once it says so from there: what it said before was of the place left,
// and its loading the new one is no stall. One catching up is waited for again, as for a new film.
func (current *session) seek(now time.Time, sender *member, position time.Duration) {
	for _, each := range current.members {
		for _, attached := range each.devices {
			if attached.role.plays() && attached.hasBeenReady {
				attached.buffering, attached.isCatchingUp = true, false
			}
		}
	}
	state := current.group.state
	if state == GroupPlaying {
		state = GroupWaiting
	}
	from := current.positionAt(now)
	current.group.position, current.group.at = position, now
	current.announce(now, state, EventSeeked, sender.id, from)
	current.group.lastSeek = current.group.seq
	current.settle(now)
}

// stalled takes a player's stall: the group never stops for one member (D250 §2). While it plays,
// it plays on, and the player catches up alone, its member named to everyone. When no other player
// plays along, the group waits for it instead, as at a sync point: playing on would play to no one,
// and the one watching would jump ahead to catch up, missing what they had not seen (D250 §2's
// amendment of 2026-10-12). Before the group starts moving no player stalls: one loading then is
// starting, as one running up to the start loads from before it (D282), and taken for a stall, a
// whole session's run-ups turned its start into waits and starts, over and over (the browser tier,
// 2026-10-12).
func (current *session) stalled(now time.Time, who *member, stalledDevice *device) {
	if current.group.state != GroupPlaying || stalledDevice.isCatchingUp || now.Before(current.group.at) {
		return
	}
	if !current.isAnyPlayingAlong() {
		current.change(now, GroupWaiting, EventWaitingFor, who.id)
		return
	}
	stalledDevice.isCatchingUp = true
	current.change(now, GroupPlaying, EventContinuedWithout, who.id)
}

// dropped takes a player's leaving: a playing group it was the last to play along with waits, as
// for a stall, until a player is ready again, its own back or another (D250 §2's amendment of
// 2026-10-12).
func (current *session) dropped(now time.Time, who *member, droppedDevice *device) {
	if current.group.state != GroupPlaying || !droppedDevice.playsAlong() || current.isAnyPlayingAlong() {
		return
	}
	current.change(now, GroupWaiting, EventWaitingFor, who.id)
}

// startOf is where and when a group that starts playing now starts moving: where it is, the start's
// lead from now (D268 §3). While a player along has not yet learned how late it starts, the lead is
// the run-up, which it plays through from before where the group starts, and a group at the film's
// first moments starts the run-up in, so there is film to run up through (D282): the owner, *"If the
// video is at the very start, losing a second or two isn't bad"*.
func (current *session) startOf(now time.Time) (time.Duration, time.Time) {
	position := current.positionAt(now)
	if current.runUp <= current.startLead || !current.isAnyStartUnlearned() {
		return position, now.Add(current.startLead)
	}
	return max(position, current.runUp), now.Add(current.runUp)
}

// isAnyStartUnlearned is whether a player along has not yet learned how late it starts.
func (current *session) isAnyStartUnlearned() bool {
	for _, each := range current.members {
		for _, attached := range each.devices {
			if attached.playsAlong() && !attached.knowsStartLag {
				return true
			}
		}
	}
	return false
}

// playsAlong is whether the device is a player the group plays along with: one that has once been
// ready, is not catching up alone, and is ready now.
func (each *device) playsAlong() bool {
	return each.role.plays() && each.hasBeenReady && !each.isCatchingUp && each.isReady()
}

// isAnyPlayingAlong is whether some player plays along with the group: whether it plays to anyone.
func (current *session) isAnyPlayingAlong() bool {
	for _, each := range current.members {
		for _, attached := range each.devices {
			if attached.playsAlong() {
				return true
			}
		}
	}
	return false
}

// endWait ends a sync point's wait once its time, the sync wait, is up and a player is ready: the
// players not yet ready catch up alone, and the group plays without them (D250 §3). While no player
// is ready the group waits on, so it never plays to no one. It runs at every message a device
// sends; a ready player sends one each second while the group waits, so the wait ends on time.
func (current *session) endWait(now time.Time) {
	if current.group.state != GroupWaiting || now.Before(current.group.waitUntil) {
		return
	}
	var late []*device
	isAnyReady := false
	without := current.group.byMember // the host whose command it was, if no player is late
	for _, each := range current.members {
		for _, attached := range each.devices {
			if !attached.role.plays() || !attached.hasBeenReady || attached.isCatchingUp {
				continue
			}
			if attached.isReady() {
				isAnyReady = true
			} else {
				late, without = append(late, attached), each.id
			}
		}
	}
	if !isAnyReady {
		return
	}
	for _, attached := range late {
		attached.isCatchingUp = true
	}
	current.change(now, GroupPlaying, EventContinuedWithout, without)
}

// settle starts a group waiting at a sync point playing once every device that plays along is
// ready. A playing group is never stopped by a player that is not (D250 §2). A device that has not
// yet once been ready, still choosing its speakers or loading, is not waited for (D246 §6). A group
// with no player ready waits on, so it never plays to no one: the one it waited for dropped, it
// plays once that player is back and ready (D250 §2's amendment of 2026-10-12).
func (current *session) settle(now time.Time) {
	if current.group.state != GroupWaiting || !current.isAnyPlayingAlong() {
		return
	}
	for _, each := range current.members {
		for _, attached := range each.devices {
			if attached.role.plays() && attached.hasBeenReady && !attached.isCatchingUp && !attached.isReady() {
				return
			}
		}
	}
	current.change(now, GroupPlaying, EventEveryoneReady, 0)
}

// change moves the group to state, naming the event and who caused it, and tells every device.
func (current *session) change(now time.Time, state GroupState, event EventKind, byMember int64) {
	current.announce(now, state, event, byMember, 0)
}

// announce is change, with where a seek jumped from. A group that starts playing starts moving
// startLead from now, so every player starts at once (D268 §3); one that plays on moves on as it was.
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
	if state == GroupWaiting {
		current.group.waitUntil = now.Add(current.syncWait)
	}
	current.group.seq++
	current.logChange(now)
	current.broadcast(Outbound{
		V: ProtocolVersion, Type: MessageGroup, Seq: current.group.seq, Group: current.groupView(now),
	})
	// Each player's status moves with the group: a new film loads, a seek waits, one is left behind.
	current.sendRoster()
}

// logChange logs the group's state as announce just set it: what plays, where, what changed it and
// by whom, and while it waits, until when and for which members' players, so a group that will not
// play says why (the owner's test, 2026-10-06, where it could only be guessed).
func (current *session) logChange(now time.Time) {
	attributes := []any{
		"sessionId", current.id, "state", string(current.group.state), "event", string(current.group.event),
		"byMemberId", current.group.byMember, "fileId", current.group.fileID,
		positionMsAttribute, current.positionAt(now).Milliseconds(),
	}
	if startsIn := current.startsIn(now); startsIn > 0 {
		attributes = append(attributes, "startsInMs", startsIn.Milliseconds())
	}
	if current.group.state == GroupWaiting {
		attributes = append(attributes, "waitingForMemberIds", current.membersNotReady(),
			"waitUntil", current.group.waitUntil.UTC().Format(time.RFC3339))
	}
	current.logger.Info("watch together: the group changed", attributes...)
}

// membersNotReady are the members, by id and in order, with a player the group waits for that is
// not ready: one that has once been ready and is not catching up alone.
func (current *session) membersNotReady() []int64 {
	var waited []int64
	for _, each := range current.members {
		for _, attached := range each.devices {
			if attached.role.plays() && attached.hasBeenReady && !attached.isCatchingUp && !attached.isReady() {
				waited = append(waited, each.id)
				break
			}
		}
	}
	slices.Sort(waited)
	return waited
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
