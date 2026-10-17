// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"errors"
	"sync"
	"time"
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

// handOff is one session waiting to be taken, and when its code was made.
type handOff struct {
	issued Issued
	madeAt time.Time
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
	code, err := randomSecret(service.random)
	if err != nil {
		return "", errors.Join(err, service.SignOut(ctx, issued.Token))
	}
	service.handOffs.mutex.Lock()
	expired := service.handOffs.expiredLocked(service.clock.Now())
	isFull := len(service.handOffs.byCode) >= handOffsAtMost
	if !isFull {
		service.handOffs.byCode[code] = handOff{issued: issued, madeAt: service.clock.Now()}
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

// endUntaken signs out the sessions whose hand-offs expired untaken, which no one holds.
func (service *Service) endUntaken(ctx context.Context, expired []Issued) error {
	var errs []error
	for _, issued := range expired {
		errs = append(errs, service.SignOut(ctx, issued.Token))
	}
	return errors.Join(errs...)
}
