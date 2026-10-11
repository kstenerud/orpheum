// SPDX-License-Identifier: AGPL-3.0-or-later

package authtest

import "github.com/kstenerud/orpheum/internal/core/auth"

var _ auth.SetupCodes = (*SetupCodes)(nil)

// SetupCodes is the run directory's setup-code file, in memory. Code is what it holds; empty
// is no file.
type SetupCodes struct {
	Code string
}

// Read returns the stored code, or auth.ErrNoSetupCode when there is none.
func (codes *SetupCodes) Read() (string, error) {
	if codes.Code == "" {
		return "", auth.ErrNoSetupCode
	}
	return codes.Code, nil
}

// Write stores code.
func (codes *SetupCodes) Write(code string) error {
	codes.Code = code
	return nil
}

// Remove forgets the stored code.
func (codes *SetupCodes) Remove() error {
	codes.Code = ""
	return nil
}
