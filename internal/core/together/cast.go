// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// A cast is a session of one (D88, D337; plans/cast-to-device.md §3, A): the person who casts is its
// host and only member, one member on two devices (D338 §4). The device that receives is attached
// as their video viewer (RoleVideo) and leads; the device that cast is their control panel with
// voice (RolePanel), their remote, which plays nothing (D120). It has no links, no locators and no
// place among the person's Watch Together sessions: Open, Mine and a host's endpoints never find
// it, so a host keeps their party beside it.

// The reasons a cast ends, which each of its devices is told as it ends (MessageEnded).
const (
	// CastTakenBack is the receiver's person taking control back on it (D337 §4).
	CastTakenBack = "takenBack"
	// CastReplaced is another of the person's devices casting to the same receiver (D337 §6).
	CastReplaced = "replaced"
	// CastStopped is the remote's Stop casting.
	CastStopped = "stopped"
	// CastFilmEnded is the film ending on the receiver with nothing to play next (§9 Q4).
	CastFilmEnded = "filmEnded"
	// CastDeclined is the receiver's Not now, asked whether to play it (D358 §3).
	CastDeclined = "declined"
	// CastUnanswered is the receiver's question left unanswered for a minute (D358 §4).
	CastUnanswered = "unanswered"
)

// CastOpened is a new cast: its session, and a credential each for the device that cast and the
// one that receives.
type CastOpened struct {
	SessionID          int64
	SenderCredential   string
	ReceiverCredential string
}

// OpenCast starts a cast hosted by host, nothing playing: a session of one, beside any session they
// host (D88, D337).
func (hub *Hub) OpenCast(host Person) (CastOpened, error) {
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
	hub.lastID++
	opened := &session{
		id: hub.lastID, hostUserID: host.UserID, isCast: true, links: map[int64]*link{},
		members: map[int64]*member{}, group: group{state: GroupIdle, rate: 1, at: hub.clock.Now()},
		logger: hub.logger, syncWait: StartingSyncWait, snapSeeksBeyond: StartingSnapSeeksBeyond,
		lastRights: everyRight(),
	}
	hub.lastID++
	hostPerson := host
	caster := &member{
		id: hub.lastID, name: host.Name, isHost: true, person: &hostPerson, devices: map[int64]*device{},
	}
	caster.addCredential(madeCredential{credential: sender})
	caster.addCredential(madeCredential{credential: receiver})
	opened.members[caster.id] = caster
	hub.sessions[opened.id] = opened
	return CastOpened{SessionID: opened.id, SenderCredential: sender, ReceiverCredential: receiver}, nil
}

// CastCredential is another credential for the cast sessionID that userID cast, for a device of
// theirs coming back to it: a page loaded again.
func (hub *Hub) CastCredential(sessionID, userID int64) (string, error) {
	credential, err := randbytes.Base64URLSecret(hub.random, secretBytes)
	if err != nil {
		return "", fmt.Errorf("make a cast's credential: %w", err)
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	cast, err := hub.castOf(sessionID, userID)
	if err != nil {
		return "", err
	}
	for _, each := range cast.members {
		each.addCredential(madeCredential{credential: credential})
	}
	return credential, nil
}

// EndCast ends the cast sessionID that userID cast: each of its devices is told why, and its
// credentials admit nothing more.
func (hub *Hub) EndCast(sessionID, userID int64, reason string) error {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	cast, err := hub.castOf(sessionID, userID)
	if err != nil {
		return err
	}
	cast.broadcast(Outbound{V: ProtocolVersion, Type: MessageEnded, Reason: reason})
	delete(hub.sessions, sessionID)
	return nil
}

// CastPosition is where the cast sessionID that userID cast stands now, in milliseconds, and
// whether it plays: where a remote offers to play from once the cast ends (§9 Q5).
func (hub *Hub) CastPosition(sessionID, userID int64) (positionMs int64, isPlaying bool, err error) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	cast, err := hub.castOf(sessionID, userID)
	if err != nil {
		return 0, false, err
	}
	return cast.positionAt(hub.clock.Now()).Milliseconds(), cast.group.state == GroupPlaying, nil
}

// castOf is the cast userID cast by sessionID.
func (hub *Hub) castOf(sessionID, userID int64) (*session, error) {
	cast, found := hub.sessions[sessionID]
	if !found || !cast.isCast || cast.hostUserID != userID {
		return nil, ErrNoSession
	}
	return cast, nil
}
