// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"

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

// Membership is a session a signed-in person is in, and a credential for this device of theirs.
type Membership struct {
	SessionID            int64
	MemberID             int64
	IsHost               bool
	MembershipCredential string
	// Locator is what this device finds the core by through a discovery (Joined.Locator).
	Locator string
}

// Mine is every session userID is in, as its host or as a member who joined signed in, each with a
// new credential for the device that asks: a person's session follows them to any device they sign
// in on, and having it back cuts off no other (D267 §2, §3).
func (hub *Hub) Mine(userID int64) ([]Membership, error) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	mine := []Membership{}
	for _, each := range hub.sessions {
		if each.isCast {
			continue // a cast is had back through its receiver and remote, never as a session (cast.go)
		}
		for _, candidate := range each.members {
			if candidate.person == nil || candidate.person.UserID != userID {
				continue
			}
			made, err := newCredential(hub.random)
			if err != nil {
				return nil, err
			}
			if candidate.isHost {
				made.locator = "" // the host's device finds the core by their own session's locator
			}
			candidate.addCredential(made)
			mine = append(mine, Membership{
				SessionID: each.id, MemberID: candidate.id, IsHost: candidate.isHost,
				MembershipCredential: made.credential, Locator: made.locator,
			})
		}
	}
	if len(mine) > 0 {
		hub.linksChanged()
	}
	return mine, nil
}
