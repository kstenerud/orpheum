// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
)

// A session's leader is the screen that is the film (D371 §1), chosen among its contenders alone:
// the devices whose player plays the session's film now, on a page its browser shows (D374 §1,
// contends). The host's contenders are preferred (D372 §3, confirmed; D374 §2): among them D371's
// rule, the newest as it first contends and the one that puts a film on. While none of the host's
// contends, the lead passes down the line to the contender present longest without a break, a
// guest's as readily as the host's (D372 §1), by the seniority the core stamped as it saw the device
// arrive. A leader gone (its connection closed or silent, its page hidden, its member removed, its
// player no longer playing the film) hands the lead on at once (§5, D374 §1), and one stalled past
// stalledLeaderGrace while its Watch Together group plays hands it on then, the group playing on
// (D373 §2), unless it is the only contender, whose stall its group stands for (D374 §3). With no one
// left to lead, a Watch Together group that plays pauses where it stands (D372 §7).

// The reasons a session's leader changes, as its log says them (D371, D372).
const (
	// A host's contender arriving, while the host's device leads or none does: the newest leads
	// (D371 §3).
	leadAttached = "attached"
	// A host's device putting a film on: the screen that is the film.
	leadPutOn = "putOn"
	// A contender arriving, or shown again, while none leads (D372 §1).
	leadNoneLed = "noneLed"
	// A host's device that came back taking the lead back from a guest's once it
	// plays in step with the group, so the film never jumps for its return (D372 §3).
	leadHostInStep = "hostInStep"
	// The leader gone (GoneBecause): the host's newest other contender leads, else the
	// most senior contender, else none (D372 §1, §5).
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
	// Its player stalled past stalledLeaderGrace while its Watch Together group played (D373 §2).
	goneStalled GoneBecause = "stalled"
	// Its player has no film: gone, not yet loaded, or its film handed to a cast's receiver (D374 §1).
	goneNoFilm GoneBecause = "noFilm"
	// Its player has another film than the session's (D374 §1).
	goneOtherFilm GoneBecause = "otherFilm"
	// The session has no film on, so no one plays it (D374 §1).
	goneNothingOn GoneBecause = "nothingOn"
)

// passedOverPanel is why a panel, which plays nothing, never contends for the lead (D120).
const passedOverPanel = "panel"

// stalledLeaderGrace is how long a Watch Together leader's stall lasts, while its group plays, before
// the leader is gone for the lead (D373 §2). A justified constant, as the session socket's
// silenceLimit is, not a setting: two other values fix it rather than anyone's taste. A quality
// switch's or a short refill's stall waits on one chunk, two seconds of film
// (hls_segment_length_seconds' starting value), which a link that carries the film loads in less
// than the two seconds it plays for; and a stalled leader tells its state each second
// (checkDriftEveryMs, web/packages/sdk/src/together-follow-options.ts), so the core sees a stall up
// to a second late. Three seconds outlasts both, so such a stall never hands the lead back and forth.
// It is also no longer than a follower's drift before it seeks rather than nudges
// (together_seek_drift_beyond_ms' starting value): a leader that plays again within the grace is no
// further behind its group than a follower nudges across as it catches up (D374).
const stalledLeaderGrace = 3 * time.Second

// seniorityLayout is how a device's seniority is logged: the core's clock to the microsecond.
const seniorityLayout = "2006-01-02T15:04:05.000000Z07:00"

// contends is whether the device contends for the lead (D374 §1): every choice of leader is among
// the contenders alone.
func (current *session) contends(each *device) bool {
	return current.whyPassedOver(each) == ""
}

// whyPassedOver is why the device does not contend for the lead, "" when it does: a contender is a
// player on a page its browser shows (D372 §5, §6) whose player has the session's film, as its latest
// state says or as it put it on itself (D374 §1). Attached in a playing role is not enough: a device
// whose film went to a cast's receiver may keep its connection with no player, and its receiver
// contends in its place. Its page elsewhere, it is not attached at all.
func (current *session) whyPassedOver(each *device) string {
	switch {
	case !each.role.plays():
		return passedOverPanel
	case each.isPageHidden:
		return string(goneHidden)
	case current.group.fileID == 0:
		return string(goneNothingOn)
	case each.playerFileID == 0:
		return string(goneNoFilm)
	case each.playerFileID != current.group.fileID:
		return string(goneOtherFilm)
	}
	return ""
}

// isOnlyContender is whether the device is the session's one contender for the lead: its stall is
// then waited for, as a film played alone waits, there being no one else it would hold up (D374 §3).
func (current *session) isOnlyContender(from *device) bool {
	if !current.contends(from) {
		return false
	}
	for _, each := range current.members {
		for _, attached := range each.devices {
			if attached.id != from.id && current.contends(attached) {
				return false
			}
		}
	}
	return true
}

// takeContention takes a device's state as it bears on the lead (D374 §1): becoming a contender, its
// player having the session's film on a page that shows, it arrives for the lead (arrived), its first
// time since it attached as a device attaching; a leader that no longer contends, its page hidden,
// its player gone or showing another film, is gone, and the lead passes at once (D372 §5).
func (current *session) takeContention(now time.Time, who *member, from *device, wasContender bool) {
	isContender := current.contends(from)
	switch {
	case isContender && !wasContender:
		isFirst := !from.hasContended
		from.hasContended = true
		current.arrived(now, who, from, isFirst)
	case !isContender && from.id == current.leaderID:
		current.leaderGone(now, who.isHost, GoneBecause(current.whyPassedOver(from)))
	}
}

// filmChanged hands the lead on, telling the group nothing, from a leader that does not play the
// film the session now has on, or has nothing on (D374 §1): to a contender for it, or none, the group
// then playing it at once (D371 §2). A film the host's player puts on is that player's, which leads
// it (changeItem).
func (current *session) filmChanged(now time.Time) {
	leaderMember, leading := current.leaderAndMember()
	if leading != nil && !current.contends(leading) {
		current.handOn(now, leaderMember.isHost, GoneBecause(current.whyPassedOver(leading)))
	}
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
	leading.catchingUpSince = time.Time{}
	attributes := append([]any{
		logEvent, eventLeaderChanged, sessionIDAttribute, current.id,
		leaderDeviceIDAttribute, leading.id, "leaderMemberId", who.id, "why", why,
		"leaderIsHost", who.isHost, "previousLeaderDeviceId", previous,
		"leaderSeniority", leading.arrivedAt.Format(seniorityLayout),
		"leaderPresentMs", now.Sub(leading.arrivedAt).Milliseconds(),
	}, extra...)
	current.logger.Info("watch together: the session's leader changed", attributes...)
}

// arrived takes a device as it comes to contend for the lead, isAttaching its first time since it
// attached, or as it is shown again: it leads when none does; a host's leads at once, its first
// time, when the host's leads already (D371 §3); a host's coming back while a guest's leads follows
// until it plays in step (takeLeadInStep), so the film never jumps for its return (D372 §3). A
// contender plays the session's film, so a guest's never leads while nothing is on (D374 §1).
func (current *session) arrived(now time.Time, who *member, from *device, isAttaching bool) {
	if !current.contends(from) {
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
	}
}

// takeLeadInStep hands the lead back to a host's contender a guest's leads in place of, once its
// state says it plays in step with the group (D372 §3).
func (current *session) takeLeadInStep(now time.Time, who *member, from *device, message Inbound) {
	if !who.isHost || !current.contends(from) || from.id == current.leaderID {
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

// leaderGone hands the lead on at once from a leader gone, because (handOn); with no contender left,
// a Watch Together group that plays or stands for its leader pauses where it stands (D372 §7). A cast
// is a session of one, its receiver's return taking up where it stood, and is not paused.
func (current *session) leaderGone(now time.Time, wasHost bool, because GoneBecause) {
	if current.handOn(now, wasHost, because) {
		return
	}
	state := current.group.state
	if !current.isCast && current.group.fileID != 0 && (state == GroupPlaying || state == GroupWaiting) {
		current.change(now, GroupPaused, EventEveryoneLeft, 0)
	}
}

// handOn hands the lead on from a leader gone, because, to the host's newest other contender (D371
// §3), else the most senior contender (D372 §1), and reports whether one took it; with none, no one
// leads. The log says each player passed over, and why (D374 §1).
func (current *session) handOn(now time.Time, wasHost bool, because GoneBecause) bool {
	previous := current.leaderID
	// The leader gone is detached, removed, hidden or plays the film no more: it never contends.
	who, next, passedOver := current.nextLeader(current.whyPassedOver)
	gone := []any{"goneBecause", string(because), "goneLeaderWasHost", wasHost, "passedOver", passedOver}
	if next != nil {
		current.lead(now, who, next, leadLeaderGone, gone...)
		return true
	}
	current.leaderID = 0
	current.logger.Info("watch together: the session's leader changed", append([]any{
		logEvent, eventLeaderChanged, sessionIDAttribute, current.id, leaderDeviceIDAttribute, 0, "why", leadLeaderGone,
		"previousLeaderDeviceId", previous,
	}, gone...)...)
	return false
}

// nextLeader is who leads once the leader is gone: of the devices whyNot passes over for nothing, the
// host's newest, else the one present longest, ties to the lower id; nils for none. It says each
// player passed over and why, "<device id>:<why>" apart by spaces in device order, for the log (D374
// §1); a panel, never a player, goes unsaid.
func (current *session) nextLeader(whyNot func(*device) string) (*member, *device, string) {
	var hostMember, seniorMember *member
	var hostNewest, senior *device
	passedOver := map[int64]string{}
	for _, each := range current.members {
		for _, attached := range each.devices {
			if why := whyNot(attached); why != "" {
				if why != passedOverPanel {
					passedOver[attached.id] = why
				}
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
		return hostMember, hostNewest, passedOverSaid(passedOver)
	}
	return seniorMember, senior, passedOverSaid(passedOver)
}

// passedOverSaid is the players passed over for the lead, by device id, as the log says them:
// "<device id>:<why>", apart by spaces, in device order.
func passedOverSaid(passedOver map[int64]string) string {
	said := make([]string, 0, len(passedOver))
	for _, id := range slices.Sorted(maps.Keys(passedOver)) {
		said = append(said, strconv.FormatInt(id, 10)+":"+passedOver[id])
	}
	return strings.Join(said, " ")
}

// passStalledLead hands the lead on from a leader stalled past stalledLeaderGrace while its Watch
// Together group plays (D373 §2): to the host's newest other contender that plays on, else the most
// senior, as from a leader gone (D372 §1); the group plays on, and a host's stalled device takes the
// lead back once it plays in step again (takeLeadInStep, D373 §3). A contender stalled too is passed
// over; with no one else to lead, the stalled leader keeps the lead and the group plays on. The only
// contender's stall, a cast's receiver's among them, its group stands for instead (followLeader,
// D374 §3), so its group does not play.
func (current *session) passStalledLead(now time.Time) {
	if current.group.state != GroupPlaying {
		return
	}
	leaderMember, leading := current.leaderAndMember()
	if leading == nil || leading.stalledSince.IsZero() {
		return
	}
	stalledFor := now.Sub(leading.stalledSince)
	if stalledFor < stalledLeaderGrace {
		return
	}
	who, next, passedOver := current.nextLeader(func(each *device) string {
		if why := current.whyPassedOver(each); why != "" {
			return why
		}
		if each.buffering {
			return string(goneStalled)
		}
		return ""
	})
	if next == nil {
		return
	}
	current.lead(now, who, next, leadLeaderGone, "goneBecause", string(goneStalled),
		"goneLeaderWasHost", leaderMember.isHost, "stalledMs", stalledFor.Milliseconds(), "passedOver", passedOver)
	current.sendRoster()
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
// shown again: hidden, it contends no more, and a leader's lead passes at once (takeContention, D372
// §5); shown again, its seniority starts anew, now, and it contends again as a device arriving would
// (§2, §3).
func (current *session) takeVisibility(now time.Time, who *member, from *device, isHidden bool) {
	if isHidden == from.isPageHidden {
		return
	}
	from.isPageHidden = isHidden
	current.logger.Info("watch together: a device's page was hidden or shown",
		append(current.deviceAttributes(who, from), "isPageHidden", isHidden)...)
	if !isHidden {
		from.arrivedAt = seniorityAt(now)
	}
}

// seniorityAt is a device's seniority arriving at now: the core's clock at microsecond resolution
// (D372 §1).
func seniorityAt(now time.Time) time.Time {
	return now.Truncate(time.Microsecond)
}
