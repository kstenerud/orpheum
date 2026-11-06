// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// A film's candidates' runtimes, read to fit them to its files (D365 §4), and the log of how they
// settled it (§5).

// filmFit is what a film's candidates are fitted against, and what reading their runtimes fetched.
type filmFit struct {
	// lengths are the lengths of the film's live files that are known, 0 before a file is probed
	// being none.
	lengths []fileLength
	// fetched are the details fetched for a runtime, by the candidate's id: the one taken is stored
	// from them rather than fetched again.
	fetched map[string][]byte
}

// fileLengths are the known lengths of a title's live files, each saying whether its name says it
// is a longer cut (D412 §4).
func (matcher *Matcher) fileLengths(ctx context.Context, titleID int64) ([]fileLength, error) {
	files, err := matcher.database.Read().ListLiveFilesOfWork(ctx, titleID)
	if err != nil {
		return nil, fmt.Errorf("list title %d's files: %w", titleID, err)
	}
	var lengths []fileLength
	for _, file := range files {
		if file.DurationMs > 0 {
			lengths = append(lengths, fileLength{ms: file.DurationMs, isLongerCut: namesALongerCut(file.Path)})
		}
	}
	return lengths, nil
}

// fitFilm reads the runtimes of a film's candidates its name admits (filmsToRank) from the
// provider's details of each, where there are several to rank and one of the film's files has a
// known length to fit them to; otherwise none could change the ranking. A runtime kept with an
// earlier search's candidate of the item is read from cache.db, and fetched again only when the
// candidate is (D365 §4). It returns the candidates with their runtimes, and what it fitted them
// against.
func (matcher *Matcher) fitFilm(
	ctx context.Context, provider Provider, titleID int64, query Query, candidates []Candidate,
) ([]Candidate, filmFit, error) {
	if query.Kind != KindMovie {
		return candidates, filmFit{}, nil
	}
	lengths, err := matcher.fileLengths(ctx, titleID)
	if err != nil {
		return nil, filmFit{}, err
	}
	fit := filmFit{lengths: lengths, fetched: map[string][]byte{}}
	admitted, _ := filmsToRank(query, candidates)
	if len(admitted) < 2 || len(lengths) == 0 {
		return candidates, fit, nil
	}
	runtimes, err := matcher.keptRuntimes(ctx, provider, titleID)
	if err != nil {
		return nil, filmFit{}, err
	}
	for _, candidate := range admitted {
		if _, isKept := runtimes[candidate.ID]; isKept {
			continue
		}
		runtime, body, fetchErr := fetchRuntime(ctx, provider, candidate.ID)
		if fetchErr != nil {
			return nil, filmFit{}, fetchErr
		}
		if body != nil {
			runtimes[candidate.ID], fit.fetched[candidate.ID] = runtime, body
		}
	}
	read := slices.Clone(candidates)
	for index := range read {
		if runtime, found := runtimes[read[index].ID]; found {
			read[index].RuntimeMs = runtime
		}
	}
	return read, fit, nil
}

// keptRuntimes are the runtimes kept with an item's candidates at provider, by the candidate's id:
// those known.
func (matcher *Matcher) keptRuntimes(ctx context.Context, provider Provider, itemID int64) (map[string]int64, error) {
	kept, keptBy, err := matcher.keptCandidates(ctx, itemID)
	if err != nil {
		return nil, err
	}
	runtimes := map[string]int64{}
	for _, candidate := range kept {
		if keptBy == provider.Name() && candidate.RuntimeMs > 0 {
			runtimes[candidate.ID] = candidate.RuntimeMs
		}
	}
	return runtimes, nil
}

// fetchRuntime is a film's runtime, from the provider's details of it, asked as any other fetch
// is, through the provider's own limits; and the details. A film the provider no longer has is one
// of unknown runtime, with no details.
func fetchRuntime(ctx context.Context, provider Provider, id string) (int64, []byte, error) {
	body, err := provider.Fetch(ctx, KindMovie, id)
	if errors.Is(err, ErrNoMatch) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, fmt.Errorf("fetch %s %s for its runtime: %w", provider.Name(), id, err)
	}
	metadata, err := provider.Parse(KindMovie, body)
	if err != nil {
		return 0, nil, fmt.Errorf("read %s %s: %w", provider.Name(), id, err)
	}
	return metadata.RuntimeMs, body, nil
}

// logFilmSettled says why a film's search settled as it did, where its name admitted several
// candidates (D365 §5): each one's year, runtime, votes and how its runtime fits the film's files,
// the likeliest first, then the one taken, if any.
func (matcher *Matcher) logFilmSettled(
	ctx context.Context, titleID int64, query Query, candidates []Candidate, fit filmFit, settled settlement,
) {
	admitted, _ := filmsToRank(query, candidates)
	if query.Kind != KindMovie || len(admitted) < 2 {
		return
	}
	for _, candidate := range rankFilms(query, admitted, fit.lengths) {
		matcher.logger.InfoContext(ctx, "a film's candidate",
			slogx.EventKey, "filmCandidate",
			"itemId", titleID, "candidate", candidate.ID,
			"year", candidate.Year, "runtimeMs", candidate.RuntimeMs, "votes", candidate.Votes,
			"fit", fitOf(candidate, fit.lengths).String())
	}
	matcher.logger.InfoContext(ctx, "a film's candidates settled",
		slogx.EventKey, "filmCandidatesSettled",
		"itemId", titleID, "year", query.Year,
		"fileLengthsMs", lengthsInMs(fit.lengths), "taken", settled.chosen.ID, "isGuess", settled.how == byGuess)
}
