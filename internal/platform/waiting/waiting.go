// SPDX-License-Identifier: AGPL-3.0-or-later

package waiting

import "context"

// personKey is the mark's key.
type personKey struct{}

// ForAPerson marks ctx as one a person waits on.
func ForAPerson(ctx context.Context) context.Context {
	return context.WithValue(ctx, personKey{}, true)
}

// IsAPerson is whether a person waits on ctx: whether ForAPerson marked it or what it derives from.
func IsAPerson(ctx context.Context) bool {
	marked, isBool := ctx.Value(personKey{}).(bool)
	return isBool && marked
}
