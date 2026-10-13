// SPDX-License-Identifier: AGPL-3.0-or-later

package textindex

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// atomicFold spells out the letters that are one code point with no decomposition, so no
// Unicode option removes their "diacritic": Ø is not O with a stroke to Unicode. They fold as a
// reader types them (D48). It returns "" for any other letter.
func atomicFold(character rune) string {
	switch character {
	case 'ø', 'Ø':
		return "o"
	case 'æ', 'Æ':
		return "ae"
	case 'œ', 'Œ':
		return "oe"
	case 'ß', 'ẞ':
		return "ss"
	case 'ð', 'Ð', 'đ', 'Đ':
		return "d"
	case 'ł', 'Ł':
		return "l"
	case 'þ', 'Þ':
		return "th"
	case 'ı':
		return "i"
	case 'ħ', 'Ħ':
		return "h"
	case 'ŧ', 'Ŧ':
		return "t"
	case 'ŋ', 'Ŋ':
		return "ng"
	}
	return ""
}

// Fold is the one normaliser (D48): compatibility-decomposed, combining marks dropped, atomic
// letters spelled out, lower case. Every script's letters survive it; only marks go. What is
// indexed and what is searched for both pass through it, and nothing else folds text for
// search.
func Fold(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	for _, character := range norm.NFKD.String(text) {
		switch {
		case unicode.Is(unicode.Mn, character):
		case atomicFold(character) != "":
			builder.WriteString(atomicFold(character))
		default:
			builder.WriteRune(unicode.ToLower(character))
		}
	}
	return builder.String()
}

// SameWork is the one identity fold (D232): what makes two names name the same work, and every
// comparison of a work's, a series' or an extra's name goes through it. It is Fold — compatibility
// forms, case, combining marks and atomic letters folded as search folds them (D48) — keeping
// only letters, numbers and the spacing marks some scripts write vowels with, so punctuation and
// spacing, which files and providers write each their own way, say nothing: "Lantern's Agents"
// and "Lanterns Agents", "Moth-Lantern" and "Moth Lantern", "Fièvre" and "FIEVRE" are one work. An
// ampersand is the word it stands for: "Wit & Wilfulness" is "Wit and Wilfulness" (the owner's
// NAS, 2026-10-13, where TMDB's single "Wit & Wilfulness" of 2005 waited for a person's choice).
func SameWork(name string) string {
	folded := Fold(strings.ReplaceAll(name, "&", " and "))
	var builder strings.Builder
	builder.Grow(len(folded))
	for _, character := range folded {
		if unicode.IsLetter(character) || unicode.IsNumber(character) || unicode.Is(unicode.Mc, character) {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}
