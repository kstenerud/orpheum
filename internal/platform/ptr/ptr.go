// SPDX-License-Identifier: GPL-3.0-or-later

package ptr

// ValueOrZero is what value points to, or its type's zero value when it is nil.
func ValueOrZero[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}
