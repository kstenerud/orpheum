// SPDX-License-Identifier: GPL-3.0-or-later

package textindex

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Route is which index a search asks (D48).
type Route string

// The routes: a short query scans the folded names for a substring; a Latin query of three characters or more asks the
// word index, which ranks and matches prefixes; any other asks the trigram index, which finds
// substrings in any script.
const (
	RouteShort   Route = "short"
	RouteWords   Route = "words"
	RouteTrigram Route = "trigram"
)

// minTrigramRunes is the length under which the trigram index cannot answer: it needs three
// characters (D48's measured threshold).
const minTrigramRunes = 3

// Plan is a search, folded and routed.
type Plan struct {
	Route Route
	// IndexQuery is the index's query: a folded substring, an FTS5 word query, or a trigram phrase.
	IndexQuery string
}

// PlanQuery folds a person's query with the one normaliser and routes it. An empty plan
// (IndexQuery "") asks for nothing.
func PlanQuery(query string) Plan {
	folded := strings.TrimSpace(Fold(query))
	if folded == "" {
		return Plan{}
	}
	if utf8.RuneCountInString(folded) < minTrigramRunes {
		return Plan{Route: RouteShort, IndexQuery: folded}
	}
	if !isLatin(folded) {
		return Plan{Route: RouteTrigram, IndexQuery: quote(folded)}
	}
	var terms []string
	for _, word := range strings.FieldsFunc(folded, isNotWordRune) {
		// Each word as itself or as a prefix: the same matches as a bare prefix, but bm25 ranks a
		// whole word above a word it begins (D48, from Navidrome).
		terms = append(terms, "("+quote(word)+" OR "+quote(word)+"*)")
	}
	if len(terms) == 0 {
		return Plan{}
	}
	// FTS5 joins bare words with an implicit AND, but not parenthesised groups: they need it said.
	return Plan{Route: RouteWords, IndexQuery: strings.Join(terms, " AND ")}
}

// isLatin reports whether every letter is Latin: the word index tokenises Latin words and
// fails on CJK and Thai, which it takes as one token or shreds (D48).
func isLatin(text string) bool {
	for _, character := range text {
		if unicode.IsLetter(character) && !unicode.Is(unicode.Latin, character) {
			return false
		}
	}
	return true
}

func isNotWordRune(character rune) bool {
	return !unicode.IsLetter(character) && !unicode.IsNumber(character)
}

// quote makes text one FTS5 string: operators and punctuation inside it are text.
func quote(text string) string {
	return `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
}
