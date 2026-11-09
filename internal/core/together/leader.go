// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// A session's leader is the screen that is the film (D371 §1), chosen among its contenders alone:
// the devices whose player plays the session's film now, on a page its browser shows (D374 §1,
// contends). Who leads, and from where, is one procedure (D377). The host's contenders are preferred
// (D372 §3, D374 §2, D377 §2): among them D371's rule, the newest as it first contends and the one that
// puts a film on. While none of the host's contends, the lead passes down the line to the guest's
// contender whose current run started earliest (D377 §2): a run starts as the core hears the device
// playing or ready with the session's film, by the core's clock, and ends, the device to the back of
// the line, as it disconnects, its page is hidden, its player leaves the film, or it stays stalled or
// paused past stalledLeaderGrace (takeRun, endRun); connecting alone starts none. A leader gone (its
// connection closed or silent, its page hidden, its member removed, its player no longer playing the
// film) hands the lead on at once (D372 §5, D374 §1), and one stalled, or paused by its browser, past
// stalledLeaderGrace while its Watch Together group plays hands it on then, the group playing on (D373
// §2, D377 §5), unless it is the only contender, whose stall its group stands for (D374 §3). With no
// one left to lead, a Watch Together group is held where it stands, its clock still, keeping whether
// it played, and the first contender back leads it on as it was (D375). A device's own position becomes
// the group's only as it is chosen to lead a group no one leads, when it presents one; a handover from
// a live leader takes none, its successor in step (D377 §4, placeByNewLeader).

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
	// The leader gone (GoneBecause): the host's newest other contender leads, else the guest's
	// contender whose run started earliest, else none (D372 §5, D377 §2).
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
	// Its player, leading, paused by its browser, not its person, past stalledLeaderGrace while its
	// Watch Together group played, which no device's own pause stops (D377 §5).
	gonePaused GoneBecause = "paused"
	// Its player left the session's film: another put on, or its page left it (D377 §2). A run's end only.
	goneLeftFilm GoneBecause = "leftFilm"
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
// (hlsSegmentLengthSeconds' starting value), which a link that carries the film loads in less
// than the two seconds it plays for; and a stalled leader tells its state each second
// (checkDriftEveryMs, web/packages/sdk/src/together-follow-options.ts), so the core sees a stall up
// to a second late. Three seconds outlasts both, so such a stall never hands the lead back and forth.
// It is also no longer than a follower's drift before it seeks rather than nudges
// (togetherSeekDriftBeyondMs' starting value): a leader that plays again within the grace is no
// further behind its group than a follower nudges across as it catches up (D374).
const stalledLeaderGrace = 3 * time.Second

// runLayout is how the start of a device's run is logged: the core's clock to the microsecond.
const runLayout = "2006-01-02T15:04:05.000000Z07:00"

// takeRun starts or ends the device's run as its state says (D377 §2): playing or ready, not
// buffering, with the session's film on a page that shows, it starts one, now by the core's clock, if
// it has none; its page hidden, or its player on another film or none, ends it. A stall or a pause
// ends it only past the grace (passStalledLead, endStalledRuns); its connection closing, as it
// detaches (Detach). Each start and end is logged (runStarted, runEnded with why).
func (current *session) takeRun(now time.Time, who *member, from *device, message Inbound) {
	hasFilm := current.group.fileID != 0 && message.FileID == current.group.fileID
	switch {
	case message.IsPageHidden:
		current.endRun(who, from, string(goneHidden))
	case !hasFilm:
		current.endRun(who, from, string(goneLeftFilm))
	case !message.IsBuffering && from.runSince.IsZero():
		from.runSince = now.Truncate(time.Microsecond)
		current.logger.Info("watch together: a device's run began",
			append(current.deviceAttributes(who, from), slogx.EventKey, eventRunStarted,
				"runSince", from.runSince.Format(runLayout))...)
	}
}

// endRun ends the device's run, why, sending it to the back of the line for the lead (D377 §2); one
// with no run has none to end.
func (current *session) endRun(who *member, from *device, why string) {
	if from.runSince.IsZero() {
		return
	}
	from.runSince = time.Time{}
	current.logger.Info("watch together: a device's run ended",
		append(current.deviceAttributes(who, from), slogx.EventKey, eventRunEnded, "why", why)...)
}

// isAheadInLine is whether one is ahead of other in line for the lead (D377 §2): one with a run before
// one with none, the earlier run first, ties, and two with none, to the lower id.
func isAheadInLine(one, other *device) bool {
	if one.runSince.IsZero() != other.runSince.IsZero() {
		return !one.runSince.IsZero()
	}
	if order := one.runSince.Compare(other.runSince); order != 0 {
		return order < 0
	}
	return cmp.Less(one.id, other.id)
}

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
	current.leadsBecause = leadsBecause(why, extra)
	leading.catchingUpSince = time.Time{}
	run := []any{"leaderRunSince", "none"}
	if !leading.runSince.IsZero() {
		run = []any{
			"leaderRunSince", leading.runSince.Format(runLayout),
			"leaderRunMs", now.Sub(leading.runSince).Milliseconds(),
		}
	}
	attributes := append(append([]any{
		slogx.EventKey, eventLeaderChanged, sessionIDAttribute, current.id,
		leaderDeviceIDAttribute, leading.id, "leaderMemberId", who.id, "why", why,
		"leaderIsHost", who.isHost, "previousLeaderDeviceId", previous,
	}, run...), extra...)
	attributes = append(attributes, current.placeByNewLeader(now, leading, why, previous == 0)...)
	current.logger.Info("watch together: the session's leader changed", attributes...)
}

// Where the group's position comes from as a device takes the lead (D377 §4), as its leaderChanged line
// says it (positionFromAttribute).
const (
	positionFromAttribute = "positionFrom"
	// The new leader's own position, which it presented: chosen as no one led, it had one.
	positionFromLeader = "leader"
	// The core's: a handover from a live leader, a new leader with no position of its own, or a film
	// just put on where it stands.
	positionFromCore = "core"
)

// devicePlace is where a device's latest state put its player: at position as the state reached the
// core, at, moving on at the group's rate while it played (isMoving), in the film fileID; and whether
// the state said the player remembers that place (isRemembered, Inbound.RemembersPlace).
type devicePlace struct {
	fileID       int64
	position     time.Duration
	at           time.Time
	isMoving     bool
	isRemembered bool
}

// positionAt is where the place has its player at now, moving on at rate while it played.
func (place devicePlace) positionAt(now time.Time, rate float64) time.Duration {
	if !place.isMoving {
		return place.position
	}
	return place.position + time.Duration(float64(now.Sub(place.at))*rate)
}

// hasOwnPosition is whether the device has a position of its own to present as it is chosen to lead
// a group no one leads (D377 §4): its latest state has the session's film and says its player was
// playing it with its session on this page, as a follower in step or as its leader, before this
// connection or on it (Inbound.RemembersPlace: a device back from a dropped connection, its film still
// loaded, says so in its first state); and that state is no older than the group's last change, which
// may have moved the group past it. A page loaded afresh says it only once it has played along, so
// until then it has none, and the core's position stands.
func (current *session) hasOwnPosition(each *device) bool {
	place := each.place
	return place.isRemembered && current.group.fileID != 0 && place.fileID == current.group.fileID &&
		!place.at.Before(current.group.changedAt)
}

// placeByNewLeader is where the group stands as leading takes the lead, why: the one rule for every
// change of leader (D377 §4), whether a device came to contend, the leader went, stalled or paused
// past its grace, or the host's came back in step. Chosen while no one led (wasLeaderless), a device
// with a position of its own (hasOwnPosition) presents it, and the group stands there, the others
// taking it on; one with none leaves the group where the core has it. A handover from a live leader
// takes no position, its successor in step with it, and nor does a film put on, which stands where it
// was put on. A group held for want of a player goes on from there as its first contender back takes
// the lead (resumeOnReturn, D375). It returns, for the leaderChanged line, where the position came from
// (positionFromAttribute), the group's position now, and for a presented one how far the group stepped.
func (current *session) placeByNewLeader(now time.Time, leading *device, why string, wasLeaderless bool) []any {
	fromCore := func(position time.Duration) []any {
		return []any{positionFromAttribute, positionFromCore, positionMsAttribute, position.Milliseconds()}
	}
	if why == leadPutOn { // put on where it stands, its clock not yet started (changeItem)
		return fromCore(current.group.position)
	}
	if !wasLeaderless || current.group.state == GroupIdle || !current.hasOwnPosition(leading) {
		return fromCore(current.positionAt(now))
	}
	position := leading.place.positionAt(now, current.group.rate)
	stepped := position - current.positionAt(now)
	current.group.position, current.group.at = position, now
	return []any{
		positionFromAttribute, positionFromLeader, positionMsAttribute, position.Milliseconds(),
		"steppedMs", stepped.Milliseconds(),
	}
}

// leadsBecause is why a leader leads, as the roster says it: why, and the goneBecause extra names
// after a colon, when it names one.
func leadsBecause(why string, extra []any) string {
	for index := 0; index+1 < len(extra); index += 2 {
		if extra[index] == "goneBecause" {
			if because, isText := extra[index+1].(string); isText {
				return why + ":" + because
			}
		}
	}
	return why
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
		current.resumeOnReturn(now, who)
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
// alone (togetherDriftLeftAloneMs, D250 §1), the follower's own threshold for needing no
// correction.
func (current *session) inStep(now time.Time, from *device, message Inbound) (int64, bool) {
	if current.group.fileID == 0 || message.IsBuffering || !from.hasBeenReady ||
		message.IsPlaying != (current.group.state == GroupPlaying) {
		return 0, false
	}
	off := current.reportedPosition(from, message) - current.positionAt(now)
	if off < 0 {
		off = -off
	}
	return off.Milliseconds(), off <= current.driftLeftAlone
}

// leaderGone hands the lead on at once from a leader gone, because (handOn); with no contender left,
// a Watch Together group with a film on is held Paused where it stands, its clock stopped as the last
// contender went, keeping whether it was playing or standing to play (isHeldPlaying) or a person had
// paused it (D372 §7, D375 §1): the hold is no person's pause, and the first contender back carries it
// on (resumeOnReturn). A cast is a session of one, its receiver's return taking up where it stood, and
// is not held.
func (current *session) leaderGone(now time.Time, wasHost bool, because GoneBecause) {
	if current.handOn(now, wasHost, because) {
		return
	}
	state := current.group.state
	if current.isCast || current.group.fileID == 0 || state == GroupIdle {
		return
	}
	current.group.isHeldPlaying = state == GroupPlaying || state == GroupWaiting
	current.change(now, GroupPaused, EventEveryoneLeft, 0)
}

// resumeOnReturn carries on a group held for want of a player as the first contender back, who,
// takes the lead (D375 §2): one that was playing is to play again from where the group stands, its
// new leader's position if it presents one, else where it was held (placeByNewLeader, D377 §4), standing
// there for its leader, as a film put on does, until the leader plays (Waiting, then leaderPlays,
// D371 §1, D373 §4); one a person had paused stays paused there. No time limits the hold.
func (current *session) resumeOnReturn(now time.Time, who *member) {
	if !current.group.isHeldPlaying || current.group.state != GroupPaused {
		return
	}
	current.change(now, current.playingState(), EventResumedOnReturn, who.id)
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
	current.leadsBecause = ""
	current.logger.Info("watch together: the session's leader changed", append([]any{
		slogx.EventKey, eventLeaderChanged, sessionIDAttribute, current.id, leaderDeviceIDAttribute, 0,
		"why", leadLeaderGone, "previousLeaderDeviceId", previous,
	}, gone...)...)
	return false
}

// nextLeader is who leads once the leader is gone: of the devices whyNot passes over for nothing, the
// host's newest, else the first in line, its run the earliest (isAheadInLine, D377 §2); nils for none.
// It says each
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
			if senior == nil || isAheadInLine(attached, senior) {
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

// passStalledLead hands the lead on from a leader stalled, or paused by its browser, past
// stalledLeaderGrace while its Watch Together group plays (D373 §2, D377 §5): to the host's newest
// other contender that plays on, else the first in line, as from a leader gone; the group plays on, the
// leader's run ends, and a host's device takes the lead back once it plays in step again
// (takeLeadInStep, D373 §3). A contender stalled too is passed over; with no one else to lead, the
// leader keeps the lead and the group plays on. The only contender's stall, a cast's receiver's among
// them, its group stands for instead (followLeader, D374 §3), so its group does not play. Every other
// device stalled as long goes to the back of the line too (endStalledRuns).
func (current *session) passStalledLead(now time.Time) {
	if current.group.state != GroupPlaying {
		return
	}
	current.endStalledRuns(now)
	leaderMember, leading := current.leaderAndMember()
	if leading == nil {
		return
	}
	since, because := leading.stalledSince, goneStalled
	if since.IsZero() && !leading.playing && !leading.buffering && !leading.playerChangedAt.IsZero() {
		since, because = leading.playerChangedAt, gonePaused // its own pause, not a person's (D377 §5)
	}
	if since.IsZero() || now.Sub(since) < stalledLeaderGrace {
		return
	}
	who, next, passedOver := current.nextLeader(func(each *device) string {
		if why := current.whyPassedOver(each); why != "" {
			return why
		}
		if each.id == leading.id {
			return string(because) // paused, it says no stall
		}
		if each.buffering {
			return string(goneStalled)
		}
		return ""
	})
	if next == nil {
		return
	}
	current.endRun(leaderMember, leading, string(because))
	current.lead(now, who, next, leadLeaderGone, "goneBecause", string(because),
		"goneLeaderWasHost", leaderMember.isHost, "stalledMs", now.Sub(since).Milliseconds(), "passedOver", passedOver)
	current.sendRoster()
}

// endStalledRuns sends every device stalled past stalledLeaderGrace while its group plays to the back
// of the line for the lead, its run ended (D377 §2).
func (current *session) endStalledRuns(now time.Time) {
	for _, each := range current.members {
		for _, attached := range each.devices {
			if !attached.stalledSince.IsZero() && now.Sub(attached.stalledSince) >= stalledLeaderGrace {
				current.endRun(each, attached, string(goneStalled))
			}
		}
	}
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
// §5), its run ended (takeRun); shown again, it contends again as a device arriving would, its new run
// starting once it plays or is ready (D377 §2).
func (current *session) takeVisibility(who *member, from *device, isHidden bool) {
	if isHidden == from.isPageHidden {
		return
	}
	from.isPageHidden = isHidden
	current.logger.Info("watch together: a device's page was hidden or shown",
		append(current.deviceAttributes(who, from), "isPageHidden", isHidden)...)
}
