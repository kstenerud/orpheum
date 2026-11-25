// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// A cast is a session of one (D88, D337; plans/cast-to-device.md §3, A): the person who casts is its
// host and only member, one member on two devices (D338 §4). The device that receives is attached
// as their video viewer (RoleVideo) and leads; the device that cast is their control panel with
// voice (RolePanel), their remote, which plays nothing (D120). It has no links, no locators and no
// place among the person's Watch Together sessions: Open, Mine and a host's endpoints never find
// it, so a host keeps their party beside it. It lasts while the person plays one video after another
// on the receiver (D364): its group goes back to nothing on between them (CastReady).

// The reasons a cast ends, which each of its devices is told as it ends (MessageEnded).
const (
	// CastTakenBack is the receiver's person taking control back on it (D337 §4).
	CastTakenBack = "takenBack"
	// CastStopped is the remote's Stop casting.
	CastStopped = "stopped"
	// CastReceiverGone is the receiver gone, no page of it connected, for two minutes (D364 §6).
	CastReceiverGone = "receiverGone"
	// CastDeclined is the receiver's Not now, asked whether to play it (D358 §3).
	CastDeclined = "declined"
	// CastUnanswered is the receiver's question left unanswered for a minute (D358 §4).
	CastUnanswered = "unanswered"
	// CastSessionEnded is the end of the Watch Together session the receiver took its person's part
	// in (D389 §1): the cast ends with it.
	CastSessionEnded = "sessionEnded"
	// CastIdle is a cast to a guest receiver a device code made, ended after nothing played in it
	// for its idle end (D469 §3).
	CastIdle = "idle"
)

// endedCastKept is how long a cast's end is kept for a device of it that says hello after it: a
// remote opened as the cast ends reads the cast, then says hello a moment later, and is told why it
// ended rather than only that it is gone (the browser tier, 2026-10-29). A justified constant: a
// slow phone's read and hello, many times over.
const endedCastKept = time.Minute

// endedCast is one credential of a cast that ended, why, and when.
type endedCast struct {
	hash    [sha256.Size]byte
	reason  string
	endedAt time.Time
}

// CastEndedError is a hello whose credential was a cast's that ended within endedCastKept: why it
// ended, a cast's reason. It is ErrNoSession too, to every caller that asks only that.
type CastEndedError struct {
	Reason string
}

func (ended *CastEndedError) Error() string {
	return "the cast ended (" + ended.Reason + "): " + ErrNoSession.Error()
}

// Unwrap is ErrNoSession: the session is gone.
func (ended *CastEndedError) Unwrap() error { return ErrNoSession }

// casting is one cast in the hub (D555 §2): a member's playing device, the castee, and its remote,
// the caster's page, in one session under the one membership. Alone it is a session of one, the
// cast's own (D88), whose id it shares; in a Watch Together session it is the caster's part in that
// session, no second session beside it. Its credentials are the member's, each marked with the
// cast's id (credential.castID), and its end takes them back.
type casting struct {
	id     int64
	userID int64
	// sessionID is the session the cast is in, and memberID its caster's membership there.
	sessionID, memberID int64
	// isOwn is whether the session is the cast's own, of one, which ends with it.
	isOwn bool
	// held is the membership the casting page holds as a guest, whose part the cast takes (a host
	// who joined their own session as a guest from another device casts as that guest); "" for the
	// person's own. A secret: never logged.
	held string
}

// castCredentialsKept is how many credentials a cast keeps, its castee's and its remote's, each page
// loaded again adding one: the newest, past every page a cast's two devices could have open.
const castCredentialsKept = 8

// CastOpened is a new cast: its id, a credential each for the device that cast and the one that
// receives, and the Watch Together session it is the caster's part in, 0 for its own of one.
type CastOpened struct {
	CastID             int64
	SenderCredential   string
	ReceiverCredential string
	TogetherSessionID  int64
}

// OpenCast starts a cast by host, the casting page holding the guest's membership held, or "" for
// the person's own part (D555 §2): in the Watch Together session they are in, the caster's part in
// it, the castee to attach as their playing device and the remote as their panel, no session opened
// beside it; else a hidden session of one with nothing playing (D88, D337), beside any party.
func (hub *Hub) OpenCast(host Person, held string) (CastOpened, error) {
	sender, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return CastOpened{}, fmt.Errorf("make a cast's credential: %w", err)
	}
	receiver, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return CastOpened{}, fmt.Errorf("make a cast's credential: %w", err)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	opened := &casting{userID: host.UserID, held: held}
	if hub.castIntoPart(opened) {
		hub.lastID++
		opened.id = hub.lastID
	} else {
		hub.castAlone(opened, host)
	}
	hub.casts[opened.id] = opened
	_, caster := hub.castersPart(opened)
	caster.addCastPart(sender, opened.id)
	caster.addCastPart(receiver, opened.id)
	return CastOpened{
		CastID: opened.id, SenderCredential: sender, ReceiverCredential: receiver,
		TogetherSessionID: opened.togetherSessionID(),
	}, nil
}

// castIntoPart puts the cast in its person's part in the Watch Together session they are in, the
// part held names first: whether they are in one. The caller holds the mutex.
func (hub *Hub) castIntoPart(into *casting) bool {
	places := hub.placesOf(&Person{UserID: into.userID}, into.held, 0)
	if len(places) == 0 {
		return false
	}
	taken := places[0] // the host's first, should a person be in two by a sign-in at a hello
	if into.held != "" {
		heldHash := sha256.Sum256([]byte(into.held))
		for _, each := range places {
			if each.as.admits(heldHash) {
				taken = each
				break
			}
		}
	}
	into.sessionID, into.memberID, into.isOwn = taken.in.id, taken.as.id, false
	return true
}

// castAlone opens the cast's own session of one, hosted by host, nothing playing (D88), the cast
// taking its id. The caller holds the mutex.
func (hub *Hub) castAlone(into *casting, host Person) {
	hub.lastID++
	alone := &session{
		id: hub.lastID, hostUserID: host.UserID, isCast: true, links: map[int64]*link{},
		members: map[int64]*member{}, group: group{state: GroupIdle, rate: 1, at: hub.clock.Now()},
		logger: hub.logger, snapSeeksBeyond: StartingSnapSeeksBeyond,
		lastRights: everyRight(),
	}
	hub.lastID++
	hostPerson := host
	caster := &member{
		id: hub.lastID, name: host.Name, isHost: true, person: &hostPerson, devices: map[int64]*device{},
	}
	alone.members[caster.id] = caster
	hub.sessions[alone.id] = alone
	into.id, into.sessionID, into.memberID, into.isOwn = alone.id, alone.id, caster.id, true
}

// castersPart is the session the cast is in and its caster's membership there; nil for none, its
// session ended or its member gone. The caller holds the mutex.
func (hub *Hub) castersPart(cast *casting) (*session, *member) {
	where, found := hub.sessions[cast.sessionID]
	if !found {
		return nil, nil
	}
	caster, found := where.members[cast.memberID]
	if !found {
		return nil, nil
	}
	return where, caster
}

// togetherSessionID is the Watch Together session the cast is a part in, 0 for its own of one.
func (of *casting) togetherSessionID() int64 {
	if of.isOwn {
		return 0
	}
	return of.sessionID
}

// CastPart is the Watch Together session the cast castID that userID cast is their part in, 0 for its
// own of one, and whether they host it: its castee leads that session as their player if so, and
// follows it as any guest's player does if not (D555 §1, §2).
func (hub *Hub) CastPart(castID, userID int64) (sessionID int64, isHost bool) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	place, err := hub.castOf(castID, userID)
	if err != nil || place.cast.isOwn {
		return 0, false
	}
	return place.where.id, place.caster.isHost
}

// CastCredential is another credential for the cast castID that userID cast, for a device of
// theirs coming back to it: a page loaded again. A cast alone whose person is now in a Watch Together
// session moves into their part in it first, its own session let go, its devices to say hello again
// by the credential handed them (D555 §2).
func (hub *Hub) CastCredential(castID, userID int64) (string, error) {
	credential, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return "", fmt.Errorf("make a cast's credential: %w", err)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	place, err := hub.castOf(castID, userID)
	if err != nil {
		return "", err
	}
	caster := place.caster
	if place.cast.isOwn {
		alone := place.cast.sessionID
		if hub.castIntoPart(place.cast) {
			delete(hub.sessions, alone)
			_, caster = hub.castersPart(place.cast)
		}
	}
	caster.addCastPart(credential, castID)
	return credential, nil
}

// EndCast ends the cast castID that userID cast: each of its devices is told why and let go, and its
// credentials admit nothing more, its own session of one ended with it; a device saying hello with
// one within endedCastKept is told why too (Attach's *CastEndedError). In a Watch Together session
// the session goes on, with the caster's other devices in it (D555 §2).
func (hub *Hub) EndCast(castID, userID int64, reason string) error {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	ended, found := hub.casts[castID]
	if !found || ended.userID != userID {
		return ErrNoSession
	}
	hub.endCast(ended, reason)
	return nil
}

// endCast ends the cast ended for reason (EndCast). The caller holds the mutex.
func (hub *Hub) endCast(ended *casting, reason string) {
	delete(hub.casts, ended.id)
	where, caster := hub.castersPart(ended)
	if where == nil {
		return
	}
	now := hub.clock.Now()
	hub.endedCasts = hub.endedCastsKeptAt(now)
	for _, held := range caster.credentials {
		if held.castID == ended.id {
			hub.endedCasts = append(hub.endedCasts, endedCast{hash: held.hash, reason: reason, endedAt: now})
		}
	}
	told := Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: reason}
	if ended.isOwn {
		where.broadcast(told)
		delete(hub.sessions, where.id)
		return
	}
	where.letCastGo(now, caster, ended.id, told)
}

// endCastsIn ends every cast that is a part where the Watch Together session ended as it ends, each of
// its devices told sessionEnded rather than the session's own reason (D389 §1). The caller holds the
// mutex.
func (hub *Hub) endCastsIn(ended *session) {
	for _, each := range hub.casts {
		if each.sessionID == ended.id && !each.isOwn {
			hub.endCast(each, CastSessionEnded)
		}
	}
}

// letCastGo takes back the credentials of the cast castID from its caster and lets go each device
// that said hello with one, telling it told; the lead passes on as a device's going passes it. The
// caller holds the mutex.
func (current *session) letCastGo(now time.Time, caster *member, castID int64, told Outbound) {
	hashes := map[[sha256.Size]byte]bool{}
	caster.credentials = slices.DeleteFunc(caster.credentials, func(kept credential) bool {
		if kept.castID == castID {
			hashes[kept.hash] = true
			return true
		}
		return false
	})
	for id, attached := range caster.devices {
		if !hashes[attached.credentialHash] {
			continue
		}
		attached.send(told)
		delete(caster.devices, id)
		current.endRun(caster, attached, string(goneRemoved))
		if id == current.leaderID {
			current.leaderGone(now, caster.isHost, goneRemoved)
		}
	}
	current.sendRoster()
}

// endedCastsKeptAt is the ended casts' credentials still kept at now (endedCastKept), the oldest
// first. The caller holds the mutex.
func (hub *Hub) endedCastsKeptAt(now time.Time) []endedCast {
	kept := hub.endedCasts
	for len(kept) > 0 && now.Sub(kept[0].endedAt) >= endedCastKept {
		kept = kept[1:]
	}
	return kept
}

// noSessionFor is why credential, which names no member, admits nothing: a *CastEndedError when it
// was a cast's that ended within endedCastKept, else ErrNoSession. The caller holds the mutex.
func (hub *Hub) noSessionFor(credential string) error {
	hash := sha256.Sum256([]byte(credential))
	for _, each := range hub.endedCastsKeptAt(hub.clock.Now()) {
		if subtle.ConstantTimeCompare(each.hash[:], hash[:]) == 1 {
			return &CastEndedError{Reason: each.reason}
		}
	}
	return ErrNoSession
}

// CastPosition is where the cast castID that userID cast stands now, in milliseconds, and whether it
// plays, in the session it is in, its own or the Watch Together session: where a remote offers to
// play from once the cast ends (§9 Q5).
func (hub *Hub) CastPosition(castID, userID int64) (positionMs int64, isPlaying bool, err error) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	place, err := hub.castOf(castID, userID)
	if err != nil {
		return 0, false, err
	}
	return place.where.positionAt(hub.clock.Now()).Milliseconds(), place.where.group.state == GroupPlaying, nil
}

// CastPlays is the film the cast castID that userID cast plays in its session now; FileID 0 for none.
func (hub *Hub) CastPlays(castID, userID int64) Played {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	place, err := hub.castOf(castID, userID)
	if err != nil {
		return Played{}
	}
	return Played{FileID: place.where.group.fileID, ItemID: place.where.group.itemID}
}

// CastIdleFor is how long nothing has played in the cast castID that userID cast: none while it
// plays, else since its group last changed, a film stopped, paused or put on (D469 §3).
func (hub *Hub) CastIdleFor(castID, userID int64) (time.Duration, error) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	place, err := hub.castOf(castID, userID)
	if err != nil {
		return 0, err
	}
	if place.where.group.state == GroupPlaying {
		return 0, nil
	}
	return hub.clock.Now().Sub(place.where.group.at), nil
}

// CastReady has the cast castID that userID cast play nothing, its receiver ready between videos
// (D364 §5): the next video is put on afresh, from where it is asked to start, even the one that
// played before. A cast in a Watch Together session leaves the session's film to the session.
func (hub *Hub) CastReady(castID, userID int64) error {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	place, err := hub.castOf(castID, userID)
	if err != nil {
		return err
	}
	where := place.where
	if !place.cast.isOwn || where.group.fileID == 0 {
		return nil
	}
	where.group.fileID, where.group.itemID, where.group.position = 0, 0, 0
	where.filmChanged(hub.clock.Now()) // nothing on, no one leads (D374 §1)
	where.change(hub.clock.Now(), GroupIdle, EventStopped, 0)
	return nil
}

// castOf is the cast userID cast by castID, the session it is in and its caster's membership there.
// The caller holds the mutex.
func (hub *Hub) castOf(castID, userID int64) (castPlace, error) {
	held, found := hub.casts[castID]
	if !found || held.userID != userID {
		return castPlace{}, ErrNoSession
	}
	where, caster := hub.castersPart(held)
	if where == nil {
		return castPlace{}, ErrNoSession
	}
	return castPlace{cast: held, where: where, caster: caster}, nil
}

// castPlace is a cast, the session it is in and its caster's membership there.
type castPlace struct {
	cast   *casting
	where  *session
	caster *member
}
