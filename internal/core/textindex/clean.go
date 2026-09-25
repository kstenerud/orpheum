// SPDX-License-Identifier: GPL-3.0-or-later

package textindex

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The bounds a shown text is held to (D211 §2). They are safety limits, not tastes: no real
// title needs more, and a hostile one is stopped at them.
const (
	// At most this many combining marks on one letter. Vietnamese and Thai stack two or three and
	// Tibetan four, while "Zalgo" text stacks dozens to spill over the lines around it.
	maxMarkRun = 4
	// MaxNameRunes bounds a one-line text: a name, a path, a track title.
	MaxNameRunes = 512
	// MaxProseRunes bounds a paragraph: an overview.
	MaxProseRunes = 10_000
)

// Line is text for one line of a display: a name, a tagline, a path. Controls, line breaks and
// runs of spaces become one space; see Clean for what is removed.
func Line(text string) string {
	return clean(text, false, MaxNameRunes)
}

// Prose is text shown as paragraphs, an overview: line breaks are kept, and at most two in a
// row.
func Prose(text string) string {
	return clean(text, true, MaxProseRunes)
}

// clean is the one display sanitiser (D211 §2). It makes text valid UTF-8 and removes what can
// make a display lie or break: bidi embeddings, overrides and isolates (U+202E turns
// "evil\u202Evkm.exe" into what reads as a video file), invisible characters, C0 and C1
// controls, non-characters, and combining marks past maxMarkRun on one letter; it bounds the
// length. It keeps what real text needs: the left-to-right and right-to-left marks, and the zero
// width joiners (emoji, Persian, Indic scripts).
func clean(text string, prose bool, limit int) string {
	var builder strings.Builder
	builder.Grow(min(len(text), limit*utf8.UTFMax))
	marks, runes, space, breaks := 0, 0, false, 0
	for _, character := range strings.ToValidUTF8(text, "�") {
		if runes >= limit {
			break
		}
		switch {
		case character == '\n' && prose:
			space, breaks = false, breaks+1
			continue
		case unicode.IsSpace(character) || unicode.IsControl(character):
			space = true
			continue
		case removed(character):
			continue
		case unicode.In(character, unicode.Mn, unicode.Me):
			if marks >= maxMarkRun {
				continue
			}
			marks++
		default:
			marks = 0
		}
		switch {
		case breaks > 0 && runes > 0:
			builder.WriteString(strings.Repeat("\n", min(breaks, 2)))
			runes += min(breaks, 2)
		case space && runes > 0:
			builder.WriteByte(' ')
			runes++
		}
		space, breaks = false, 0
		builder.WriteRune(character)
		runes++
	}
	return builder.String()
}

// removed reports the characters a display never shows: bidi embeddings, overrides and isolates,
// invisible formatting, and non-characters.
func removed(character rune) bool {
	switch {
	case character >= 0x202A && character <= 0x202E, // LRE RLE PDF LRO RLO
		character >= 0x2066 && character <= 0x2069,                    // LRI RLI FSI PDI
		character == 0x200B, character == 0x2060, character == 0xFEFF, // zero width space, word joiner, BOM
		character >= 0x2061 && character <= 0x2064, // invisible operators
		character == 0x00AD, character == 0x180E,   // soft hyphen, Mongolian vowel separator
		character >= 0xFDD0 && character <= 0xFDEF,
		character&0xFFFE == 0xFFFE: // non-characters
		return true
	}
	return false
}
