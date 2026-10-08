// SPDX-License-Identifier: GPL-3.0-or-later

package together

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"

	"github.com/kstenerud/orpheum/internal/platform/randbytes"
)

// credentialsKeptPerMember is how many devices' credentials a member keeps: each device that has the
// membership back adds one, and a page reloaded again and again would otherwise add without end. A
// device whose credential went asks for the membership again (Mine).
const credentialsKeptPerMember = 16

// addCredential admits credential's devices too, keeping the newest credentialsKeptPerMember.
func (each *member) addCredential(credential string) {
	each.credentialHashes = append(each.credentialHashes, sha256.Sum256([]byte(credential)))
	if extra := len(each.credentialHashes) - credentialsKeptPerMember; extra > 0 {
		each.credentialHashes = each.credentialHashes[extra:]
	}
}

// admits is whether a credential whose hash is hash is one of the member's.
func (each *member) admits(hash [sha256.Size]byte) bool {
	isAdmitted := false
	for _, kept := range each.credentialHashes {
		isAdmitted = subtle.ConstantTimeCompare(kept[:], hash[:]) == 1 || isAdmitted
	}
	return isAdmitted
}

// Membership is a session a signed-in person is in, and a credential for this device of theirs.
type Membership struct {
	SessionID            int64
	MemberID             int64
	IsHost               bool
	MembershipCredential string
}

// Mine is every session userID is in, as its host or as a member who joined signed in, each with a
// new credential for the device that asks: a person's session follows them to any device they sign
// in on, and having it back cuts off no other (D267 §2, §3).
func (hub *Hub) Mine(userID int64) ([]Membership, error) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	mine := []Membership{}
	for _, each := range hub.sessions {
		for _, candidate := range each.members {
			if candidate.person == nil || candidate.person.UserID != userID {
				continue
			}
			credential, err := randbytes.Base64URLSecret(hub.random, secretBytes)
			if err != nil {
				return nil, fmt.Errorf("make a membership credential: %w", err)
			}
			candidate.addCredential(credential)
			mine = append(mine, Membership{
				SessionID: each.id, MemberID: candidate.id, IsHost: candidate.isHost,
				MembershipCredential: credential,
			})
		}
	}
	return mine, nil
}
