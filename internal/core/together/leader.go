// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"cmp"
	"time"
)

// A session's leader is the screen that is the film (D371 §1). The host's devices are preferred
// (D372 §3, confirmed): among them D371's rule, the newest that plays as it attaches and the one
// that puts a film on. While none of the host's can lead, the lead passes down the line to the
// player present longest without a break, a guest's as readily as the host's (D372 §1), by the
// seniority the core stamped as it saw the device arrive. A leader gone (its connection closed or
// silent, its page hidden, its member removed) hands the lead on at once (§5); a stall does not.
// With no one left to lead, a Watch Together group that plays pauses where it stands (§7).

// The reasons a session's leader changes, as its log says them (D371, D372).
const (
	// A host's device that plays attaching, while the host's device leads or none
	// does, or while nothing is on: the newest leads (D371 §3).
	leadAttached = "attached"
	// A host's device putting a film on: the screen that is the film.
	leadPutOn = "putOn"
	// A device that can lead arriving, or shown again, while none leads (D372 §1).
	leadNoneLed = "noneLed"
	// A host's device that came back taking the lead back from a guest's once it
	// plays in step with the group, so the film never jumps for its return (D372 §3).
	leadHostInStep = "hostInStep"
	// The leader gone (GoneBecause): the host's newest other player leads, else the
	// most senior player, else none (D372 §1, §5).
	leadLeaderGone = "leaderGone"
)

// GoneBecause is why a device is gone, for the lead: its log says it with the lead's passing.
type GoneBecause string

// The ways a device goes.
const (
	// GoneClosed: its connection closed or lost.
	GoneClosed GoneBecause = "closed"
	// GoneSilent: its connection silent past the server's limit, and dropped for it.
	GoneSilent GoneBecause = "silent"
	// Its page hidden by its browser, its film paused: a locked phone (D372 §5).
	goneHidden GoneBecause = "hidden"
	// Its member out of the session: removed by the host, left, or their place taken.
	goneRemoved GoneBecause = "removed"
)

// seniorityLayout is how a device's seniority is logged: the core's clock to the microsecond.
const seniorityLayout = "2006-01-02T15:04:05.000000Z07:00"

// canLead is whether the device may lead: it plays the film, on a page its browser shows (D372 §5,
// §6). Its page elsewhere, it is not attached at all.
func (each *device) canLead() bool {
	return each.role.plays() && !each.isPageHidden
}

// leader is the session's leader, the device that is the film (D371 §1); nil while none leads.
func (current *session) leader() *device {
	_, leading := current.leaderAndMember()
	return leading
}

// leaderAndMember is the session's leader and its member; nils while none leads.
func (current *session) leaderAndMember() (*member, *device) {
	if current.leaderID == 0 {
		return nil, nil
	}
	for _, each := range current.members {
		if attached, found := each.devices[current.leaderID]; found {
			return each, attached
		}
	}
	return nil, nil
}

// lead makes the device the session's leader, logging who leads now, why, and its seniority, so the
// log alone explains each change (D372); extra says more of why.
func (current *session) lead(now time.Time, who *member, leading *device, why string, extra ...any) {
	if leading.id == current.leaderID {
		return
	}
	previous := current.leaderID
	current.leaderID = leading.id
	attributes := append([]any{
		logEvent, eventLeaderChanged, sessionIDAttribute, current.id,
		leaderDeviceIDAttribute, leading.id, "leaderMemberId", who.id, "why", why,
		"leaderIsHost", who.isHost, "previousLeaderDeviceId", previous,
		"leaderSeniority", leading.arrivedAt.Format(seniorityLayout),
		"leaderPresentMs", now.Sub(leading.arrivedAt).Milliseconds(),
	}, extra...)
	current.logger.Info("watch together: the session's leader changed", attributes...)
}

// arrived takes a device that can lead as it arrives, isAttaching, or is shown again: it leads when
// none does; a host's leads at once when it attaches and the host's leads already (D371 §3), or
// when nothing is on; a host's coming back while a guest's leads a film follows until it plays in
// step (takeLeadInStep), so the film never jumps for its return (D372 §3).
func (current *session) arrived(now time.Time, who *member, from *device, isAttaching bool) {
	if !from.canLead() {
		return
	}
	leaderMember, leading := current.leaderAndMember()
	switch {
	case leading == nil:
		why := leadNoneLed
		if who.isHost {
			why = leadAttached
		}
		current.lead(now, who, from, why)
	case who.isHost && isAttaching && leaderMember.isHost:
		current.lead(now, who, from, leadAttached) // the newest of the host's players leads
	case who.isHost && !leaderMember.isHost && current.group.fileID == 0:
		current.lead(now, who, from, leadAttached) // nothing on, nothing to jump
	}
}

// takeLeadInStep hands the lead back to a host's device a guest's leads in place of, once its state
// says it plays in step with the group (D372 §3).
func (current *session) takeLeadInStep(now time.Time, who *member, from *device, message Inbound) {
	if !who.isHost || !from.canLead() || from.id == current.leaderID {
		return
	}
	leaderMember, leading := current.leaderAndMember()
	if leading != nil && leaderMember.isHost {
		return
	}
	if offMs, isInStep := current.inStep(now, from, message); isInStep {
		current.lead(now, who, from, leadHostInStep, "offGroupMs", offMs)
	}
}

// inStep is how far a device's state says its player is off the group, and whether that is in
// step: ready, playing as the group plays or held as it stands, within the drift a follower leaves
// alone (together_drift_left_alone_ms, D250 §1), the follower's own threshold for needing no
// correction.
func (current *session) inStep(now time.Time, from *device, message Inbound) (int64, bool) {
	if current.group.fileID == 0 || message.Buffering || !from.hasBeenReady ||
		message.Playing != (current.group.state == GroupPlaying) {
		return 0, false
	}
	off := current.reportedPosition(from, message) - current.positionAt(now)
	if off < 0 {
		off = -off
	}
	return off.Milliseconds(), off <= current.driftLeftAlone
}

// leaderGone hands the lead on at once from a leader gone, because: to the host's newest other
// device that can lead (D371 §3), else to the most senior player (D372 §1); with none, a Watch
// Together group that plays or stands for its leader pauses where it stands (§7). A cast is a
// session of one, its receiver's return taking up where it stood, and is not paused.
func (current *session) leaderGone(now time.Time, wasHost bool, because GoneBecause) {
	previous := current.leaderID
	gone := []any{"goneBecause", string(because), "goneLeaderWasHost", wasHost}
	// The leader gone is detached, removed or hidden, so it is never the next.
	if who, next := current.nextLeader(); next != nil {
		current.lead(now, who, next, leadLeaderGone, gone...)
		return
	}
	current.leaderID = 0
	current.logger.Info("watch together: the session's leader changed", append([]any{
		logEvent, eventLeaderChanged, sessionIDAttribute, current.id, leaderDeviceIDAttribute, 0, "why", leadLeaderGone,
		"previousLeaderDeviceId", previous,
	}, gone...)...)
	state := current.group.state
	if !current.isCast && current.group.fileID != 0 && (state == GroupPlaying || state == GroupWaiting) {
		current.change(now, GroupPaused, EventEveryoneLeft, 0)
	}
}

// nextLeader is who leads once the leader is gone: the host's newest device that can lead, else
// the device that can lead present longest, ties to the lower id; nils for none.
func (current *session) nextLeader() (*member, *device) {
	var hostMember, seniorMember *member
	var hostNewest, senior *device
	for _, each := range current.members {
		for _, attached := range each.devices {
			if !attached.canLead() {
				continue
			}
			if each.isHost && (hostNewest == nil || attached.id > hostNewest.id) {
				hostMember, hostNewest = each, attached
			}
			if senior == nil || isSenior(attached, senior) {
				seniorMember, senior = each, attached
			}
		}
	}
	if hostNewest != nil {
		return hostMember, hostNewest
	}
	return seniorMember, senior
}

// isSenior is whether one has been present longer than other: arrived earlier, or, arrived the same
// microsecond, has the lower id (D372 §1).
func isSenior(one, other *device) bool {
	if order := one.arrivedAt.Compare(other.arrivedAt); order != 0 {
		return order < 0
	}
	return cmp.Less(one.id, other.id)
}

// leaderStillHere hands the lead on when the leader is no longer attached: its member removed, or
// its device detached.
func (current *session) leaderStillHere(now time.Time, wasHost bool, because GoneBecause) {
	if current.leaderID != 0 && current.leader() == nil {
		current.leaderGone(now, wasHost, because)
	}
}

// takeVisibility takes a device's word that its page is hidden, its film paused by its browser, or
// shown again: hidden, it is gone for the lead, which passes at once (D372 §5); shown again, its
// seniority starts anew, now, and it may lead as a device arriving would (§2, §3).
func (current *session) takeVisibility(now time.Time, who *member, from *device, isHidden bool) {
	if isHidden == from.isPageHidden {
		return
	}
	from.isPageHidden = isHidden
	current.logger.Info("watch together: a device's page was hidden or shown",
		append(current.deviceAttributes(who, from), "isPageHidden", isHidden)...)
	if isHidden {
		if from.id == current.leaderID {
			current.leaderGone(now, who.isHost, goneHidden)
		}
		return
	}
	from.arrivedAt = seniorityAt(now)
	current.arrived(now, who, from, false)
}

// seniorityAt is a device's seniority arriving at now: the core's clock at microsecond resolution
// (D372 §1).
func seniorityAt(now time.Time) time.Time {
	return now.Truncate(time.Microsecond)
}
