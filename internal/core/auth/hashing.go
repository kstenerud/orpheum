// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import "context"

// The service's password work, each hash in a hash slot (password.go). Kept apart from password.go,
// which the paid service carries as a twin and has no Service of its own to hang these on.

// hashPassword is HashPassword in a hash slot.
func (service *Service) hashPassword(
	ctx context.Context, password string, parameters Argon2Parameters,
) (string, error) {
	var (
		hashed string
		err    error
	)
	hash := func() { hashed, err = HashPassword(password, parameters, service.random) }
	if slotErr := service.hashing.run(ctx, hash); slotErr != nil {
		return "", slotErr
	}
	return hashed, err
}

// verifyPassword is VerifyPassword in a hash slot.
func (service *Service) verifyPassword(
	ctx context.Context, password, hash string,
) (isMatch bool, hashParameters Argon2Parameters, err error) {
	verify := func() { isMatch, hashParameters, err = VerifyPassword(password, hash) }
	if slotErr := service.hashing.run(ctx, verify); slotErr != nil {
		return false, Argon2Parameters{}, slotErr
	}
	return isMatch, hashParameters, err
}
