// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"cmp"
	"slices"
	"time"
)

// CastFilm is what a cast hands its castee to play: the file, the title it plays as, the one it was
// opened from, its name, and how long it plays; FileID 0 for nothing, the castee ready between videos
// (D364 §5).
type CastFilm struct {
	FileID     int64
	ItemID     int64
	ItemName   string
	DurationMs int64
}

// CastHanded is a cast as its castee is handed it and its remote shows it (D555 §2): its film, where
// the castee starts it, nil for where the person left it, whether it opens it paused there, and
// whether the castee took the cast, asked no more (D468 §1).
type CastHanded struct {
	Film       CastFilm
	PositionMs *int64
	IsPaused   bool
	IsAccepted bool
}

// handed is what a cast hands its castee (CastPuts), and whether the castee took it (CastHeard).
type handed struct {
	video CastFilm
	// startMs and isPaused are where the castee starts the film and whether paused, as it was put;
	// the cast's group, once it has moved on with that film since (putSeq), says where it stands.
	startMs  *int64
	isPaused bool
	// putSeq is the cast's group's change as the film was put, or as the cast moved into a part.
	putSeq     int64
	isAccepted bool
}

// CastPuts has the cast castID that userID cast hand its castee video, from positionMs, nil for where
// the person left it, paused there or not (D364 §3; the owner, 2026-11-08): what the person plays on
// the caster while the cast lasts. The castee is to be handed it again (the live registry's hand-off).
func (hub *Hub) CastPuts(castID, userID int64, video CastFilm, positionMs *int64, isPaused bool) error {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	place, err := hub.castOf(castID, userID)
	if err != nil {
		return err
	}
	place.cast.handed = handed{
		video: video, startMs: positionMs, isPaused: isPaused, putSeq: place.where.group.seq,
		isAccepted: place.cast.handed.isAccepted,
	}
	return nil
}

// CastHandedOf is the cast castID that userID cast as its castee is handed it now (D555 §2): its film
// from where it was put, or, where the cast's group has played that film on since, from where the
// group stands, paused as the group is: the hub's word on where a cast stands wins over where it was
// put, so a castee loaded again picks it up there.
func (hub *Hub) CastHandedOf(castID, userID int64) (CastHanded, error) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	place, err := hub.castOf(castID, userID)
	if err != nil {
		return CastHanded{}, err
	}
	kept := place.cast.handed
	told := CastHanded{
		Film: kept.video, PositionMs: kept.startMs, IsPaused: kept.isPaused, IsAccepted: kept.isAccepted,
	}
	if kept.playsOn(place.where) {
		positionMs := place.where.positionAt(hub.clock.Now()).Milliseconds()
		told.PositionMs, told.IsPaused = &positionMs, place.where.group.state == GroupPaused
	}
	return told, nil
}

// playsOn is whether where's group plays the film handed and has moved on since it was put.
func (kept *handed) playsOn(where *session) bool {
	return kept.video.FileID != 0 && where.group.fileID == kept.video.FileID && where.group.seq != kept.putSeq
}

// anchor has the film handed start where it stood in from, as the cast moves into its caster's part
// in into (D555 §7): the castee plays on from there, into's group not yet its.
func (kept *handed) anchor(from, into *session, now time.Time) {
	if kept.playsOn(from) {
		positionMs := from.positionAt(now).Milliseconds()
		kept.startMs, kept.isPaused = &positionMs, from.group.state == GroupPaused
	}
	kept.putSeq = into.group.seq
}

// follow has the cast hand the film into plays from where into stands, as any guest's player
// follows it (D555 §1, §7): moved into a guest's part in into, or its host's player as another member
// moved into's film along (castsMoveAlong). Its film is left behind, so the castee is not handed a
// film the session does not play. The film is named by its ids alone until the caster's rights name
// it (the API's CastPuts); nothing, the castee ready, while into plays nothing.
func (kept *handed) follow(into *session, now time.Time) {
	kept.video = CastFilm{FileID: into.group.fileID, ItemID: into.group.itemID}
	kept.startMs, kept.isPaused = nil, false
	if into.group.fileID != 0 {
		positionMs := into.positionAt(now).Milliseconds()
		kept.startMs, kept.isPaused = &positionMs, into.group.state == GroupPaused
	}
	kept.putSeq = into.group.seq
}

// castsMoveAlong has each cast that is its host's playing device in where, handed the film a next or
// a previous moved where on from, hand the film it put on for everyone instead, from where it put it
// on (D471 §2, D555 §1): a guest's next takes the host's television along, as it takes the host's
// own page (the walk on staging, 2026-11-26: the television stayed on the episode before, and the
// host's stop of the cast put that back on for the group). It returns the casts moved, for their
// castees to be handed again (OnCastMovedAlong); none where message moved nothing along, wasOn the
// film where played before it. The caller holds the mutex.
func (hub *Hub) castsMoveAlong(where *session, wasOn int64, message Inbound) []EndedCast {
	isMove := message.Command == CommandNext || message.Command == CommandPrevious
	if !isMove || message.movedFrom == 0 || wasOn != message.movedFrom || message.FileID == 0 ||
		where.group.fileID != message.FileID {
		return nil
	}
	now := hub.clock.Now()
	var moved []EndedCast
	for _, each := range hub.casts {
		if each.isOwn || each.sessionID != where.id || each.handed.video.FileID != message.movedFrom {
			continue
		}
		if _, caster := hub.castersPart(each); caster == nil || !caster.isHost {
			continue // a guest's castee follows the session itself (cast-ready.tsx)
		}
		each.handed.follow(where, now)
		moved = append(moved, EndedCast{CastID: each.id, UserID: each.userID})
	}
	slices.SortFunc(moved, func(a, b EndedCast) int { return cmp.Compare(a.CastID, b.CastID) })
	return moved
}

// leave has the film handed be the one from plays as the cast leaves it for its own session of one,
// from where from stands (D555 §5, §7: every move keeps the position), asked before the cast's
// devices leave from, whose going may pause it: a castee that followed the session, or led it on to
// another film, plays on with that film, never the one it was handed before, nor from where that
// was put. Where from plays nothing, what was handed stands. The caller sets putSeq.
func (kept *handed) leave(from *session, now time.Time) {
	if from.group.fileID != 0 {
		if kept.video.FileID != from.group.fileID {
			kept.video = CastFilm{FileID: from.group.fileID, ItemID: from.group.itemID}
		}
		positionMs := from.positionAt(now).Milliseconds()
		kept.startMs, kept.isPaused = &positionMs, from.group.state == GroupPaused
	}
}

// CastUnansweredAfter is how long a cast's castee may leave its question unanswered before the hub
// ends the cast itself (D358 §4; principles §28): the castee ends it at its own minute and the caster
// a few seconds after, but a castee never handed the question, or a caster whose page went away, ends
// nothing, and its caster stayed asking (the owner, 2026-11-03). Past both pages' own bounds, so a page
// there says why first. A justified constant.
const CastUnansweredAfter = 90 * time.Second

// EndUnansweredCasts ends each signed-in person's cast whose castee has not taken it within
// CastUnansweredAfter of its start, as unanswered (D358 §4), each of its devices told why, by the
// cast's own deadline beside its silence's (D555 §3), and returns them for their casters' pages to be
// told. A cast taken goes on however long; a guest's cast by link ends by EndGuestCasts.
func (hub *Hub) EndUnansweredCasts() []EndedCast {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	now := hub.clock.Now()
	ended := []EndedCast{}
	for _, each := range hub.casts {
		if each.isGuests() || hub.forgetGone(each) || each.handed.isAccepted ||
			now.Sub(each.openedAt) < CastUnansweredAfter {
			continue
		}
		ended = append(ended, EndedCast{CastID: each.id, UserID: each.userID})
		hub.endCast(each, CastUnanswered)
	}
	return ended
}
