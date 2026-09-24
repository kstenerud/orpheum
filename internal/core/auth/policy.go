// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
)

// Settings is where auth reads its opinions: internal/core/settings in production. There is no
// value in code to fall back on; a missing setting is an error (D200).
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// saltBytes and keyBytes are argon2id's output format, not costs: they stay constants.
const (
	saltBytes = 16
	keyBytes  = 32
)

// parameters reads argon2id's costs.
func (service *Service) parameters(ctx context.Context) (Parameters, error) {
	memory, err := service.bounded(ctx, settings.Argon2MemoryKiB, math.MaxUint32)
	if err != nil {
		return Parameters{}, err
	}
	iterations, err := service.bounded(ctx, settings.Argon2Iterations, math.MaxUint32)
	if err != nil {
		return Parameters{}, err
	}
	parallelism, err := service.bounded(ctx, settings.Argon2Parallelism, math.MaxUint8)
	if err != nil {
		return Parameters{}, err
	}
	// Each value was checked against its type's maximum by bounded, so none of these can overflow.
	return Parameters{
		MemoryKiB:   uint32(memory),     //nolint:gosec // G115: bounded to MaxUint32 above
		Iterations:  uint32(iterations), //nolint:gosec // G115: bounded to MaxUint32 above
		Parallelism: uint8(parallelism), //nolint:gosec // G115: bounded to MaxUint8 above
		SaltBytes:   saltBytes, KeyBytes: keyBytes,
	}, nil
}

// failurePolicy reads the failed sign-in limit and its window.
func (service *Service) failurePolicy(ctx context.Context) (int64, time.Duration, error) {
	limit, err := service.settings.Int(ctx, settings.SignInFailureLimit)
	if err != nil {
		return 0, 0, err
	}
	windowSeconds, err := service.settings.Int(ctx, settings.SignInFailureWindowS)
	if err != nil {
		return 0, 0, err
	}
	return limit, time.Duration(windowSeconds) * time.Second, nil
}

// sessionPolicy reads how long a session may go unused, and how often its use is written.
func (service *Service) sessionPolicy(ctx context.Context) (idleExpiry, lastSeenResolution time.Duration, err error) {
	idleHours, err := service.settings.Int(ctx, settings.SessionIdleExpiryH)
	if err != nil {
		return 0, 0, err
	}
	resolutionSeconds, err := service.settings.Int(ctx, settings.SessionLastSeenResolutionS)
	if err != nil {
		return 0, 0, err
	}
	return time.Duration(idleHours) * time.Hour, time.Duration(resolutionSeconds) * time.Second, nil
}

// bounded reads an integer setting and refuses one outside [0, maximum], so the conversion to a
// narrower type that follows cannot overflow.
func (service *Service) bounded(ctx context.Context, key settings.Key, maximum uint64) (uint64, error) {
	value, err := service.settings.Int(ctx, key)
	if err != nil {
		return 0, err
	}
	if value < 0 || uint64(value) > maximum {
		return 0, fmt.Errorf("setting %s is %d, outside 0..%d", key, value, maximum)
	}
	return uint64(value), nil
}
