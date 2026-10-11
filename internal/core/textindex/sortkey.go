// SPDX-License-Identifier: AGPL-3.0-or-later

package textindex

import (
	"fmt"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// Sorter makes the CLDR sort keys a locale lists in (D47): byte-comparable, so SQLite orders
// them with its default BINARY collation and pages through an index. Numbers compare as numbers,
// so "Episode 2" comes before "Episode 10".
type Sorter struct {
	locale   string
	collator *collate.Collator
	buffer   collate.Buffer
}

// NewSorter returns the sorter for a BCP 47 locale.
func NewSorter(locale string) (*Sorter, error) {
	tag, err := language.Parse(locale)
	if err != nil {
		return nil, fmt.Errorf("sort locale %q: %w", locale, err)
	}
	return &Sorter{locale: tag.String(), collator: collate.New(tag, collate.Numeric, collate.IgnoreCase)}, nil
}

// Locale is the sorter's locale, as sort_key rows name it.
func (sorter *Sorter) Locale() string { return sorter.locale }

// Key is a name's sort key. It is a fresh slice the caller may keep.
func (sorter *Sorter) Key(name string) []byte {
	key := sorter.collator.KeyFromString(&sorter.buffer, name)
	sorter.buffer.Reset()
	return append([]byte(nil), key...)
}
