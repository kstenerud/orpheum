// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kstenerud/orpheum/discovery/proof"
)

// How a sign-in at the discovery's loader reaches the core's own client origin (D308 §4): the
// loader signs in with the password, the core keeps the new session and answers a code, the loader
// sends the browser on with the code and keeps nothing, and the client origin takes the session by
// the code, once. The session itself never leaves the core until then, and the loader never holds
// it.
const (
	// How long a code works: a redirect's moment, with room for a slow phone. Fixed, not a setting,
	// since it bounds how long a code left in a browser's history means anything.
	handOffLife = time.Minute
	// How many wait at once, a bound on memory: each was a sign-in, which the failure limiter already
	// paces.
	handOffsAtMost = 1000
)

var (
	// ErrNoHandOff is a code no sign-in waits with: taken already, expired, lost to a restart, or
	// never made.
	ErrNoHandOff = errors.New("no sign-in waits with that code; sign in again")
	// ErrTooManyHandOffs is a hand-off asked for while as many wait as may.
	ErrTooManyHandOffs = errors.New("too many sign-ins are waiting to be taken; try again in a minute")
)

// handOffs are the sessions made at a loader's sign-in and not yet taken, by their codes.
type handOffs struct {
	mutex  sync.Mutex
	byCode map[string]handOff
}

// handOff is one session waiting to be taken, when its code was made, and the person a realm
// sign-in names them as (proof.RealmPerson), empty when the core has no realm.
type handOff struct {
	issued Issued
	madeAt time.Time
	person string
}

// expiredLocked removes every hand-off expired at now and returns their sessions; the mutex is held.
func (kept *handOffs) expiredLocked(now time.Time) []Issued {
	expired := []Issued{}
	for code, waiting := range kept.byCode {
		if now.Sub(waiting.madeAt) >= handOffLife {
			expired = append(expired, waiting.issued)
			delete(kept.byCode, code)
		}
	}
	return expired
}

// HandOff signs a person in as SignIn does, with its limits, and keeps the session for the device's
// client origin to take by the code it answers, once, within handOffLife.
func (service *Service) HandOff(
	ctx context.Context, name, password, clientAddress string, device NewDevice,
) (string, error) {
	issued, err := service.SignIn(ctx, name, password, clientAddress, device)
	if err != nil {
		return "", err
	}
	person := ""
	if realm := service.currentRealm(); realm != "" {
		person = proof.RealmPerson(realm, name)
	}
	return service.keepHandOff(ctx, issued, person)
}

// keepHandOff keeps issued for a client origin to take by the code it answers, once, within
// handOffLife, for person: whom the bootstrap token the client origin holds names, by whom its offers
// are proven under the code (HandOffOfferKeys). The session ends if it cannot be kept.
func (service *Service) keepHandOff(ctx context.Context, issued Issued, person string) (string, error) {
	code, err := randomSecret(service.random)
	if err != nil {
		return "", errors.Join(err, service.SignOut(ctx, issued.Token))
	}
	service.handOffs.mutex.Lock()
	expired := service.handOffs.expiredLocked(service.clock.Now())
	isFull := len(service.handOffs.byCode) >= handOffsAtMost
	if !isFull {
		service.handOffs.byCode[code] = handOff{issued: issued, madeAt: service.clock.Now(), person: person}
	}
	service.handOffs.mutex.Unlock()
	err = service.endUntaken(ctx, expired)
	if isFull {
		return "", errors.Join(ErrTooManyHandOffs, err, service.SignOut(ctx, issued.Token))
	}
	return code, err
}

// TakeHandOff is the session a hand-off's code keeps, once: the code then works no more.
// ErrNoHandOff for a code taken, expired or unknown.
func (service *Service) TakeHandOff(ctx context.Context, code string) (Issued, error) {
	service.handOffs.mutex.Lock()
	expired := service.handOffs.expiredLocked(service.clock.Now())
	waiting, found := service.handOffs.byCode[code]
	delete(service.handOffs.byCode, code)
	service.handOffs.mutex.Unlock()
	if err := service.endUntaken(ctx, expired); err != nil {
		return Issued{}, err
	}
	if !found {
		return Issued{}, ErrNoHandOff
	}
	return waiting.issued, nil
}

// HandOffOfferKeys are the keys a client origin holding a live hand-off's code for person proves its
// offers under (proof.HandOffOfferKey), which the discovery passes on under the person, as it does a
// realm sign-in's: so a client origin reached only over WebRTC can take its session (D308 §4).
func (service *Service) HandOffOfferKeys(person string) [][]byte {
	service.handOffs.mutex.Lock()
	defer service.handOffs.mutex.Unlock()
	now := service.clock.Now()
	keys := [][]byte{}
	for code, waiting := range service.handOffs.byCode {
		if person != "" && waiting.person == person && now.Sub(waiting.madeAt) < handOffLife {
			keys = append(keys, proof.HandOffOfferKey(code))
		}
	}
	return keys
}

// endUntaken signs out the sessions whose hand-offs expired untaken, which no one holds.
func (service *Service) endUntaken(ctx context.Context, expired []Issued) error {
	var errs []error
	for _, issued := range expired {
		errs = append(errs, service.SignOut(ctx, issued.Token))
	}
	return errors.Join(errs...)
}
