// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
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
