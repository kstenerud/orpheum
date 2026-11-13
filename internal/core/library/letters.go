// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"bytes"
	"context"

	"github.com/kstenerud/orpheum/internal/core/textindex"
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

// railStarts are the sort keys the rail's letters start at, by their place in RailLetters: none for
// #, which is before A.
func railStarts(sorter *textindex.Sorter) [][]byte {
	letters := RailLetters()
	starts := make([][]byte, len(letters))
	for index, letter := range letters[1:] {
		starts[index+1] = sorter.Key(letter)
	}
	return starts
}

// withLetters gives each entry of a list by name or by file name the rail's letter it is under, by
// its key: the letter a jump to it starts from, for the rail to stand at beside its poster.
func (browser *Browser) withLetters(ctx context.Context, keyed []keyedEntry) error {
	sorter, err := browser.sorter(ctx)
	if err != nil {
		return err
	}
	letters, starts := RailLetters(), railStarts(sorter)
	for index := range keyed {
		keyed[index].entry.Letter = letters[letterUnder(starts, keyed[index].key)]
	}
	return nil
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
