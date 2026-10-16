// SPDX-License-Identifier: AGPL-3.0-or-later

package ptr

// ValueOrZero is what value points to, or its type's zero value when it is nil.
func ValueOrZero[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

// NilIfZero points to value, or is nil when value is its type's zero value: the opposite of
// ValueOrZero, for writing an optional value such as a nullable column.
func NilIfZero[T comparable](value T) *T {
	var zero T
	if value == zero {
		return nil
	}
	return &value
}

// Equal is whether two pointers are both nil, or point to equal values: two nullable columns
// holding the same value, which == on the pointers does not see.
func Equal[T comparable](first, second *T) bool {
	if first == nil || second == nil {
		return first == second
	}
	return *first == *second
}
