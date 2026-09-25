// SPDX-License-Identifier: GPL-3.0-or-later

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
