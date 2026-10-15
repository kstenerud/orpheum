// SPDX-License-Identifier: AGPL-3.0-or-later

package querytally

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
)

// Tally counts the database statements a unit of work runs, by query name, and its write
// transactions: a request's, so its count can be checked not to grow with the library and named
// when it is slow (D303, DL6, DL17). Safe for concurrent use.
type Tally struct {
	mutex   sync.Mutex
	byQuery map[string]int
	writes  int
}

// tallyKey carries a Tally on a context.
type tallyKey struct{}

// New returns ctx counting into a new Tally, and the Tally.
func New(ctx context.Context) (context.Context, *Tally) {
	tally := &Tally{byQuery: map[string]int{}}
	return context.WithValue(ctx, tallyKey{}, tally), tally
}

// Of is the Tally ctx counts into, or nil.
func Of(ctx context.Context) *Tally {
	tally, _ := ctx.Value(tallyKey{}).(*Tally) //nolint:errcheck // absent is nil
	return tally
}

// Statements is how many statements were run.
func (tally *Tally) Statements() int {
	tally.mutex.Lock()
	defer tally.mutex.Unlock()
	total := 0
	for _, count := range tally.byQuery {
		total += count
	}
	return total
}

// Writes is how many write transactions were run.
func (tally *Tally) Writes() int {
	tally.mutex.Lock()
	defer tally.mutex.Unlock()
	return tally.writes
}

// String names each query run and how often, the most run first: "GetLiveGrant×1 ListTitles×1".
func (tally *Tally) String() string {
	tally.mutex.Lock()
	defer tally.mutex.Unlock()
	names := slices.SortedFunc(maps.Keys(tally.byQuery), func(a, b string) int {
		if tally.byQuery[a] != tally.byQuery[b] {
			return tally.byQuery[b] - tally.byQuery[a]
		}
		return strings.Compare(a, b)
	})
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s×%d", name, tally.byQuery[name]))
	}
	return strings.Join(parts, " ")
}

// Count records one statement, named by its sqlc header ("-- name: GetLiveGrant :one"), or
// "unnamed".
func (tally *Tally) Count(query string) {
	name := "unnamed"
	if rest, found := strings.CutPrefix(query, "-- name: "); found {
		name, _, _ = strings.Cut(rest, " ")
	}
	tally.mutex.Lock()
	defer tally.mutex.Unlock()
	tally.byQuery[name]++
}

// CountWrite records one write transaction.
func (tally *Tally) CountWrite() {
	tally.mutex.Lock()
	defer tally.mutex.Unlock()
	tally.writes++
}
