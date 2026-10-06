// SPDX-License-Identifier: GPL-3.0-or-later

package together

import (
	"slices"
	"time"
)

// startingSyncWait is how long a sync point waits for the players before a device has brought the
// setting's, together_sync_wait_seconds: its starting value (D250 §3).
const startingSyncWait = 20 * time.Second

// command applies the host's command in arrival order (cut-a §5.1): in Cut A only the host controls
// playback (D246 §8); a seek based on a group another seek has changed since is refused as stale.
func (current *session) command(now time.Time, sender *member, from *device, message Inbound) {
	refuse := func(code string) {
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, BasedOnSeq: message.BasedOnSeq, Code: code})
	}
	if !sender.isHost {
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
		current.seek(now, sender, time.Duration(message.PositionMs)*time.Millisecond)
	case CommandRate:
		if message.Rate <= 0 || !current.everyPlayerSetsRate() {
			refuse(codeInvalid)
			return
		}
		current.group.position, current.group.at = current.positionAt(now), now
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
// it plays on, and the player catches up alone, its member named to everyone.
func (current *session) stalled(now time.Time, who *member, stalledDevice *device) {
	if current.group.state != GroupPlaying || stalledDevice.isCatchingUp {
		return
	}
	stalledDevice.isCatchingUp = true
	current.change(now, GroupPlaying, EventContinuedWithout, who.id)
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
// yet once been ready, still choosing its speakers or loading, is not waited for (D246 §6); a group
// with no such device plays.
func (current *session) settle(now time.Time) {
	if current.group.state != GroupWaiting {
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

// announce is change, with where a seek jumped from.
func (current *session) announce(
	now time.Time, state GroupState, event EventKind, byMember int64, seekedFrom time.Duration,
) {
	current.group.position, current.group.at = current.positionAt(now), now
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
		SeekedFromMs: current.group.seekedFrom.Milliseconds(),
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
