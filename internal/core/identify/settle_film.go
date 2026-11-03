// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"cmp"
	"slices"
)

// How a film's candidates settle its match (D365, revising D301 §1 for films; a series keeps D301
// §1, an episode's length saying little of its series): with a year, by how each one's runtime fits
// the film's files, then its year, then its votes; with none, the well-known first (D412).

// A candidate's runtime fits a film's file when the two differ by at most the larger of these: 3
// minutes, or 3% of the runtime (D365 §1). Credits, another cut and PAL's speed-up move a film's
// length by about as much, so a closer bound would pass over the film it is, and a looser one could
// not tell staging's Crowd of 92 minutes, its file 92.1, from another of 88.
const (
	runtimeFitsWithinMs      = 3 * 60_000
	runtimeFitsWithinPercent = 3
)

// A film's file may be a longer cut of the film than the runtime the provider gives (D412 §4): up
// to this much longer, in percent of the runtime, or longerCutNamedPercent where its name says it
// is a longer cut ("Extended", "Director's Cut", "Special Edition", "Unrated"). Cameron's Wings is
// 137 minutes at TMDB and its Special Edition 154, 12% longer; the longest extended cuts of the
// LOTR films run about 31% longer, and Kingdom of Heaven's director's cut 35%.
const (
	longerCutPercent      = 30
	longerCutNamedPercent = 50
)

// fileLength is one of a film's files' length, known, and whether its name says it is a longer cut
// of the film (D412 §4).
type fileLength struct {
	ms          int64
	isLongerCut bool
}

// lengthsInMs are lengths' milliseconds, as the log says them.
func lengthsInMs(lengths []fileLength) []int64 {
	ms := make([]int64, 0, len(lengths))
	for _, length := range lengths {
		ms = append(ms, length.ms)
	}
	return ms
}

// runtimeFit is how a candidate's runtime fits a film's files, the likeliest first (D365 §1, D412 §4).
type runtimeFit int

const (
	fitsTheFile runtimeFit = iota
	// A file longer than the runtime by more than a fit, and by at most a longer cut's
	// share of it: a closer fit than a contradiction, but not as close as a fit (D412 §4).
	fitsALongerCut
	runtimeUnknown
	contradictsTheFile
)

// String is the fit as the log names it (D365 §5).
func (fit runtimeFit) String() string {
	switch fit {
	case fitsTheFile:
		return "fits"
	case fitsALongerCut:
		return "edition"
	case contradictsTheFile:
		return "contradicts"
	case runtimeUnknown:
	}
	return "unknown"
}

// fitOf is how a candidate's runtime fits the lengths of a film's files, each one known: it fits
// when it fits any of them, as a film's versions may be cuts of different lengths, and failing that
// fits a longer cut when any of them is one of it. A candidate whose runtime is unknown, or a film
// none of whose lengths is, neither fits nor contradicts.
func fitOf(candidate Candidate, lengths []fileLength) runtimeFit {
	if candidate.RuntimeMs <= 0 || len(lengths) == 0 {
		return runtimeUnknown
	}
	margin := max(runtimeFitsWithinMs, candidate.RuntimeMs*runtimeFitsWithinPercent/100)
	fit := contradictsTheFile
	for _, length := range lengths {
		difference := length.ms - candidate.RuntimeMs
		if difference <= margin && -difference <= margin {
			return fitsTheFile
		}
		longest := int64(longerCutPercent)
		if length.isLongerCut {
			longest = longerCutNamedPercent
		}
		if difference > 0 && difference <= candidate.RuntimeMs*longest/100 {
			fit = fitsALongerCut
		}
	}
	return fit
}

// settleFilm is settleMatchSaying of a film (D365). The candidates its name admits (filmsToRank)
// are ranked (rankFilms): one admitted is a clear winner, but for a film with no year, which is a
// guess; several are a guess, the first ranked taken, except several all a year off the file's
// none of which fits it, which stay a person's choice as D301 §1 has them. With none admitted, a
// work under another name may be guessed (settleByOtherNames).
func settleFilm(
	query Query, candidates []Candidate, isYearExpected bool, lengths []fileLength,
) (Candidate, settledBy, string) {
	found, rule := filmsToRank(query, candidates)
	if len(found) == 0 {
		return settleByOtherNames(query, candidates)
	}
	best := rankFilms(query, found, lengths)[0]
	if query.Year != 0 && best.Year != query.Year && rule == settledByNameAndYear {
		rule = settledByNameAndYearOff
	}
	switch {
	case len(found) == 1 && (query.Year != 0 || !isYearExpected):
		return best, clearly, rule
	case len(found) > 1 && query.Year != 0 && !isAnyOfYear(found, query.Year) &&
		fitOf(best, lengths) != fitsTheFile:
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

// rankFilms is found, the likeliest first. With a year in the query (D365 §1–§3, D412 §2): those
// whose runtime fits the film's files, then the rest, an unknown runtime as one the files
// contradict; within each, those of the named year before those a year off, then the most voted at
// the source. With no year (D412), the well-known first: the most voted, but only by a wide margin
// (voteTiers); then the fit, a close one over a longer cut's over the rest; then one whose original
// name is the query's over one named so only in translation; then the oldest, a candidate with no
// year last, and the most voted (D301 §1). The provider's order after.
func rankFilms(query Query, found []Candidate, lengths []fileLength) []Candidate {
	type ranking struct {
		candidate                Candidate
		fit, voteTier            int
		isNamedInTranslationOnly bool
	}
	tiers := voteTiers(found)
	ranked := make([]ranking, 0, len(found))
	for index, candidate := range found {
		ranked = append(ranked, ranking{
			candidate: candidate, fit: int(rankedFit(query, candidate, lengths)), voteTier: tiers[index],
			isNamedInTranslationOnly: isNamedInTranslationOnly(query, candidate),
		})
	}
	slices.SortStableFunc(ranked, func(one, other ranking) int {
		if query.Year != 0 {
			return cmp.Or(cmp.Compare(one.fit, other.fit),
				cmp.Compare(yearsOff(one.candidate.Year, query.Year), yearsOff(other.candidate.Year, query.Year)),
				cmp.Compare(other.candidate.Votes, one.candidate.Votes))
		}
		return cmp.Or(cmp.Compare(one.voteTier, other.voteTier), cmp.Compare(one.fit, other.fit),
			compareFalseFirst(one.isNamedInTranslationOnly, other.isNamedInTranslationOnly),
			compareOldest(one.candidate.Year, other.candidate.Year),
			cmp.Compare(other.candidate.Votes, one.candidate.Votes))
	})
	films := make([]Candidate, 0, len(ranked))
	for _, each := range ranked {
		films = append(films, each.candidate)
	}
	return films
}

// rankedFit is the class of fit a candidate is ranked in (D412 §2, §4): an unknown runtime ranks
// with those the file contradicts, so only a fit lifts a candidate; with no year a longer cut's fit
// lifts one too, but with a year only a fit does, as D365 §1 has it, the year the stronger cue.
func rankedFit(query Query, candidate Candidate, lengths []fileLength) runtimeFit {
	switch fit := fitOf(candidate, lengths); {
	case fit == runtimeUnknown, fit == fitsALongerCut && query.Year != 0:
		return contradictsTheFile
	default:
		return fit
	}
}

// Votes decide between a film's candidates with no year only by a wide margin (D412 §3): one with
// at least votesDecideBy times the other's. Below votesCountFrom a count is noise, counted as that
// many, so a film needs ten times that to decide over one nobody rated.
const (
	votesDecideBy  = 10
	votesCountFrom = 10
)

// voteTiers are found's tiers by votes, by index (D412 §3): the first tier those within a wide
// margin of the most voted, the next those within one of the most voted of the rest, and so on.
// A tier is each candidate's own, so ranking by it stays an order, as "decides by a margin" between
// pairs would not be: 100 is ten times 5, but neither is ten times 20.
func voteTiers(found []Candidate) []int {
	counted := func(votes int) int { return max(votes, votesCountFrom) }
	byVotes := make([]int, len(found))
	for index := range byVotes {
		byVotes[index] = index
	}
	slices.SortStableFunc(byVotes, func(one, other int) int {
		return cmp.Compare(found[other].Votes, found[one].Votes)
	})
	tiers := make([]int, len(found))
	tier, top := 0, 0
	for position, index := range byVotes {
		votes := counted(found[index].Votes)
		if position == 0 {
			top = votes
		} else if votes*votesDecideBy <= top {
			tier, top = tier+1, votes
		}
		tiers[index] = tier
	}
	return tiers
}

// isNamedInTranslationOnly is whether a candidate is named as the query only by its translated
// name, its original name another (D412 §1): Blind & Ugly, named in English, is Blind & Hässlich.
// A candidate whose original name is not known is not.
func isNamedInTranslationOnly(query Query, candidate Candidate) bool {
	return candidate.OriginalName != "" && workName(candidate.OriginalName) != workName(query.Name)
}

// compareFalseFirst orders false before true.
func compareFalseFirst(one, other bool) int {
	switch {
	case one == other:
		return 0
	case one:
		return 1
	}
	return -1
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
