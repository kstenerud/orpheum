// SPDX-License-Identifier: GPL-3.0-or-later

package langtag

import "golang.org/x/text/language"

// BCP47 is raw as a BCP 47 tag: ISO 639-2's bibliographic and terminological codes ("ger",
// "deu") both become "de". Unknown or undetermined is empty.
func BCP47(raw string) string {
	parsed, err := language.Parse(raw)
	if err != nil || parsed == language.Und {
		return ""
	}
	return parsed.String()
}
