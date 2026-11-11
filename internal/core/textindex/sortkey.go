// SPDX-License-Identifier: AGPL-3.0-or-later

package textindex

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// Sorter makes the CLDR sort keys a locale lists in (D47): byte-comparable, so SQLite orders
// them with its default BINARY collation and pages through an index. Numbers compare as numbers,
// so "Episode 2" comes before "Episode 10".
type Sorter struct {
	locale   string
	language string // the locale's language, which picks the articles a name sorts without (D402)
	collator *collate.Collator
	buffer   collate.Buffer
}

// NewSorter returns the sorter for a BCP 47 locale.
func NewSorter(locale string) (*Sorter, error) {
	tag, err := language.Parse(locale)
	if err != nil {
		return nil, fmt.Errorf("sort locale %q: %w", locale, err)
	}
	base, _ := tag.Base()
	collator := collate.New(tag, collate.Numeric, collate.IgnoreCase)
	return &Sorter{locale: tag.String(), language: base.String(), collator: collator}, nil
}

// Locale is the sorter's locale, as sort_key rows name it.
func (sorter *Sorter) Locale() string { return sorter.locale }

// Key is a name's sort key: the key of the name without its leading article in the sorter's
// language, so "The Gold Rush" sorts among the G (D402, refining D47). The name shown is unchanged. It
// is a fresh slice the caller may keep.
func (sorter *Sorter) Key(name string) []byte {
	return sorter.KeyExact(sorter.withoutArticle(name))
}

// KeyExact is the sort key of a text as written, its leading article kept: a file name's, which
// sorts as it is spelled (D349 section 4), and a sort title, which a person chose (D402).
// It is a fresh slice the caller may keep.
func (sorter *Sorter) KeyExact(text string) []byte {
	key := sorter.collator.KeyFromString(&sorter.buffer, text)
	sorter.buffer.Reset()
	return append([]byte(nil), key...)
}

// articles are the words a name may begin with and sort without, by language (D402): the
// definite and indefinite articles. One that ends in an apostrophe is elided onto the word that
// follows it ("L'Arkon"). The matcher's "The Show" is "Show" reads the same list (IsArticle).
func articlesOf(lang string) []string {
	switch lang {
	case "en":
		return []string{"the", "a", "an"}
	case "de":
		return []string{"der", "die", "das", "ein", "eine"}
	case "fr":
		return []string{"le", "la", "les", "l'", "un", "une"}
	case "es":
		return []string{"el", "la", "los", "las", "un", "una"}
	}
	return nil
}

// IsArticle is whether a word is an article in a language, as the identity fold reads it (D232):
// "The" and "the" are. An elided article ("l'") is no word of its own.
func IsArticle(lang, word string) bool {
	return slices.Contains(articlesOf(lang), SameWork(word))
}

// withoutArticle is a name less its leading article, when something follows it: "The" alone stays
// whole, and so does a name in a language with no list (D402).
func (sorter *Sorter) withoutArticle(name string) string {
	trimmed := strings.TrimLeftFunc(name, unicode.IsSpace)
	for _, article := range articlesOf(sorter.language) {
		if !strings.HasSuffix(article, "'") {
			continue
		}
		for _, apostrophe := range []string{"'", "’"} {
			prefix := strings.TrimSuffix(article, "'") + apostrophe
			if len(trimmed) > len(prefix) && strings.EqualFold(trimmed[:len(prefix)], prefix) {
				return strings.TrimLeftFunc(trimmed[len(prefix):], unicode.IsSpace)
			}
		}
	}
	first, rest, hasRest := strings.Cut(trimmed, " ")
	rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
	if hasRest && rest != "" && isWord(first) && IsArticle(sorter.language, first) {
		return rest
	}
	return name
}

// isWord is whether text is letters alone: "A." in "A. B." is no article.
func isWord(text string) bool {
	return strings.IndexFunc(text, func(character rune) bool { return !unicode.IsLetter(character) }) < 0
}
