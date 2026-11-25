// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import "context"

// status is where the device stands with its group (DeviceStatus), isPlaying whether the session
// plays a film and isLeader whether the device leads it: what its page waits on first, then its
// player's state. A follower that buffers catches up; the leader that does is the group's buffering
// (D371 §1, §2).
func (each *device) status(isPlaying, isLeader bool) DeviceStatus {
	switch {
	case !each.role.plays():
		return ""
	case each.standing == StandingNeedsTap:
		return StatusNeedsTap
	case !isPlaying:
		return ""
	case !each.hasBeenReady:
		return StatusLoading
	case each.suspended != "":
		return StatusSuspended
	case each.buffering && isLeader:
		return StatusBuffering
	case each.buffering:
		return StatusCatchingUp
	}
	return StatusWatching
}

// takeCastState keeps what state a cast's castee says its cast is in with the cast (D555 §4), logs
// each change, and tells everyone, the cast's remote among them; a state no castee says, or one said
// by a device of no cast, is refused.
func (current *session) takeCastState(ctx context.Context, who *member, from *device, state CastState) {
	if !state.isKnown() || from.cast == nil {
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, Code: codeInvalid})
		return
	}
	if state == from.cast.state && from.saysCastState {
		return
	}
	from.cast.state, from.saysCastState = state, true
	current.logger.InfoContext(ctx, "cast: its castee said what state the cast is in",
		append(current.deviceAttributes(who, from), "castId", from.cast.id, "castState", string(state))...)
	current.sendRoster()
}

// castState is what state the device's cast is in, for a device that said one, its castee; empty
// for every other device.
func (each *device) castState() CastState {
	if each.cast == nil || !each.saysCastState {
		return ""
	}
	return each.cast.state
}

// takeStanding keeps what a device's page says it waits on, logs each change, and tells everyone; a
// standing no page says is refused.
func (current *session) takeStanding(ctx context.Context, who *member, from *device, standing Standing) {
	if !standing.isKnown() {
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, Code: codeInvalid})
		return
	}
	if standing == from.standing {
		return
	}
	from.standing = standing
	current.logger.InfoContext(ctx, "watch together: a device's page said what it waits on",
		append(current.deviceAttributes(who, from), "standing", string(standing))...)
	current.sendRoster()
}
