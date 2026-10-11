// SPDX-License-Identifier: AGPL-3.0-or-later

package langtag

import (
	"regexp"

	"golang.org/x/text/language"
)

// Undetermined is BCP 47's tag for a language not known, where a tag must be written: an audio
// track's name in FFmpeg's stream map, an item's original text. The library itself stores such a
// language as empty.
const Undetermined = "und"

// BCP47 is raw as a BCP 47 tag: ISO 639-2's bibliographic and terminological codes ("ger",
// "deu") both become "de". Unknown or undetermined is empty.
func BCP47(raw string) string {
	parsed, err := language.Parse(raw)
	if err != nil || parsed == language.Und {
		return ""
	}
	return parsed.String()
}

// WellFormedPattern is the shape of a BCP 47 tag, as a regular expression: a two- or
// three-letter language, then subtags of one to eight letters or digits. One is BCP 47's
// shortest, a singleton such as the x in en-x-foo. It checks the shape only, not that the tag
// names a language, and admits only letters, digits and hyphens, so a tag that matches is safe
// in FFmpeg's stream map.
const WellFormedPattern = `^[A-Za-z]{2,3}(-[A-Za-z0-9]{1,8})*$`

// wellFormed is WellFormedPattern, compiled once.
var wellFormed = regexp.MustCompile(WellFormedPattern)

// IsWellFormed reports whether tag has the shape of a BCP 47 tag (WellFormedPattern).
func IsWellFormed(tag string) bool {
	return wellFormed.MatchString(tag)
}
