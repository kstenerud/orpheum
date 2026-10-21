// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// spellingVariantsAtMost is how many other spellings of a name a match searches for when the
// provider found no work of the name itself: a request each, made only for a name already failing.
const spellingVariantsAtMost = 3

// spellingVariants are other ways a name may be written that the provider's search, which matches
// words as they are spelled, would not find from it (the owner's staging, 2026-10-21, where TMDB has
// Edric Pellow's series as "Harbour Lights" and a search for "Harbor Lights" never finds it): the
// other side of the Atlantic's spelling, the name with its leading article or without one, and its
// numbers as Roman or Arabic numerals. Each is the whole name changed one way; none is the name.
func spellingVariants(name string) []string {
	var variants []string
	for _, variant := range []string{atlanticSpelling(name), otherArticle(name), otherNumerals(name)} {
		if variant != "" && variant != name {
			variants = append(variants, variant)
		}
	}
	return variants[:min(len(variants), spellingVariantsAtMost)]
}

// spellingPairs are words spelled one way in American English and another in British, in the
// forms a title's name most often holds them; the suffix rules below cover -ize, -ization and the
// like.
func spellingPairs() map[string]string {
	return map[string]string{
		"color": "colour", "honor": "honour", "humor": "humour", "favorite": "favourite",
		"neighbor": "neighbour", "harbor": "harbour", "armor": "armour", "savior": "saviour",
		"labor": "labour", "behavior": "behaviour", "rumor": "rumour", "flavor": "flavour",
		"valor": "valour", "splendor": "splendour", "glamor": "glamour", "parlor": "parlour",
		"theater": "theatre", "center": "centre", "meter": "metre", "fiber": "fibre", "specter": "spectre",
		"somber": "sombre", "saber": "sabre", "caliber": "calibre", "luster": "lustre",
		"gray": "grey", "defense": "defence", "offense": "offence", "traveler": "traveller",
		"catalog": "catalogue", "dialog": "dialogue", "mold": "mould", "plow": "plough",
		"pajamas": "pyjamas", "jewelry": "jewellery", "aluminum": "aluminium", "skeptic": "sceptic",
		"mustache": "moustache", "cozy": "cosy", "maneuver": "manoeuvre", "anesthesia": "anaesthesia",
		"encyclopedia": "encyclopaedia", "pediatric": "paediatric", "fetus": "foetus", "story": "storey",
	}
}

// atlanticSpelling is name with each word spelled the other side of the Atlantic's way, or "" when
// no word has another spelling.
func atlanticSpelling(name string) string {
	pairs := spellingPairs()
	other := make(map[string]string, 2*len(pairs))
	for american, british := range pairs {
		other[american], other[british] = british, american
	}
	isChanged := false
	changed := mapWords(name, func(word string) string {
		lower := strings.ToLower(word)
		replaced, found := other[lower]
		if !found {
			replaced = otherSuffix(lower)
		}
		if replaced == "" || replaced == lower {
			return word
		}
		isChanged = true
		return inCaseOf(word, replaced)
	})
	if !isChanged {
		return ""
	}
	return changed
}

// otherSuffix is a word with its -ize family ending spelled the other way, or "" for a word without
// one: "civilization" is "civilisation", and back, and an -ize word takes -ise. An -ise word
// is not taken back to -ize but in -isation, as "promise", "paradise" and "surprise" are -ise either
// side of the Atlantic; and a word must be long enough that its ending is a suffix, so "size" and
// "prize" keep theirs.
func otherSuffix(word string) string {
	const shortestWithSuffix = 7
	if len([]rune(word)) < shortestWithSuffix {
		return ""
	}
	for _, pair := range [][2]string{{"izations", "isations"}, {"ization", "isation"}} {
		if stem, found := strings.CutSuffix(word, pair[0]); found {
			return stem + pair[1]
		}
		if stem, found := strings.CutSuffix(word, pair[1]); found {
			return stem + pair[0]
		}
	}
	for _, pair := range [][2]string{
		{"izing", "ising"}, {"ized", "ised"}, {"izes", "ises"}, {"ize", "ise"}, {"yzed", "ysed"}, {"yze", "yse"},
	} {
		if stem, found := strings.CutSuffix(word, pair[0]); found {
			return stem + pair[1]
		}
	}
	return ""
}

// otherArticle is name without its leading article, or with "The" before it when it has none.
func otherArticle(name string) string {
	words := strings.Fields(name)
	if len(words) > 1 && isArticle(words[0]) {
		return strings.Join(words[1:], " ")
	}
	return "The " + name
}

// otherNumerals is name with its numbers from one to twenty as Roman numerals, or its Roman
// numerals as numbers, or "" when it has neither: "Rocky II" is "Rocky 2". A lone "I" is a word
// more often than a numeral, so it stays.
func otherNumerals(name string) string {
	romans := []string{
		"", "i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix", "x", "xi", "xii", "xiii",
		"xiv", "xv", "xvi", "xvii", "xviii", "xix", "xx",
	}
	isChanged := false
	changed := mapWords(name, func(word string) string {
		lower := strings.ToLower(word)
		for number := 2; number < len(romans); number++ {
			switch lower {
			case romans[number]:
				isChanged = true
				return strconv.Itoa(number)
			case strconv.Itoa(number):
				isChanged = true
				return strings.ToUpper(romans[number])
			}
		}
		return word
	})
	if !isChanged {
		return ""
	}
	return changed
}

// mapWords is name with each run of letters and digits replaced by change's word for it, the rest
// as it was.
func mapWords(name string, change func(word string) string) string {
	var builder strings.Builder
	var word []rune
	flush := func() {
		if len(word) > 0 {
			builder.WriteString(change(string(word)))
			word = word[:0]
		}
	}
	for _, character := range name {
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			word = append(word, character)
			continue
		}
		flush()
		builder.WriteRune(character)
	}
	flush()
	return builder.String()
}

// inCaseOf is replaced written in word's case: all capitals, a capital first, or as it is.
func inCaseOf(word, replaced string) string {
	switch {
	case word == strings.ToUpper(word):
		return strings.ToUpper(replaced)
	case unicode.IsUpper([]rune(word)[0]):
		runes := []rune(replaced)
		return string(unicode.ToUpper(runes[0])) + string(runes[1:])
	}
	return replaced
}

// searchWithSpellings searches the provider for query, and, when it finds no work of the query's
// name, for the name's other spellings in turn (spellingVariants), stopping at the first that finds
// a work of its own (the owner, 2026-10-21). It returns the query the candidates answer, the
// spelling that found one or query itself, and the candidates: that spelling's, then the rest of
// query's.
func searchWithSpellings(ctx context.Context, provider Provider, query Query) (Query, []Candidate, error) {
	candidates, err := provider.Search(ctx, query)
	if err != nil {
		return query, nil, fmt.Errorf("search %s for %q: %w", provider.Name(), query.Name, err)
	}
	if hasWorkNamed(query.Name, candidates) {
		return query, candidates, nil
	}
	for _, spelling := range spellingVariants(query.Name) {
		spelled := query
		spelled.Name = spelling
		found, err := provider.Search(ctx, spelled)
		if err != nil {
			return query, nil, fmt.Errorf("search %s for %q: %w", provider.Name(), spelling, err)
		}
		if hasWorkNamed(spelling, found) {
			return spelled, withCandidates(found, candidates), nil
		}
	}
	return query, candidates, nil
}

// hasWorkNamed is whether any candidate is named name, as the matcher compares names.
func hasWorkNamed(name string, candidates []Candidate) bool {
	want := workName(name)
	for _, candidate := range candidates {
		if workName(candidate.Name) == want || workName(candidate.OriginalName) == want {
			return true
		}
	}
	return false
}

// withCandidates is first, then each of more not already in it.
func withCandidates(first, more []Candidate) []Candidate {
	merged := slices.Clone(first)
	for _, candidate := range more {
		if !slices.ContainsFunc(merged, func(each Candidate) bool { return each.ID == candidate.ID }) {
			merged = append(merged, candidate)
		}
	}
	return merged
}
