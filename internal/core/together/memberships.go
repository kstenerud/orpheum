// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"slices"

	discoverylink "github.com/kstenerud/orpheum/discovery/link"
	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// credentialsKeptPerMember is how many devices' credentials a member keeps: each device that has the
// membership back adds one, and a page reloaded again and again would otherwise add without end. A
// device whose credential went asks for the membership again (Mine).
const credentialsKeptPerMember = 16

// credential is one device's hold on a membership: its credential's hash, and the hash of the
// locator a discovery finds the core by for that device (the protocol's §3.1), which ends with it. The
// host's has none: their device finds the core by their own session's locator.
type credential struct {
	hash        [sha256.Size]byte
	locatorHash string
}

// madeCredential is a new credential and its locator, before the member keeps their hashes; the
// host's has no locator.
type madeCredential struct {
	credential, locator string
}

// newCredential makes a membership credential and its device's locator.
func newCredential(random io.Reader) (madeCredential, error) {
	credential, err := randbytes.Base64URLSecret(random, secretBytes)
	if err != nil {
		return madeCredential{}, fmt.Errorf("make a membership credential: %w", err)
	}
	locator, err := randbytes.Base64URLSecret(random, secretBytes)
	if err != nil {
		return madeCredential{}, fmt.Errorf("make a membership's locator: %w", err)
	}
	return madeCredential{credential: credential, locator: locator}, nil
}

// addCredential admits made's devices too, keeping the newest credentialsKeptPerMember.
func (each *member) addCredential(made madeCredential) {
	kept := credential{hash: sha256.Sum256([]byte(made.credential))}
	if made.locator != "" {
		kept.locatorHash = discoverylink.LocatorHash(made.locator)
	}
	each.credentials = append(each.credentials, kept)
	if extra := len(each.credentials) - credentialsKeptPerMember; extra > 0 {
		each.credentials = each.credentials[extra:]
	}
}

// admits is whether a credential whose hash is hash is one of the member's.
func (each *member) admits(hash [sha256.Size]byte) bool {
	isAdmitted := false
	for _, kept := range each.credentials {
		isAdmitted = subtle.ConstantTimeCompare(kept.hash[:], hash[:]) == 1 || isAdmitted
	}
	return isAdmitted
}

// Membership is a session a signed-in person is in, and, for one they joined, a credential for this
// device of theirs.
type Membership struct {
	SessionID int64
	MemberID  int64
	IsHost    bool
	// MembershipCredential is this device's credential for a session the person joined; empty for
	// the one they host, which stays on the device that started it and moves only by HostHere
	// (D442 §1).
	MembershipCredential string
	// Locator is what this device finds the core by through a discovery (Joined.Locator).
	Locator string
	// HostDevices are the person's signed-in devices the session they host is on now, by the
	// sign-in's device (SignedIn.DeviceID); the hosted session's alone.
	HostDevices []int64
}

// Mine is every session userID is in, as its host or as a member who joined signed in. One they
// joined follows them to any device they sign in on, with a new credential for the device that asks,
// which cuts off no other (D267 §2, §3). The one they host stays on the device that started it: it is
// listed with the devices it is on and no credential, so another device of theirs says where it is
// and offers to move it there (HostHere); a credential no device would use is never made (D442 §1).
func (hub *Hub) Mine(userID int64) ([]Membership, error) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	mine := []Membership{}
	isMade := false
	for _, each := range hub.sessions {
		if each.isCast {
			continue // a cast is had back through its receiver and remote, never as a session (cast.go)
		}
		for _, candidate := range each.members {
			if candidate.person == nil || candidate.person.UserID != userID {
				continue
			}
			if candidate.isHost {
				mine = append(mine, Membership{
					SessionID: each.id, MemberID: candidate.id, IsHost: true, HostDevices: candidate.signedInDevices(),
				})
				continue
			}
			made, err := newCredential(hub.random)
			if err != nil {
				return nil, err
			}
			candidate.addCredential(made)
			isMade = true
			mine = append(mine, Membership{
				SessionID: each.id, MemberID: candidate.id,
				MembershipCredential: made.credential, Locator: made.locator,
			})
		}
	}
	if isMade {
		hub.linksChanged()
	}
	return mine, nil
}

// signedInDevices are the person's signed-in devices the member is on, each once, sorted.
func (each *member) signedInDevices() []int64 {
	devices := []int64{}
	for _, attached := range each.devices {
		if attached.signedInDevice != 0 && !slices.Contains(devices, attached.signedInDevice) {
			devices = append(devices, attached.signedInDevice)
		}
	}
	slices.Sort(devices)
	return devices
}

// HostHere moves the session hostUserID hosts to the device that asks, which becomes the host's one
// device: a new credential for it, every other device of the host's told the session moved away and
// cut off, none of their credentials admitting anything more, so the host plays in one place (D267
// §3, D442 §2). A lead of theirs passes on as a gone device's does.
func (hub *Hub) HostHere(sessionID, hostUserID int64) (string, error) {
	made, err := newCredential(hub.random)
	if err != nil {
		return "", err
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hosted, err := hub.hostedSession(sessionID, hostUserID)
	if err != nil {
		return "", err
	}
	for _, host := range hosted.members {
		if !host.isHost {
			continue
		}
		host.cutOff()
		host.addCredential(madeCredential{credential: made.credential}) // the host's has no locator
		now := hub.clock.Now()
		hosted.touch(now)
		hosted.leaderStillHere(now, true, goneRemoved)
		hosted.sendRoster()
		return made.credential, nil
	}
	return "", ErrNoSession
}
