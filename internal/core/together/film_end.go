// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"context"
	"time"
)

// A film that ends on its group's leader's player with nothing after it stops the group, whoever
// leads (D516).

// findNothingNext finds, for the state of a session's leader whose player played the group's film
// to its end, whether nothing plays after it in the host's ordering, as a next would find it
// (neighbourOf), as the title the group plays it as: the group then stops (stopsAtItsLeadersEnd,
// D516). Another device's end is not looked up. A file filed both as a film and as an episode,
// played as the episode, plays on to the episode after it (B0 question 20).
func (hub *Hub) findNothingNext(ctx context.Context, attachment Attachment, message *Inbound) {
	if message.Type != MessageState || !message.IsAtTheEnd || !hub.isLeadersEnd(attachment, *message) {
		return
	}
	played := hub.playedOf(attachment.SessionID)
	if played.FileID != message.FileID {
		played = Played{FileID: message.FileID}
	}
	_, _, found := hub.neighbourOf(ctx, attachment.SessionID, played, true)
	message.isNothingNext = !found
}

// isLeadersEnd is whether message is the state of attachment's device, its session's leader, whose
// player played the group's film to its end (atItsLeadersEnd).
func (hub *Hub) isLeadersEnd(attachment Attachment, message Inbound) bool {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	held, found := hub.sessions[attachment.SessionID]
	return found && held.atItsLeadersEnd(attachment.DeviceID, message)
}

// stopsAtItsLeadersEnd stops a Watch Together group whose leader's player, host's or guest's, played
// its film to its end with nothing after it in the host's ordering, as the host's Stop does, for the
// host: the guests go back to what they had before the host put a film on (D516 §1). The leader's
// player says only that it reached the end; what plays next the hub found (findNothingNext), and no
// guest's command is taken for it (D516 §2). Whether it stopped.
func (current *session) stopsAtItsLeadersEnd(now time.Time, from *device, message Inbound) bool {
	if !message.isNothingNext || !current.atItsLeadersEnd(from.id, message) {
		return false
	}
	for _, each := range current.members {
		if each.isHost {
			current.stop(now, each.id)
			return true
		}
	}
	return false
}

// atItsLeadersEnd is whether message is the state of deviceID, the Watch Together session's leader,
// saying its player played the group's film to its end: playing, or paused there, as the host's
// player's own end pauses its group (commandOfEvent, in the SDK) where its state comes after. A cast's
// own session is left as it is: its receiver's page moves it on or waits ready for the next video
// (D364 §5).
func (current *session) atItsLeadersEnd(deviceID int64, message Inbound) bool {
	return message.Type == MessageState && message.IsAtTheEnd && !current.isCast &&
		deviceID != 0 && deviceID == current.leaderID &&
		current.group.fileID != 0 && message.FileID == current.group.fileID
}
