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
	if fileID := hub.playedOf(attachment.SessionID).FileID; fileID != 0 {
		message.keyframeMs, message.isKeyframeKnown = hub.keyframeAtOrBefore(ctx, fileID, message.PositionMs)
	}
}

// findNeighbour finds, for a next or a previous, the film after or before the session's in its
// host's ordering, as the title it plays as, one the host may play, and where the host is partway
// through it; FileID stays 0 where there is none, which the command refuses.
func (hub *Hub) findNeighbour(ctx context.Context, attachment Attachment, message *Inbound) {
	isNext := message.Command == CommandNext
	if message.Type != MessageCommand || (!isNext && message.Command != CommandPrevious) {
		return
	}
	message.FileID, message.ItemID = 0, 0 // a next or a previous names no film of its own
	from := hub.playedOf(attachment.SessionID)
	message.movedFrom = from.FileID
	if message.movedFrom == 0 {
		return
	}
	if neighbour, positionMs, found := hub.neighbourOf(ctx, attachment.SessionID, from, isNext); found {
		message.FileID, message.ItemID, message.movedToMs = neighbour.FileID, neighbour.ItemID, positionMs
	}
}

// neighbourOf is the film after from (isNext) or before it in the host of sessionID's ordering, as
// the title from plays as, one the host may play, and where the host is partway through it; false
// where there is none, or no ordering is known (MovesAlongBy).
func (hub *Hub) neighbourOf(
	ctx context.Context, sessionID int64, from Played, isNext bool,
) (neighbour Played, positionMs int64, found bool) {
	if hub.neighbour == nil {
		return Played{}, 0, false
	}
	host := hub.hostOf(sessionID)
	neighbour, positionMs, found = hub.neighbour(ctx, host, from, isNext)
	if !found || !hub.mayPlay(ctx, host, neighbour.FileID) {
		return Played{}, 0, false
	}
	return neighbour, positionMs, true
}
