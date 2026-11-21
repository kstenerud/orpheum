// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import "context"

// The hub's moves that ask outside it: a seek snapped to a keyframe (D285 §1), and a next or a
// previous finding the film it puts on (D471 §2).

// SnapSeeksBy has a long seek snapped back to the keyframe keyframeAtOrBefore finds (D285 §1).
func (hub *Hub) SnapSeeksBy(keyframeAtOrBefore KeyframeAtOrBefore) {
	hub.keyframeAtOrBefore = keyframeAtOrBefore
}

// MovesAlongBy has a next or a previous find the film it puts on by neighbour (D471 §2).
func (hub *Hub) MovesAlongBy(neighbour Neighbour) {
	hub.neighbour = neighbour
}

// findKeyframe finds, for a seek, the keyframe at or before where it asks, when the hub snaps seeks.
func (hub *Hub) findKeyframe(ctx context.Context, attachment Attachment, message *Inbound) {
	if message.Type != MessageCommand || message.Command != CommandSeek || hub.keyframeAtOrBefore == nil {
		return
	}
	if fileID := hub.fileOf(attachment.SessionID); fileID != 0 {
		message.keyframeMs, message.isKeyframeKnown = hub.keyframeAtOrBefore(ctx, fileID, message.PositionMs)
	}
}

// findNeighbour finds, for a next or a previous, the film after or before the session's in its
// host's ordering, one the host may play, and where the host is partway through it; FileID stays 0
// where there is none, which the command refuses.
func (hub *Hub) findNeighbour(ctx context.Context, attachment Attachment, message *Inbound) {
	isNext := message.Command == CommandNext
	if message.Type != MessageCommand || (!isNext && message.Command != CommandPrevious) {
		return
	}
	message.FileID = 0 // a next or a previous names no film of its own
	message.movedFrom = hub.fileOf(attachment.SessionID)
	if message.movedFrom == 0 {
		return
	}
	if neighbourID, positionMs, found := hub.neighbourOf(ctx, attachment.SessionID, message.movedFrom, isNext); found {
		message.FileID, message.movedToMs = neighbourID, positionMs
	}
}

// neighbourOf is the film after fileID (isNext) or before it in the host of sessionID's ordering, one
// the host may play, and where the host is partway through it; false where there is none, or no
// ordering is known (MovesAlongBy).
func (hub *Hub) neighbourOf(
	ctx context.Context, sessionID, fileID int64, isNext bool,
) (neighbourID, positionMs int64, found bool) {
	if hub.neighbour == nil {
		return 0, 0, false
	}
	host := hub.hostOf(sessionID)
	neighbourID, positionMs, found = hub.neighbour(ctx, host, fileID, isNext)
	if !found || !hub.mayPlay(ctx, host, neighbourID) {
		return 0, 0, false
	}
	return neighbourID, positionMs, true
}
