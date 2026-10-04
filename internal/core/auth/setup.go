// SPDX-License-Identifier: GPL-3.0-or-later

package auth

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/kstenerud/orpheum/internal/platform/randbytes"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// SetupCodeFileName is the setup code's file in the instance's run directory, mode 0600, which
// `orpheum setup-code` prints. It exists only until the first admin is created.
const SetupCodeFileName = "setup-code"

// setupCodeChars is a setup code's length in safe32, as a device code is written (D223 §2): 60
// bits, like 7kqm-2xhp-9rtw, read however a person types it from the log.
const setupCodeChars = 12

var (
	// ErrSetupComplete is returned once any user exists: setup creates only the first admin.
	ErrSetupComplete = errors.New("setup is already complete")
	// ErrWrongSetupCode is returned for a code that is not the one in the log.
	ErrWrongSetupCode = errors.New("the setup code is wrong")
)

// SetupCodes keeps the pending setup code between runs and for `orpheum setup-code`. The state
// directory implements it; this package never touches a file.
type SetupCodes interface {
	// Read returns the stored code, or an error wrapping ErrNoSetupCode.
	Read() (string, error)
	Write(code string) error
	Remove() error
}

// ErrNoSetupCode is returned by SetupCodes.Read when no code is stored.
var ErrNoSetupCode = errors.New("no setup code is stored")

// EnsureSetupCode returns the code that gates the first-run wizard while no user exists,
// creating and storing one if there is none, so nobody else on the LAN can claim the admin
// account (audit-2026-09-23 §7). It returns "" once any user exists.
func (service *Service) EnsureSetupCode(ctx context.Context, codes SetupCodes) (string, error) {
	count, err := service.CountUsers(ctx)
	if err != nil {
		return "", err
	}
	if count > 0 {
		return "", codes.Remove()
	}
	code, err := codes.Read()
	if err != nil && !errors.Is(err, ErrNoSetupCode) {
		return "", fmt.Errorf("read the setup code: %w", err)
	}
	// A code an earlier release stored in another alphabet is replaced: safe32 cannot read it.
	if canonical, readErr := readSafe32(code, setupCodeChars); err == nil && readErr == nil && canonical == code {
		return code, nil
	}
	code, err = newSetupCode(service.random)
	if err != nil {
		return "", err
	}
	if err = codes.Write(code); err != nil {
		return "", fmt.Errorf("store the setup code: %w", err)
	}
	return code, nil
}

// CompleteSetup creates the first admin if code matches the stored one and no user exists yet,
// then removes the code. Checking for users and inserting happen in one write transaction, so two
// racing requests cannot both succeed.
func (service *Service) CompleteSetup(
	ctx context.Context, codes SetupCodes, code string, admin NewUser,
) (int64, error) {
	stored, err := codes.Read()
	if errors.Is(err, ErrNoSetupCode) {
		return 0, ErrSetupComplete
	}
	if err != nil {
		return 0, fmt.Errorf("read the setup code: %w", err)
	}
	given, err := readSafe32(code, setupCodeChars)
	if err != nil || subtle.ConstantTimeCompare([]byte(given), []byte(stored)) != 1 {
		return 0, ErrWrongSetupCode
	}
	admin.Role = RoleAdmin
	var id int64
	err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		count, countErr := countUsers(ctx, queries)
		if countErr != nil {
			return countErr
		}
		if count > 0 {
			return ErrSetupComplete
		}
		var insertErr error
		id, insertErr = service.insertUser(ctx, queries, admin)
		return insertErr
	})
	if err != nil {
		return 0, err
	}
	return id, codes.Remove()
}

// newSetupCode is a new setup code: 60 random bits in safe32.
func newSetupCode(random io.Reader) (string, error) {
	raw, err := randbytes.Read(random, 8)
	if err != nil {
		return "", fmt.Errorf("make a setup code: %w", err)
	}
	return safe32Code(binary.BigEndian.Uint64(raw), setupCodeChars), nil
}
