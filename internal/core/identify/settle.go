// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"strings"

	"github.com/kstenerud/orpheum/internal/core/textindex"
)

// How a search's candidates settle a match (D172, D301, D307): the matcher's tiers, its guesses,
// and the rule that settled each, as a rescan's log names it (D357 §6).

// settledBy is how a match was settled: not at all, by one clear winner, or guessed among several
// whose names match (D301), which is marked as a guess until a person confirms or corrects it.
type settledBy int

const (
	unsettled settledBy = iota
	clearly
	byGuess
)

// settleMatch is the candidate a query's search settles on, and how (D172, D301). A series' year is
// checked where its folder or tvshow.nfo gives one (D209).
//
// It looks in tiers, the next only when the one before found none: the name and the year; the name
// and a year one off, as a film's year at TMDB is often its first showing's, the year before its
// release (a film's 1997 is TMDB's 1998); and a candidate whose whole name begins the
// query's, word by word, in the year, as a release name carries a subtitle the provider's does not
// ("Show 2 The Return" is TMDB's "Show 2", 1981). One candidate in a tier is a clear
// winner, but for a film with no year, which is a guess. Several are settled by a guess (the
// owner's NAS, 2026-10-15, where 85 titles waited for a choice, many plain to them): with no year
// the oldest, with one the most voted (the better-known of two same-named films from 1992), and TMDB's order
// after. With no candidate named as the query, a work released under another name is guessed
// (underAnotherName, D307 §1).
//
// A film's candidates are settled otherwise (settleFilm, D365): by how their runtimes fit its files'
// lengths, which a series' search passes none of.
func settleMatch(
	query Query, candidates []Candidate, isYearExpected bool, lengths []fileLength,
) (Candidate, settledBy) {
	chosen, how, _ := settleMatchSaying(query, candidates, isYearExpected, lengths)
	return chosen, how
}

// The rules that settle a match, as a rescan's log names them (D357 §6): settleMatch's tiers, in
// order, then the near name and the other name it guesses by, and an id the files give.
const (
	settledByIDInHand         = "a provider id its files give"
	settledByNameAndYear      = "the name and the year"
	settledByNameAndYearOff   = "the name and a year one off"
	settledByNameBeginning    = "a name beginning the file's, in its year"
	settledByNearlyTheName    = "nearly the name"
	settledByUnderAnotherName = "a work under another name"
	settledByNothing          = "nothing: several, or none, to choose from"
)

// tierRules are what each of matchTiers' tiers settles by, in its order.
func tierRules() []string {
	return []string{settledByNameAndYear, settledByNameAndYearOff, settledByNameBeginning}
}

// settleMatchSaying is settleMatch, and which of its rules settled the match.
func settleMatchSaying(
	query Query, candidates []Candidate, isYearExpected bool, lengths []fileLength,
) (Candidate, settledBy, string) {
	if query.Kind == KindMovie {
		return settleFilm(query, candidates, isYearExpected, lengths)
	}
	for index, tier := range matchTiers(query) {
		var found []Candidate
		for _, candidate := range candidates {
			if tier(candidate) {
				found = append(found, candidate)
			}
		}
		found = namedExactly(query, found)
		switch {
		case len(found) == 1 && (query.Year != 0 || !isYearExpected):
			return found[0], clearly, tierRules()[index]
		case len(found) == 1 || len(found) > 1 && index != yearOffTier:
			return likeliest(found, query.Year == 0), byGuess, tierRules()[index]
		case len(found) > 1:
			// Several a year off the file's are none of its year: a person's choice (D301 §1).
			return Candidate{}, unsettled, settledByNothing
		}
	}
	return settleByOtherNames(query, candidates)
}

// settleByOtherNames is the guess among candidates none of which is named as the query is: one
// nearly named so, or a work under another name (D307 §1).
func settleByOtherNames(query Query, candidates []Candidate) (Candidate, settledBy, string) {
	if near, found := nearlyNamed(query, candidates); found {
		return near, byGuess, settledByNearlyTheName
	}
	if other, found := underAnotherName(query, candidates); found {
		return other, byGuess, settledByUnderAnotherName
	}
	return Candidate{}, unsettled, settledByNothing
}

// namedExactly is those of found named as the query is, its leading article and all, when any is and
// others are named so only once an article is set aside: a file's "Crowd" is TMDB's
// "Crowd", not "The Crowd" of the same year (the owner's staging, 2026-10-27). With none
// so named, or all, it is found as it is.
func namedExactly(query Query, found []Candidate) []Candidate {
	if len(found) < 2 {
		return found
	}
	want := textindex.SameWork(strings.Join(wordsOf(query.Name), " "))
	var exact []Candidate
	for _, candidate := range found {
		for _, name := range []string{candidate.Name, candidate.OriginalName} {
			if name != "" && textindex.SameWork(strings.Join(wordsOf(name), " ")) == want {
				exact = append(exact, candidate)
				break
			}
		}
	}
	if len(exact) == 0 {
		return found
	}
	return exact
}

// searchFoundTheWorkAtMost is how many candidates a search whose names all differ from the query's
// may find for it to have found the work under another name, rather than works with the words in
// theirs (D307 §1).
const searchFoundTheWorkAtMost = 2

// underAnotherName is the guess among candidates none of whose names is the query's, a work released
// under another name in another country (D307 §1: a series released under a different regional
// name is TMDB's under its original one): with a year, the one candidate of that year; with none,
// the oldest of the one or two the search found.
func underAnotherName(query Query, candidates []Candidate) (Candidate, bool) {
	if query.Year != 0 {
		var ofTheYear []Candidate
		for _, candidate := range candidates {
			if candidate.Year == query.Year {
				ofTheYear = append(ofTheYear, candidate)
			}
		}
		if len(ofTheYear) == 1 {
			return ofTheYear[0], true
		}
		return Candidate{}, false
	}
	if len(candidates) == 0 || len(candidates) > searchFoundTheWorkAtMost {
		return Candidate{}, false
	}
	return likeliest(candidates, true), true
}

// yearOffTier is matchTiers' tier of a year one off, whose several are never guessed between.
const yearOffTier = 1

// likeliest is the guess among candidates whose names all match (D301): the oldest when the name
// gave no year, then the most voted, then the first in the provider's order. A candidate with no
// year is never the oldest.
func likeliest(found []Candidate, isOldestFirst bool) Candidate {
	best := found[0]
	for _, candidate := range found[1:] {
		if isOldestFirst && candidate.Year != best.Year {
			if candidate.Year != 0 && (best.Year == 0 || candidate.Year < best.Year) {
				best = candidate
			}
			continue
		}
		if candidate.Votes > best.Votes {
			best = candidate
		}
	}
	return best
}

// matchTiers are settleMatch's tiers, in order: the name and the year; the name and a year one
// off; the name beginning the query's, word by word, in its year.
func matchTiers(query Query) []func(Candidate) bool {
	want := workName(query.Name)
	named := func(candidate Candidate) bool {
		return workName(candidate.Name) == want || workName(candidate.OriginalName) == want
	}
	withinYears := func(candidate Candidate, years int) bool {
		return query.Year == 0 || (candidate.Year >= query.Year-years && candidate.Year <= query.Year+years)
	}
	return []func(Candidate) bool{
		func(candidate Candidate) bool { return named(candidate) && withinYears(candidate, 0) },
		func(candidate Candidate) bool {
			return named(candidate) && query.Year != 0 && withinYears(candidate, 1)
		},
		func(candidate Candidate) bool {
			return query.Year != 0 && candidate.Year == query.Year &&
				(beginsByWords(query.Name, candidate.Name) || beginsByWords(query.Name, candidate.OriginalName))
		},
	}
}
