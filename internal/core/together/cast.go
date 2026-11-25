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
	// CastReceiverGone is the castee silent for CastSilentAfter, the hub's end alone (D555 §3): no
	// sender may end a cast so.
	CastReceiverGone = "receiverGone"
	// CastDeclined is the receiver's Not now, asked whether to play it (D358 §3).
	CastDeclined = "declined"
	// CastUnanswered is the receiver's question left unanswered for a minute (D358 §4).
	CastUnanswered = "unanswered"
	// CastSessionEnded is the end of the Watch Together session the receiver took its person's part
	// in, or its caster's leaving it, where the film was theirs only through the session (D389 §1,
	// D555 §7): the cast ends with it.
	CastSessionEnded = "sessionEnded"
	// CastMovedAway is its caster's place moved to another device: their seat in a Watch Together
	// session taken over there (HostHere, GuestHere; the owner, 2026-11-25), or a guest's device casting
	// anew, which replaces its last cast (D520 §1).
	CastMovedAway = "movedAway"
	// CastIdle is a cast to a guest receiver a device code made, ended after nothing played in it
	// for its idle end (D469 §3).
	CastIdle = "idle"
)

// CastMoved is no end of a cast but what the devices of its own session of one are told as the cast
// moves into its caster's part in the Watch Together session they are in, that session let go
// (moveIntoPart, D555 §2, §7): the cast plays on there, and each device says hello again by a
// credential read anew for it (CastCredential), its remote showing the cast live throughout. Distinct
// from every reason a cast ends (the walk on staging, 2026-11-25: a remote told nothing took the
// session's loss for the cast's end).
const CastMoved = "castMoved"

// CastSilentAfter is how long a cast's castee may say nothing before the hub ends its cast, its
// devices told receiverGone (D555 §3): it says where it is each 5 s while it plays, so four of those
// missed, a television switched off or its page closed, while a page loaded again says hello well
// within it. The owner's 20 s.
const CastSilentAfter = 20 * time.Second

// endedCastKept is how long a cast's end is kept for a device of it that says hello after it: a
// remote opened as the cast ends reads the cast, then says hello a moment later, and is told why it
// ended rather than only that it is gone (the browser tier, 2026-10-29). A justified constant: a
// slow phone's read and hello, many times over.
const endedCastKept = time.Minute

// endedCast is one credential of a cast that ended, why, and when; or of a cast's own session of one
// let go as the cast moved into a Watch Together part, CastMoved.
type endedCast struct {
	hash    [sha256.Size]byte
	reason  string
	endedAt time.Time
}

// CastEndedError is a hello whose credential was a cast's that ended within endedCastKept: why it
// ended, a cast's reason; or CastMoved, the cast's own session of one let go as it moved, to read the
// cast's credential anew. It is ErrNoSession too, to every caller that asks only that.
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
	// heardAt is when its castee last said anything, its deadline CastSilentAfter on (EndSilentCasts);
	// zero until it first does or takes the cast (CastHeard), the wait for its OK bounded by the
	// live registry's (D358 §4).
	heardAt time.Time
	// sender is, for a guest's cast by link (GuestCastPart), the hash of the credential of the
	// guest's device that cast (D520 §1), its caster known by membership alone, userID 0; zero for a
	// signed-in person's.
	sender [sha256.Size]byte
	// openedAt is when the cast began: a guest's castee that says nothing for the unanswered wait
	// from then ends it (EndGuestCasts).
	openedAt time.Time
	// movedDevices are the devices that said hello by its credentials in the Watch Together part it
	// left to play alone (moveCastAlone), handed it again on their live connections: its end tells
	// them, as a page not yet back on the cast's own session still listens on them.
	movedDevices []*device
	// state is what state its castee last said it is in (D555 §4), kept with its position for a
	// caster coming back; empty until it says, read as CastStarting.
	state CastState
	// handed is what the cast hands its castee to play, from where, and whether the castee took it
	// (cast_handed.go, D555 §2): the live registry keeps none of it.
	handed handed
}

// isGuests is whether the cast is a guest's by link, cast by their device sender.
func (of *casting) isGuests() bool {
	return of.sender != ([sha256.Size]byte{})
}

// EndedCast is a cast the hub ended of itself, for its caster's pages to be told: its id and whose.
type EndedCast struct {
	CastID int64
	UserID int64
}

// CastHeard notes that the castee of the cast castID that userID cast took it now (its OK, or a cast
// taken asking nothing, D468 §1): it is accepted, asked no more, and its deadline runs from now (D555
// §3).
func (hub *Hub) CastHeard(castID, userID int64) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if heard, found := hub.casts[castID]; found && heard.userID == userID && !heard.isGuests() {
		heard.heardAt = hub.clock.Now()
		heard.handed.isAccepted = true
	}
}

// heardFrom notes that from, a device of sender's, said something now: where it plays a cast, by a
// credential of it, the cast's castee is heard (D555 §3); any device of a cast keeps which, for the
// state its castee says it in (§4). The caller holds the mutex.
func (hub *Hub) heardFrom(sender *member, from *device, now time.Time) {
	for _, held := range sender.credentials {
		if held.castID == 0 || held.hash != from.credentialHash {
			continue
		}
		if heard, found := hub.casts[held.castID]; found {
			from.cast = heard // a panel too: a castee leading its person's session says its state here
			if from.role.plays() {
				heard.heardAt = now
			}
		}
		return
	}
}

// EndSilentCasts ends each cast whose castee has said nothing for CastSilentAfter, by the cast's own
// deadline, each of its devices told receiverGone, and returns them for their casters' pages to be
// told (D555 §3). A castee whose socket only closed is silent too: a page loaded again says hello
// within the wait and keeps its cast.
func (hub *Hub) EndSilentCasts() []EndedCast {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	now := hub.clock.Now()
	ended := []EndedCast{}
	for _, each := range hub.casts {
		if hub.forgetGone(each) || each.heardAt.IsZero() || now.Sub(each.heardAt) < CastSilentAfter {
			continue
		}
		ended = append(ended, EndedCast{CastID: each.id, UserID: each.userID})
		hub.endCast(each, CastReceiverGone)
	}
	return ended
}

// EndGuestCasts ends each guest's cast by link (GuestCastPart) whose castee has not said hello
// within unansweredAfter of the cast's start, as unanswered (D358 §4), or whose session has played
// nothing for idleAfter, as idle (D469 §3): each by the one end a cast has, its devices told why and
// its credentials taken back, as its silence ends it by EndSilentCasts (D555 §3, §5). It returns
// them, whose casters are known by membership alone, for the ends to be logged.
func (hub *Hub) EndGuestCasts(unansweredAfter, idleAfter time.Duration) []EndedCast {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	now := hub.clock.Now()
	ended := []EndedCast{}
	for _, each := range hub.casts {
		if !each.isGuests() || hub.forgetGone(each) {
			continue
		}
		where, _ := hub.castersPart(each)
		reason := ""
		switch {
		case each.heardAt.IsZero():
			if now.Sub(each.openedAt) >= unansweredAfter {
				reason = CastUnanswered
			}
		case where.group.state != GroupPlaying && now.Sub(where.group.at) >= idleAfter:
			reason = CastIdle
		}
		if reason == "" {
			continue
		}
		ended = append(ended, EndedCast{CastID: each.id})
		hub.endCast(each, reason)
	}
	return ended
}

// forgetGone forgets the cast where its caster's membership has gone, or holds none of its
// credentials any more, as a guest's sending device's credential gone takes back its cast's
// (endCastsWhere): whether it was. Its devices were let go as its credentials went. The caller holds
// the mutex.
func (hub *Hub) forgetGone(cast *casting) bool {
	_, caster := hub.castersPart(cast)
	if caster != nil && slices.ContainsFunc(caster.credentials, func(held credential) bool {
		return held.castID == cast.id
	}) {
		return false
	}
	delete(hub.casts, cast.id)
	return true
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
	opened := &casting{userID: host.UserID, held: held, openedAt: hub.clock.Now()}
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
	hub.openAlone(into, host, hub.lastID)
}

// openAlone opens the cast's own session of one, id, hosted by host, nothing playing. The caller
// holds the mutex.
func (hub *Hub) openAlone(into *casting, host Person, id int64) {
	alone := &session{
		id: id, hostUserID: host.UserID, isCast: true, links: map[int64]*link{},
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
	if place.cast.isOwn && hub.moveIntoPart(place.cast) {
		_, caster = hub.castersPart(place.cast)
	}
	caster.addCastPart(credential, castID)
	return credential, nil
}

// castsIntoPart moves each cast alone of userID's into their part in the Watch Together session
// they are in now, as they join it (D555 §7): the castee goes where its member goes at the join, not
// once a page of theirs comes back. It returns the casts moved. The caller holds the mutex.
func (hub *Hub) castsIntoPart(userID int64) []int64 {
	var moved []int64
	for _, each := range hub.casts {
		if each.userID == userID && each.isOwn && !each.isGuests() && hub.moveIntoPart(each) {
			moved = append(moved, each.id)
		}
	}
	// One moved alone as its person left another session for this one is in this one now: the join
	// hands it, and nothing weighs its film as a cast alone.
	hub.movedToTell = slices.DeleteFunc(hub.movedToTell, func(told EndedCast) bool {
		return slices.Contains(moved, told.CastID)
	})
	slices.Sort(moved)
	return moved
}

// moveIntoPart moves the cast, alone in its own session of one, into its person's part in the Watch
// Together session they are in, its own session let go, its devices told so to say hello again by
// the credential handed them (letMovedGo, D555 §2): whether they are in one. As the host's player it leads that
// session with the film it plays, from where it stood; as a guest's it follows the session, handed
// the session's film from where the session stands, its own film left behind (§1, §7). The caller
// holds the mutex.
func (hub *Hub) moveIntoPart(cast *casting) bool {
	alone, aloneCaster := hub.castersPart(cast)
	if alone == nil || !hub.castIntoPart(cast) {
		return false
	}
	now := hub.clock.Now()
	hub.letMovedGo(alone, aloneCaster, now)
	into, caster := hub.castersPart(cast)
	if caster.isHost {
		cast.handed.anchor(alone, into, now) // it plays on from where it stood
	} else {
		cast.handed.follow(into, now)
	}
	hub.logger.Info("watch together: a cast took its caster's part in the session they are in",
		"castId", cast.id, sessionIDAttribute, into.id, "isHost", caster.isHost)
	return true
}

// letMovedGo lets go the cast's own session of one, alone, as the cast moved out of it into a Watch
// Together part: each of its devices is told CastMoved, and a device saying hello by one of its
// caster's credentials within endedCastKept is told so too, so each reads the cast's credential anew
// rather than take the cast for ended (D555 §2). The caller holds the mutex.
func (hub *Hub) letMovedGo(alone *session, caster *member, now time.Time) {
	alone.broadcast(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: CastMoved})
	hub.endedCasts = hub.endedCastsKeptAt(now)
	for _, held := range caster.credentials {
		hub.endedCasts = append(hub.endedCasts, endedCast{hash: held.hash, reason: CastMoved, endedAt: now})
	}
	delete(hub.sessions, alone.id)
}

// EndCast ends the cast castID that userID cast: each of its devices is told why and let go, and its
// credentials admit nothing more, its own session of one ended with it; a device saying hello with
// one within endedCastKept is told why too (Attach's *CastEndedError). In a Watch Together session
// the session goes on, with the caster's other devices in it (D555 §2).
func (hub *Hub) EndCast(castID, userID int64, reason string) error {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	ended, found := hub.casts[castID]
	if !found || ended.userID != userID || ended.isGuests() {
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
		for _, moved := range ended.movedDevices {
			moved.send(told)
		}
		delete(hub.sessions, where.id)
		return
	}
	where.letCastGo(now, caster, ended.id, told)
}

// castsLeave takes each cast that is a part in the Watch Together session from as it ends, or as its
// caster gone leaves it (nil for every member's), out of it: the castee goes where its member goes
// (D555 §7). A signed-in person's moves to a session of one, playing on (moveCastAlone), whose
// caster's right to its film without the session is weighed once the lock is let go (OnCastMoved);
// a guest's by link, whose rights came from the session, ends, told reason. The caller holds the
// mutex.
func (hub *Hub) castsLeave(from *session, gone *member, reason string) {
	for _, each := range hub.casts {
		if each.isOwn || each.sessionID != from.id || (gone != nil && each.memberID != gone.id) {
			continue
		}
		if each.isGuests() {
			hub.endCast(each, reason)
			continue
		}
		hub.moveCastAlone(each)
	}
}

// moveCastAlone moves the cast, a part in a Watch Together session, to a session of one of its own,
// whose id it takes, playing what it played from where it stood (D88, D555 §7): its devices leave
// the session told nothing, to be handed their new part (onCastMoved), and its credentials go with
// it, so a device saying hello by one comes to the cast where it is now. The caller holds the mutex.
func (hub *Hub) moveCastAlone(cast *casting) {
	where, caster := hub.castersPart(cast)
	if where == nil {
		delete(hub.casts, cast.id)
		return
	}
	now := hub.clock.Now()
	playing := where.group
	taken, devices := where.takeCastOut(now, caster, cast.id, nil)
	cast.movedDevices = devices
	hub.openAlone(cast, Person{UserID: cast.userID, Name: caster.name}, cast.id)
	alone, own := hub.castersPart(cast)
	alone.group = playing
	cast.handed.leave(where, alone, now)
	own.credentials = append(own.credentials, taken...)
	cast.held = ""
	hub.movedToTell = append(hub.movedToTell, EndedCast{CastID: cast.id, UserID: cast.userID})
	hub.logger.Info("watch together: a cast left its caster's part, to play alone",
		"castId", cast.id, sessionIDAttribute, where.id)
}

// letCastGo takes back the credentials of the cast castID from its caster and lets go each device
// that said hello with one, telling it told; the lead passes on as a device's going passes it. The
// caller holds the mutex.
func (current *session) letCastGo(now time.Time, caster *member, castID int64, told Outbound) {
	current.takeCastOut(now, caster, castID, &told)
}

// takeCastOut is letCastGo telling each device told, or nothing for nil, and returns the credentials
// it took and the devices it let go. The caller holds the mutex.
func (current *session) takeCastOut(
	now time.Time, caster *member, castID int64, told *Outbound,
) ([]credential, []*device) {
	hashes := map[[sha256.Size]byte]bool{}
	var taken []credential
	var gone []*device
	caster.credentials = slices.DeleteFunc(caster.credentials, func(kept credential) bool {
		if kept.castID == castID {
			hashes[kept.hash] = true
			taken = append(taken, kept)
			return true
		}
		return false
	})
	for id, attached := range caster.devices {
		if !hashes[attached.credentialHash] {
			continue
		}
		if told != nil {
			attached.send(*told)
		}
		gone = append(gone, attached)
		delete(caster.devices, id)
		current.endRun(caster, attached, string(goneRemoved))
		if id == current.leaderID {
			current.leaderGone(now, caster.isHost, goneRemoved)
		}
	}
	current.sendRoster()
	return taken, gone
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

// CastStateOf is what state the cast castID that userID cast is in, as its castee last said, for a
// caster coming back to it (D555 §4): CastStarting until it says; empty for a cast that is none of
// theirs or ended.
func (hub *Hub) CastStateOf(castID, userID int64) CastState {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	place, err := hub.castOf(castID, userID)
	switch {
	case err != nil:
		return ""
	case place.cast.state == "":
		return CastStarting
	}
	return place.cast.state
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
	if !found || held.userID != userID || held.isGuests() {
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
