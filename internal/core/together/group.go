// SPDX-License-Identifier: GPL-3.0-or-later

package together

import "time"

// How long a group waits for its players, and when it does not wait at all: a stalled player holds
// the group once, then catches up alone (D247 §4). Silo-Server's numbers, to be measured.
const (
	// The group waits this long for its players to load what the host put on, and for the host's
	// to load where the host jumped to (the owner's choice, 2026-10-05).
	firstLoadWait = 60 * time.Second
	// The group waits this long for a player that stalled, or for its players after a play or a seek.
	reloadWait = 10 * time.Second
	// For this long after a member's stall, another of theirs pauses no one.
	stallCooldown = 5 * time.Minute
	// For this long after the group waited for a stall, another stall pauses no one.
	groupWaitCooldown = time.Minute
)

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

// changeItem puts fileID on for everyone, from position, as the host played it: the group waits
// until every player that has been ready loads it, those catching up too, then plays (D246 §5,
// D247 §4). The host's player that put it on is waited for though it has not yet once said it is
// ready: the host's player page connects as it opens, and the host chose the film (the owner's
// test, 2026-10-05, where the group played on as the host's player still loaded).
func (current *session) changeItem(now time.Time, sender *member, from *device, fileID int64, position time.Duration) {
	for _, each := range current.members {
		for _, attached := range each.devices {
			attached.isCatchingUp = false // a new film is waited for by everyone
		}
	}
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

// stalled takes a player's stall: while the group plays, it pauses the group only if neither its
// member nor the group has stalled lately; otherwise the group plays on and the player catches up
// alone (D247 §4).
func (current *session) stalled(now time.Time, who *member, stalledDevice *device) {
	if current.group.state != GroupPlaying || stalledDevice.isCatchingUp {
		return
	}
	hasStalledLately := !who.lastStall.IsZero() && now.Sub(who.lastStall) < stallCooldown
	hasWaitedLately := !current.lastStallWait.IsZero() && now.Sub(current.lastStallWait) < groupWaitCooldown
	who.lastStall = now
	if hasStalledLately || hasWaitedLately {
		stalledDevice.isCatchingUp = true
		current.change(now, GroupPlaying, EventContinuedWithout, who.id)
		return
	}
	current.lastStallWait = now // settle makes the group wait for it
}

// endWait ends a Waiting group's wait once its time is up and a player is ready: the players not
// yet ready catch up alone, and the group plays on without them (D247 §4). While no player is
// ready, or the host's is not and its own longer time is not up, the group waits on. It runs at
// every message a device sends; a ready player sends one each second while the group waits, so the
// wait ends on time.
func (current *session) endWait(now time.Time) {
	if current.group.state != GroupWaiting || now.Before(current.group.waitUntil) {
		return
	}
	var late []*device
	isAnyReady, isHostWaitedFor := false, now.Before(current.group.hostWaitUntil)
	without := current.group.byMember // one that left, if no player is late
	for _, each := range current.members {
		for _, attached := range each.devices {
			if !attached.role.plays() || !attached.hasBeenReady || attached.isCatchingUp {
				continue
			}
			switch {
			case attached.isReady():
				isAnyReady = true
			case each.isHost && isHostWaitedFor:
				return
			default:
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

// settle starts a waiting group playing once every device that plays along is ready, or stops a
// playing one that has one not ready. A device that has not yet once been ready, still choosing
// its speakers or loading, is not waited for (D246 §6); a group with no such device plays.
func (current *session) settle(now time.Time) {
	allReady := true
	var waitingFor int64
	for _, each := range current.members {
		for _, attached := range each.devices {
			if !attached.role.plays() || !attached.hasBeenReady || attached.isCatchingUp {
				continue
			}
			if !attached.isReady() {
				allReady, waitingFor = false, each.id
			}
		}
	}
	switch {
	case current.group.state == GroupWaiting && allReady:
		current.change(now, GroupPlaying, EventEveryoneReady, 0)
	case current.group.state == GroupPlaying && !allReady:
		current.change(now, GroupWaiting, EventBuffering, waitingFor)
	}
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
		current.group.waitUntil = now.Add(reloadWait)
		if event == EventChangedItem {
			current.group.waitUntil = now.Add(firstLoadWait)
		}
		current.group.hostWaitUntil = current.group.waitUntil
		if event == EventSeeked {
			// The host chose where to go: the others wait for them as for a film put on.
			current.group.hostWaitUntil = now.Add(firstLoadWait)
		}
	}
	current.group.seq++
	current.broadcast(Outbound{
		V: ProtocolVersion, Type: MessageGroup, Seq: current.group.seq, Group: current.groupView(now),
	})
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
