// SPDX-License-Identifier: AGPL-3.0-or-later

package live

import (
	"cmp"
	"slices"
	"time"
)

// GuestByCodeWaitsAfter is how long a guest receiver a device code made waits to be cast to (D469 §2):
// the life of the hand-off its device takes its session by (auth's deviceHandOffLife), since the
// device learns of its sign-in through the paid service, takes it, and loads its client before its
// page connects, and its sender casts to it once it has. A justified constant.
const GuestByCodeWaitsAfter = 3 * time.Minute

// GuestIdleAfter is how long the cast to a guest receiver a device code made may have nothing playing
// before it ends, and the guest with it, as a Chromecast's session ends (D469 §3; the owner's thirty
// minutes).
const GuestIdleAfter = 30 * time.Minute

// guest is a guest receiver (D465 §1): a Google Cast device running the receiver for one cast from
// the device that sent it its grant, which no other device of the person's sees or casts to.
type guest struct {
	senderDeviceID int64
	// name is the Cast device's name, as its sender's Cast menu showed it.
	name string
	// since is when its grant was traded: one never cast to is ended UnansweredAfter it (EndGuests).
	since time.Time
	// hasReceived is whether its one cast started: once it ends, it is no one's target (D465 §1).
	hasReceived bool
	// isByCode is a guest receiver a device code made (ExpectGuestByCode, D469 §2): never cast to, it
	// ends GuestByCodeWaitsAfter its making, and its cast ends after GuestIdleAfter with nothing playing.
	isByCode bool
}

// waitsFor is how long the guest receiver may go uncast to before EndGuests ends it.
func (each *guest) waitsFor() time.Duration {
	if each.isByCode {
		return GuestByCodeWaitsAfter
	}
	return UnansweredAfter
}

// Guest is a guest receiver that EndGuests ended: its person's device, to remove.
type Guest struct {
	UserID   int64
	DeviceID int64
}

// ExpectGuest makes userID's device deviceID, named name, a guest receiver of senderDeviceID's (D465
// §1), as its grant is traded: it is in no devices message, and only senderDeviceID casts to it, once.
func (registry *Registry) ExpectGuest(userID, deviceID, senderDeviceID int64, name string) {
	registry.expect(userID, deviceID, &guest{senderDeviceID: senderDeviceID, name: name})
}

// ExpectGuestByCode is ExpectGuest for a device whose code its person answered by casting to it (D469
// §2): it waits GuestByCodeWaitsAfter for its cast, which ends after GuestIdleAfter with nothing
// playing (GuestCastsEndingIdle).
func (registry *Registry) ExpectGuestByCode(userID, deviceID, senderDeviceID int64, name string) {
	registry.expect(userID, deviceID, &guest{senderDeviceID: senderDeviceID, name: name, isByCode: true})
}

// expect makes userID's device deviceID the guest receiver expected, from now.
func (registry *Registry) expect(userID, deviceID int64, expected *guest) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	if owner == nil {
		owner = newPerson()
		registry.people[userID] = owner
	}
	expected.since = registry.clock.Now()
	owner.guests[deviceID] = expected
}

// GuestCastsEndingIdle is the casts to guest receivers a device code made, which end after
// GuestIdleAfter with nothing playing (D469 §3): their sessions know how long nothing has played.
func (registry *Registry) GuestCastsEndingIdle() []Cast {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	casts := []Cast{}
	for _, owner := range registry.people {
		for deviceID, each := range owner.guests {
			if held := owner.casts[deviceID]; held != nil && each.isByCode {
				casts = append(casts, held.Cast)
			}
		}
	}
	slices.SortFunc(casts, func(one, other Cast) int { return cmp.Compare(one.ID, other.ID) })
	return casts
}

// GuestCastableBy is the name of userID's guest receiver deviceID, and whether senderDeviceID may
// cast to it: the device that sent its grant, before its one cast.
func (registry *Registry) GuestCastableBy(userID, deviceID, senderDeviceID int64) (string, bool) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	if owner == nil || owner.guests[deviceID] == nil {
		return "", false
	}
	each := owner.guests[deviceID]
	return each.name, each.senderDeviceID == senderDeviceID && !each.hasReceived
}

// IsGuest is whether userID's device deviceID is a guest receiver ExpectGuest made and EndGuests has
// not ended.
func (registry *Registry) IsGuest(userID, deviceID int64) bool {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	owner := registry.people[userID]
	return owner != nil && owner.guests[deviceID] != nil
}

// EndGuests ends every guest receiver whose one cast has ended, or that was never cast to within
// UnansweredAfter of its trade (GuestByCodeWaitsAfter for one a code made), each of its connections
// told its sign-in ended and let go, and returns them for their devices to be removed (D465 §1: it
// keeps nothing after its cast).
func (registry *Registry) EndGuests() []Guest {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	now := registry.clock.Now()
	ended := []Guest{}
	for userID, owner := range registry.people {
		for deviceID, each := range owner.guests {
			if owner.casts[deviceID] != nil || !each.hasReceived && now.Sub(each.since) < each.waitsFor() {
				continue
			}
			delete(owner.guests, deviceID)
			for id, open := range owner.connections {
				if open.deviceID == deviceID {
					open.send(Outbound{Type: MessageSignedOut})
					delete(owner.connections, id)
				}
			}
			ended = append(ended, Guest{UserID: userID, DeviceID: deviceID})
		}
		if owner.isEmpty() {
			delete(registry.people, userID)
		}
	}
	slices.SortFunc(ended, func(one, other Guest) int { return cmp.Compare(one.DeviceID, other.DeviceID) })
	return ended
}
