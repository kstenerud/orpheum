// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"strings"
	"unicode"
)

// saidValue is kept, a value the database keeps in snake case (a job's kind, an extra's type, a
// secret's name, a library's metadata source, an override's field), as the API says it: camelCase,
// as every key and value it sends is (the owner, 2026-11-09). The stored values, and the migrations
// that name them, stay as they are, so the API turns one into the other where such a value crosses
// it. A secret's name stays because its sealed value is bound to it.
func saidValue(kept string) string {
	var said strings.Builder
	upper := false
	for _, each := range kept {
		switch {
		case each == '_':
			upper = true
		case upper:
			said.WriteRune(unicode.ToUpper(each))
			upper = false
		default:
			said.WriteRune(each)
		}
	}
	return said.String()
}

// keptValue is said, a camelCase value from the API, as the database keeps it: saidValue's
// inverse.
func keptValue(said string) string {
	var kept strings.Builder
	for _, each := range said {
		if unicode.IsUpper(each) {
			kept.WriteRune('_')
			kept.WriteRune(unicode.ToLower(each))
			continue
		}
		kept.WriteRune(each)
	}
	return kept.String()
}

// saidKeys is counts keyed by values the database keeps (a job's kind), keyed as the API says them.
func saidKeys(kept map[string]int) map[string]int {
	said := make(map[string]int, len(kept))
	for key, count := range kept {
		said[saidValue(key)] = count
	}
	return said
}
