// SPDX-License-Identifier: AGPL-3.0-or-later

package errs

import "fmt"

// WrapIf wraps err with what was being done, or is nil when err is.
func WrapIf(doing string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", doing, err)
}
