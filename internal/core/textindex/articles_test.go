// SPDX-License-Identifier: AGPL-3.0-or-later

package textindex

import (
	"bytes"
	"slices"
	"testing"
)

// D402: a name sorts without its leading article, in the language of the sort locale; a name that
// is only an article, or begins with an abbreviation, stays whole.
func TestSorter_FilesANameUnderItsWordsNotItsLeadingArticle(t *testing.T) {
	t.Parallel()
	ordered := func(locale string, names ...string) []string {
		t.Helper()
		sorter, err := NewSorter(locale)
		if err != nil {
			t.Fatal(err)
		}
		sorted := slices.Clone(names)
		slices.SortFunc(sorted, func(a, b string) int { return bytes.Compare(sorter.Key(a), sorter.Key(b)) })
		return sorted
	}
	if got := ordered("en", "Destiny", "The Gold Rush", "A Woman of Paris", "Metropolis", "An Elephant", "The"); !slices.Equal(got,
		[]string{"A Woman of Paris", "An Elephant", "Metropolis", "The Gold Rush", "The", "Destiny"}) {
		t.Errorf("en: %v; want The Gold Rush among M, A Woman of Paris among B, and The alone under T", got)
	}
	if got := ordered("de", "Destiny", "Der letzte Mann", "Ulme", "Der Golem"); !slices.Equal(got,
		[]string{"Der Golem", "Ulme", "Der letzte Mann", "Destiny"}) {
		t.Errorf("de: %v; want Der letzte Mann under U, after Ulme, and Der Golem under B", got)
	}
	if got := ordered("fr", "Zazie", "L'Arkon", "Der Golem", "Bof"); !slices.Equal(got,
		[]string{"L'Arkon", "Bof", "Der Golem", "Zazie"}) {
		t.Errorf("fr: %v; want L'Arkon under A, Der Golem under M", got)
	}
	if got := ordered("es", "Destiny", "El Laberinto", "Los Otros"); !slices.Equal(got,
		[]string{"El Laberinto", "Los Otros", "Destiny"}) {
		t.Errorf("es: %v", got)
	}
	// English articles are no articles in German, and an initialism is no article.
	if got := ordered("de", "The Gold Rush", "Metropolis"); !slices.Equal(got, []string{"Metropolis", "The Gold Rush"}) {
		t.Errorf("de: %v; The is no German article", got)
	}
	if got := ordered("en", "A. I.", "Metropolis", "I Robot"); !slices.Equal(got, []string{"A. I.", "Metropolis", "I Robot"}) {
		t.Errorf("en: %v; A. I. keeps its A", got)
	}
}

func TestIsArticle_ReadsTheLanguagesList(t *testing.T) {
	t.Parallel()
	if !IsArticle("en", "The") || !IsArticle("de", "Die") || IsArticle("de", "the") || IsArticle("xx", "the") {
		t.Error("IsArticle did not follow each language's own list")
	}
}
