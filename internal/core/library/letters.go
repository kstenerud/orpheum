// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"bytes"
	"context"
	"fmt"
)

// RailLetters are the alphabet rail's letters, in order: # for what sorts before A, then A to Z. A
// title is under the last letter its key in the list's order is at or after.
func RailLetters() []string {
	letters := []string{"#"}
	for letter := 'A'; letter <= 'Z'; letter++ {
		letters = append(letters, string(letter))
	}
	return letters
}

// Letters are the rail's letters a list by name or by file name has a title under, in the
// sort_locale's order, so the rail offers only those that lead somewhere (D349 §9). It asks for the
// first title from each letter that has one, never the list: a read for each letter with a title,
// and one more.
func (browser *Browser) Letters(ctx context.Context, query ListQuery) ([]string, error) {
	if !isByName(query.Sort) {
		return nil, fmt.Errorf("letters of %s by %s: %w", query.Kind, query.Sort, ErrNoSuchSort)
	}
	sorter, err := browser.sorter(ctx)
	if err != nil {
		return nil, err
	}
	letters := RailLetters()
	starts := make([][]byte, len(letters))
	for index, letter := range letters[1:] {
		starts[index+1] = sorter.Key(letter)
	}
	// A list's letters are the same whichever way it runs: they are read A to Z (D366 §4).
	query.Limit, query.Cursor, query.From, query.Order = 1, "", "", OrderAscending
	found := []string{}
	after := listPosition{key: []byte{}, id: 0}
	for {
		first, listErr := browser.firstFrom(ctx, query, after)
		if listErr != nil || first == nil {
			return found, listErr
		}
		under := letterUnder(starts, first.key)
		found = append(found, letters[under])
		if under+1 == len(letters) {
			return found, nil
		}
		after = listPosition{key: starts[under+1], id: -1}
	}
}

// firstFrom is the first entry after a place in a list's order, or nil when there is none.
func (browser *Browser) firstFrom(ctx context.Context, query ListQuery, after listPosition) (*keyedEntry, error) {
	kinds := []string{query.Kind}
	if query.Kind == ListAny {
		kinds = []string{ListFilms, ListSeries, ListOther}
	}
	var first *keyedEntry
	for _, kind := range kinds {
		keyed, err := browser.listKind(ctx, kind, query, after)
		if err != nil {
			return nil, fmt.Errorf("the first %s after a letter: %w", kind, err)
		}
		if len(keyed) > 0 && (first == nil || bytes.Compare(keyed[0].key, first.key) < 0) {
			first = &keyed[0]
		}
	}
	return first, nil
}

// letterUnder is the place of the letter a key is under: the last whose start it is at or after,
// # (0) before A.
func letterUnder(starts [][]byte, key []byte) int {
	under := 0
	for index := 1; index < len(starts); index++ {
		if bytes.Compare(key, starts[index]) >= 0 {
			under = index
		}
	}
	return under
}
