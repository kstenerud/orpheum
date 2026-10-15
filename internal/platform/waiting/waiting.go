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

// readingKey marks a context whose request only reads.
type readingKey struct{}

// ForAReader marks ctx as a request that only reads, a GET or a HEAD: nothing it does should take
// the database's writer (D303, DL10), and the store reports what does.
func ForAReader(ctx context.Context) context.Context {
	return context.WithValue(ctx, readingKey{}, true)
}

// IsAReader is whether ctx was marked as a request that only reads, or derives from one.
func IsAReader(ctx context.Context) bool {
	marked, isBool := ctx.Value(readingKey{}).(bool)
	return isBool && marked
}
