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
// runs of spaces become one space; see clean for what is removed.
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
func clean(text string, isProse bool, maxRunes int) string {
	var builder strings.Builder
	builder.Grow(min(len(text), maxRunes*utf8.UTFMax))
	marks, runes, isSpacePending, pendingBreaks := 0, 0, false, 0
	for _, character := range strings.ToValidUTF8(text, "�") {
		if runes >= maxRunes {
			break
		}
		switch {
		case character == '\n' && isProse:
			isSpacePending, pendingBreaks = false, pendingBreaks+1
			continue
		case unicode.IsSpace(character) || unicode.IsControl(character):
			isSpacePending = true
			continue
		case isNeverShown(character):
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
		case pendingBreaks > 0 && runes > 0:
			builder.WriteString(strings.Repeat("\n", min(pendingBreaks, 2)))
			runes += min(pendingBreaks, 2)
		case isSpacePending && runes > 0:
			builder.WriteByte(' ')
			runes++
		}
		isSpacePending, pendingBreaks = false, 0
		builder.WriteRune(character)
		runes++
	}
	return builder.String()
}

// isNeverShown reports whether a character is one a display never shows: bidi embeddings,
// overrides and isolates, invisible formatting, and non-characters.
func isNeverShown(character rune) bool {
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

// Path is a filesystem path as the library shows and relates it (D211 §2): what Line removes is
// replaced with U+FFFD instead, so a path that had something hidden in it shows that it did, and
// nothing else changes — no space collapsed, no length cut — so a path still begins with its
// root. The bytes that open the file are kept apart, as bytes (principles §22).
func Path(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	marks := 0
	for _, character := range strings.ToValidUTF8(text, "�") {
		switch {
		case (unicode.IsControl(character) && character != ' ') || isNeverShown(character):
			builder.WriteRune('�')
			marks = 0
			continue
		case unicode.In(character, unicode.Mn, unicode.Me):
			if marks >= maxMarkRun {
				continue
			}
			marks++
		default:
			marks = 0
		}
		builder.WriteRune(character)
	}
	return builder.String()
}
