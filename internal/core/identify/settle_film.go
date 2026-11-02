// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"cmp"
	"slices"
)

// How a film's candidates settle its match (D365, revising D301 §1 for films; a series keeps D301
// §1, an episode's length saying little of its series): by how each one's runtime fits the film's
// files, then its year, then its votes.

// A candidate's runtime fits a film's file when the two differ by at most the larger of these: 3
// minutes, or 3% of the runtime (D365 §1). Credits, another cut and PAL's speed-up move a film's
// length by about as much, so a closer bound would pass over the film it is, and a looser one could
// not tell staging's Crowd of 92 minutes, its file 92.1, from another of 88.
const (
	runtimeFitsWithinMs      = 3 * 60_000
	runtimeFitsWithinPercent = 3
)

// runtimeFit is how a candidate's runtime fits a film's files, the likeliest first (D365 §1).
type runtimeFit int

const (
	fitsTheFile runtimeFit = iota
	runtimeUnknown
	contradictsTheFile
)

// String is the fit as the log names it (D365 §5).
func (fit runtimeFit) String() string {
	switch fit {
	case fitsTheFile:
		return "fits"
	case contradictsTheFile:
		return "contradicts"
	case runtimeUnknown:
	}
	return "unknown"
}

// fitOf is how a candidate's runtime fits the lengths of a film's files, lengthsMs, each one known:
// it fits when it fits any of them, as a film's versions may be cuts of different lengths. A
// candidate whose runtime is unknown, or a film none of whose lengths is, neither fits nor
// contradicts.
func fitOf(candidate Candidate, lengthsMs []int64) runtimeFit {
	if candidate.RuntimeMs <= 0 || len(lengthsMs) == 0 {
		return runtimeUnknown
	}
	margin := max(runtimeFitsWithinMs, candidate.RuntimeMs*runtimeFitsWithinPercent/100)
	for _, length := range lengthsMs {
		if difference := length - candidate.RuntimeMs; difference <= margin && -difference <= margin {
			return fitsTheFile
		}
	}
	return contradictsTheFile
}

// settleFilm is settleMatchSaying of a film (D365). The candidates its name admits (filmsToRank)
// are ranked (rankFilms): one admitted is a clear winner, but for a film with no year, which is a
// guess; several are a guess, the first ranked taken, except several all a year off the file's
// none of which fits it, which stay a person's choice as D301 §1 has them. With none admitted, a
// work under another name may be guessed (settleByOtherNames).
func settleFilm(
	query Query, candidates []Candidate, isYearExpected bool, lengthsMs []int64,
) (Candidate, settledBy, string) {
	found, rule := filmsToRank(query, candidates)
	if len(found) == 0 {
		return settleByOtherNames(query, candidates)
	}
	best := rankFilms(query, found, lengthsMs)[0]
	if query.Year != 0 && best.Year != query.Year && rule == settledByNameAndYear {
		rule = settledByNameAndYearOff
	}
	switch {
	case len(found) == 1 && (query.Year != 0 || !isYearExpected):
		return best, clearly, rule
	case len(found) > 1 && query.Year != 0 && !isAnyOfYear(found, query.Year) &&
		fitOf(best, lengthsMs) != fitsTheFile:
		// Several a year off the file's, none fitting it, are none of its year: a person's choice.
		return Candidate{}, unsettled, settledByNothing
	}
	return best, byGuess, rule
}

// filmsToRank are the candidates a film's name admits, and the rule that admits them (D365 §2):
// those named as the file is, with a year in its name or NFO those of that year and of a year
// either side, a premiere and a release, or a release in two countries, falling in different years;
// with no year, all of them. With none so named, those whose name begins the file's, word by word,
// in its year (matchTiers). Where some are named as the file is and others so only with a leading
// article set aside, the first are taken, among those of one year (namedExactly), as the year is
// the stronger cue (the owner, 2026-10-27).
func filmsToRank(query Query, candidates []Candidate) ([]Candidate, string) {
	tiers := matchTiers(query)
	named := func(candidate Candidate) bool {
		return tiers[0](candidate) || tiers[yearOffTier](candidate)
	}
	if found := namedExactlyEachYear(query, slices.Collect(filtered(candidates, named))); len(found) > 0 {
		return found, settledByNameAndYear
	}
	beginning := tiers[len(tiers)-1]
	return namedExactly(query, slices.Collect(filtered(candidates, beginning))), settledByNameBeginning
}

// filtered yields those of candidates that keep says to.
func filtered(candidates []Candidate, keep func(Candidate) bool) func(yield func(Candidate) bool) {
	return func(yield func(Candidate) bool) {
		for _, candidate := range candidates {
			if keep(candidate) && !yield(candidate) {
				return
			}
		}
	}
}

// namedExactlyEachYear is namedExactly among the candidates of each year, with a year in the query,
// so a candidate named exactly in a year off the file's sets none of its year aside; with no year,
// namedExactly of them all.
func namedExactlyEachYear(query Query, found []Candidate) []Candidate {
	if query.Year == 0 {
		return namedExactly(query, found)
	}
	var kept []Candidate
	for _, candidate := range found {
		ofItsYear := slices.Collect(filtered(found, func(other Candidate) bool { return other.Year == candidate.Year }))
		if slices.ContainsFunc(namedExactly(query, ofItsYear), func(exact Candidate) bool {
			return exact.ID == candidate.ID
		}) {
			kept = append(kept, candidate)
		}
	}
	return kept
}

// isAnyOfYear is whether any of found is of year.
func isAnyOfYear(found []Candidate, year int) bool {
	return slices.ContainsFunc(found, func(candidate Candidate) bool { return candidate.Year == year })
}

// rankFilms is found, the likeliest first (D365 §1–§3): those whose runtime fits the film's files,
// then those whose runtime is unknown, then those it contradicts, except with no year (rankedFit);
// within each, with a year in the query, those of that year before those a year off, then the most
// voted at the source; with no year, the oldest, a candidate with no year last, then the most voted
// (D301 §1); and the provider's order after.
func rankFilms(query Query, found []Candidate, lengthsMs []int64) []Candidate {
	ranked := slices.Clone(found)
	slices.SortStableFunc(ranked, func(one, other Candidate) int {
		if order := cmp.Compare(rankedFit(query, one, lengthsMs), rankedFit(query, other, lengthsMs)); order != 0 {
			return order
		}
		if query.Year != 0 {
			return cmp.Or(cmp.Compare(yearsOff(one.Year, query.Year), yearsOff(other.Year, query.Year)),
				cmp.Compare(other.Votes, one.Votes))
		}
		return cmp.Or(compareOldest(one.Year, other.Year), cmp.Compare(other.Votes, one.Votes))
	})
	return ranked
}

// rankedFit is the class of fit a candidate is ranked in: its fit (fitOf), but with no year in the
// query an unknown runtime ranks with those the file contradicts, so only a fit lifts a candidate
// over the oldest (the owner, 2026-11-02). With no year the name admits every work of it across the
// decades, and the ones the source knows no runtime of are its sparsest entries: the owner's
// Wings.mp4, a longer cut of Cameron's 137-minute Wings, went to a 2014 Wings of no runtime and
// one vote, as a cut contradicted the film it is. With a year, which admits three years' works, an
// unknown runtime still ranks between (D365 §1).
func rankedFit(query Query, candidate Candidate, lengthsMs []int64) runtimeFit {
	fit := fitOf(candidate, lengthsMs)
	if query.Year == 0 && fit == runtimeUnknown {
		return contradictsTheFile
	}
	return fit
}

// yearsOff is how many years year is off want.
func yearsOff(year, want int) int {
	if year > want {
		return year - want
	}
	return want - year
}

// compareOldest orders two years the older first, an unknown year, 0, last.
func compareOldest(one, other int) int {
	switch {
	case one == other:
		return 0
	case one == 0:
		return 1
	case other == 0:
		return -1
	}
	return cmp.Compare(one, other)
}
