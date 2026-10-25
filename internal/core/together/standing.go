// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import "context"

// status is where the device stands with its group (DeviceStatus), isPlaying whether the session
// plays a film: what its page waits on first, then its player's state.
func (each *device) status(isPlaying bool) DeviceStatus {
	switch {
	case !each.role.plays():
		return ""
	case each.standing == StandingNeedsTap:
		return StatusNeedsTap
	case !isPlaying:
		return ""
	case !each.hasBeenReady:
		return StatusLoading
	case each.isCatchingUp:
		return StatusCatchingUp
	case each.suspended != "":
		return StatusSuspended
	case each.buffering:
		return StatusBuffering
	}
	return StatusWatching
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
