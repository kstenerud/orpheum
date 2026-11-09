// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// Settings is where auth reads its opinions: internal/core/settings in production. There is no
// value in code to fall back on; a missing setting is healed with its starting value, written to
// the database (D200, D202).
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
}

// saltBytes and keyBytes are argon2id's output format, not costs: they stay constants.
const (
	saltBytes = 16
	keyBytes  = 32
)

// argon2Parameters reads argon2id's costs.
func (service *Service) argon2Parameters(ctx context.Context) (Argon2Parameters, error) {
	memoryKiB, err := service.boundedSetting(ctx, settings.Argon2MemoryKiB, math.MaxUint32)
	if err != nil {
		return Argon2Parameters{}, err
	}
	iterations, err := service.boundedSetting(ctx, settings.Argon2Iterations, math.MaxUint32)
	if err != nil {
		return Argon2Parameters{}, err
	}
	parallelism, err := service.boundedSetting(ctx, settings.Argon2Parallelism, math.MaxUint8)
	if err != nil {
		return Argon2Parameters{}, err
	}
	// Each value was checked against its type's maximum by bounded, so none of these can overflow.
	return Argon2Parameters{
		MemoryKiB:   uint32(memoryKiB),  //nolint:gosec // G115: bounded to MaxUint32 above
		Iterations:  uint32(iterations), //nolint:gosec // G115: bounded to MaxUint32 above
		Parallelism: uint8(parallelism), //nolint:gosec // G115: bounded to MaxUint8 above
		SaltBytes:   saltBytes, KeyBytes: keyBytes,
	}, nil
}

// failurePolicy reads the failed sign-in limit and its window.
func (service *Service) failurePolicy(ctx context.Context) (int64, time.Duration, error) {
	maxFailures, err := service.settings.Int(ctx, settings.SignInFailureLimit)
	if err != nil {
		return 0, 0, err
	}
	window, err := service.settings.Duration(ctx, settings.SignInFailureWindowSeconds)
	if err != nil {
		return 0, 0, err
	}
	return maxFailures, window, nil
}

// failureStretch is how far the failed sign-in limit stretches beyond one person on one network: a
// network's limit is network times it, a household or a carrier's shared address being many people's
// failures; and a person everywhere is held to everywhere times it, past which each sign-in waits
// pace, slowing guessing spread across many networks without letting a stranger lock them out by
// their name alone (the second audit's K2, the third's A13).
type failureStretch struct {
	network, everywhere int64
	pace                time.Duration
}

// failureStretchOf reads the stretch's settings.
func (service *Service) failureStretchOf(ctx context.Context) (failureStretch, error) {
	network, err := service.settings.Int(ctx, settings.SignInFailureLimitNetworkMultiple)
	if err != nil {
		return failureStretch{}, err
	}
	everywhere, err := service.settings.Int(ctx, settings.SignInFailureLimitEverywhereMultiple)
	if err != nil {
		return failureStretch{}, err
	}
	pace, err := service.settings.Duration(ctx, settings.SignInEverywherePaceSeconds)
	if err != nil {
		return failureStretch{}, err
	}
	return failureStretch{network: network, everywhere: everywhere, pace: pace}, nil
}

// answerFailureAt waits until signinFailureAnswerSeconds after began; a setting that cannot be read
// waits nothing, the failure being told all the same.
func (service *Service) answerFailureAt(ctx context.Context, began time.Time) {
	answer, err := service.settings.Duration(ctx, settings.SignInFailureAnswerSeconds)
	if err != nil {
		return
	}
	if wait := began.Add(answer).Sub(service.clock.Now()); wait > 0 {
		_ = service.clock.Sleep(ctx, wait) //nolint:errcheck // a request that ended needs no answer
	}
}

// sessionIdleExpiry reads how long a session may go unused.
func (service *Service) sessionIdleExpiry(ctx context.Context) (time.Duration, error) {
	idleExpiry, err := service.settings.Duration(ctx, settings.SessionIdleExpiryHours)
	if err != nil {
		return 0, err
	}
	return idleExpiry, nil
}

// sessionLocatorExpiry is how long a session may go unused before its device's locator stops
// leading a discovery to it: the idle expiry, and one flush interval more, since a use may wait
// that long in memory before it is written (FlushSessionUses). Authenticate itself reads the
// remembered use, so a session past its idle expiry is refused all the same.
func (service *Service) sessionLocatorExpiry(ctx context.Context) (time.Duration, error) {
	idleExpiry, err := service.sessionIdleExpiry(ctx)
	if err != nil {
		return 0, err
	}
	flushEvery, err := service.settings.Duration(ctx, settings.SessionLastUsedWriteIntervalSeconds)
	if err != nil {
		return 0, err
	}
	return idleExpiry + flushEvery, nil
}

// boundedSetting reads an integer setting and refuses one outside [0, maximum], so the conversion to a
// narrower type that follows cannot overflow.
func (service *Service) boundedSetting(ctx context.Context, key settings.Key, maximum uint64) (uint64, error) {
	value, err := service.settings.Int(ctx, key)
	if err != nil {
		return 0, err
	}
	if value < 0 || uint64(value) > maximum {
		return 0, fmt.Errorf("setting %s is %d, outside 0..%d", key, value, maximum)
	}
	return uint64(value), nil
}
