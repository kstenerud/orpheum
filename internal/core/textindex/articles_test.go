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
	got := ordered("en", "Wings", "The Gold Rush", "A Fool There Was", "Metropolis", "An Elephant", "The")
	if !slices.Equal(got,
		[]string{"An Elephant", "A Fool There Was", "The Gold Rush", "Metropolis", "The", "Wings"}) {
		t.Errorf("en: %v; want The Gold Rush among G, A Fool There Was among F, and The alone under T", got)
	}
	if got := ordered("de", "Variety", "Der letzte Mann", "Lantern", "Der Golem"); !slices.Equal(got,
		[]string{"Der Golem", "Lantern", "Der letzte Mann", "Variety"}) {
		t.Errorf("de: %v; want Der letzte Mann under L, after Lantern, and Der Golem under G", got)
	}
	if got := ordered("fr", "Zelkova Lane", "L'Arkon", "Les Phares du Nord", "Bof"); !slices.Equal(got,
		[]string{"L'Arkon", "Bof", "Les Phares du Nord", "Zelkova Lane"}) {
		t.Errorf("fr: %v; want L'Arkon under A, Les Phares du Nord under P", got)
	}
	if got := ordered("es", "Variety", "El Mirage", "Los Ulme"); !slices.Equal(got,
		[]string{"El Mirage", "Los Ulme", "Variety"}) {
		t.Errorf("es: %v", got)
	}
	// English articles are no articles in German, and an initialism is no article.
	if got := ordered("de", "The Gold Rush", "Metropolis"); !slices.Equal(got,
		[]string{"Metropolis", "The Gold Rush"}) {
		t.Errorf("de: %v; The is no German article", got)
	}
	if got := ordered("en", "A. B.", "Greed", "I Lockfast"); !slices.Equal(got,
		[]string{"A. B.", "Greed", "I Lockfast"}) {
		t.Errorf("en: %v; A. B. keeps its A", got)
	}
}

func TestIsArticle_ReadsTheLanguagesList(t *testing.T) {
	t.Parallel()
	if !IsArticle("en", "The") || !IsArticle("de", "Die") || IsArticle("de", "the") || IsArticle("xx", "the") {
		t.Error("IsArticle did not follow each language's own list")
	}
}
