// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"strings"
	"unicode"

	"github.com/kstenerud/orpheum/internal/core/textindex"
)

// How the matcher compares a file's name with a provider's: by the one identity fold (D232), word by
// word, a leading article aside.

// beginsByWords is whether name begins with every word of prefix, and has more: "Show 2 The
// Return" begins with "Show 2", not "Show" alone being "Show 2", and "Shows" does
// not begin with "Show". Words compare as SameWork does.
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
// "The Show" is TMDB's "Show" (the owner's staging, 2026-10-21). English only:
// the provider is searched in English.
func isArticle(word string) bool {
	return textindex.IsArticle("en", word)
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

// nearlyNamed is the one candidate whose name is the query's but for a letter or two, in its year
// when the query has one: a file's misspelling, a letter of a long name dropped (the owner,
// 2026-10-21). It is always a guess, marked as one for a person to confirm (D301), and with several
// so named, none: a person's choice.
func nearlyNamed(query Query, candidates []Candidate) (Candidate, bool) {
	want := workName(query.Name)
	most := lettersOffAtMost(want)
	if most == 0 {
		return Candidate{}, false
	}
	var found []Candidate
	for _, candidate := range candidates {
		near := func(name string) bool {
			named := workName(name)
			return name != "" && !isPlural(named, want) && lettersOff(named, want) <= most
		}
		isNear := near(candidate.Name) || near(candidate.OriginalName)
		if isNear && (query.Year == 0 || candidate.Year == query.Year) {
			found = append(found, candidate)
		}
	}
	if len(found) != 1 {
		return Candidate{}, false
	}
	return found[0], true
}

// lettersOffAtMost is how many letters a name may be off by and still be nearly the name: none for a
// short one, where a letter makes another word ("Wing", "Wine"; "Shows", "Show"), one from eight
// letters, two from fourteen.
func lettersOffAtMost(name string) int {
	switch length := len([]rune(name)); {
	case length >= 14:
		return 2
	case length >= 8:
		return 1
	}
	return 0
}

// isPlural is whether one name is the other with an s after it: a sequel's name, "Shows" after
// "Show", never a misspelling.
func isPlural(first, second string) bool {
	return first+"s" == second || second+"s" == first
}

// lettersOff is how many letters must be added, removed or changed to make one name the other (the
// Levenshtein distance).
func lettersOff(first, second string) int {
	firstRunes, secondRunes := []rune(first), []rune(second)
	previous := make([]int, len(secondRunes)+1)
	for index := range previous {
		previous[index] = index
	}
	for row, letter := range firstRunes {
		current := make([]int, len(secondRunes)+1)
		current[0] = row + 1
		for column, other := range secondRunes {
			cost := 1
			if letter == other {
				cost = 0
			}
			current[column+1] = min(previous[column+1]+1, current[column]+1, previous[column]+cost)
		}
		previous = current
	}
	return previous[len(secondRunes)]
}
