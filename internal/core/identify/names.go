// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"strings"
	"unicode"

	"github.com/kstenerud/orpheum/internal/core/textindex"
)

// How the matcher compares a file's name with a provider's: by the one identity fold (D232), word by
// word, a leading article aside.

// beginsByWords is whether name begins with every word of prefix, and has more: "Wild Rex 2 The
// Road Warrior" begins with "Wild Rex 2", not "Wild Rex" alone being "Wild Rex 2", and "Wings" does
// not begin with "Wing". Words compare as SameWork does.
func beginsByWords(name, prefix string) bool {
	words, start := withoutArticle(wordsOf(name)), withoutArticle(wordsOf(prefix))
	if len(start) == 0 || len(words) <= len(start) {
		return false
	}
	for index, word := range start {
		if textindex.SameWork(word) != textindex.SameWork(words[index]) {
			return false
		}
	}
	return true
}

// isArticle is whether a word is one a name may begin with or not and name the same work: a file's
// "The Girl in the Crate" is TMDB's "Girl in the Crate" (the owner's staging, 2026-10-21). English only:
// the provider is searched in English.
func isArticle(word string) bool {
	switch textindex.SameWork(word) {
	case "the", "a", "an":
		return true
	}
	return false
}

// withoutArticle is a name's words less a leading article, when a word follows it: "The" alone, or
// "A" in "A 1", stays.
func withoutArticle(words []string) []string {
	if len(words) > 1 && isArticle(words[0]) {
		return words[1:]
	}
	return words
}

// workName is a name as the matcher compares it: by the one identity fold (D232), its leading
// article aside.
func workName(name string) string {
	return textindex.SameWork(strings.Join(withoutArticle(wordsOf(name)), " "))
}

// wordsOf is a name's words: what lies between its spaces and its marks, "&" a word of its own.
func wordsOf(name string) []string {
	return strings.FieldsFunc(strings.ReplaceAll(name, "&", " and "), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsNumber(character) && !unicode.Is(unicode.Mc, character)
	})
}
