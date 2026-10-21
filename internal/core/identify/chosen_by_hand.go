// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// A person's own ways to a film's or series' match when the search found none of it (the owner's
// staging, 2026-10-21, where TMDB has "Harbour Lights" and the folder said "Harbor Lights"): searching
// for another name, and pasting the work's address or id at TMDB or IMDb.

// ErrBadLink is a pasted link or id that names no work of the item's kind at a provider this
// server reads.
var ErrBadLink = errors.New("that is no TMDB or IMDb address or id of a work of this kind")

// SearchAs searches the item's provider for a work named name, of year when it is not zero, as a
// person typed them, with the name's other spellings (spellingVariants), and keeps what it found as
// the item's candidates, best first, for the person to choose from.
func (chooser *Chooser) SearchAs(ctx context.Context, itemID int64, name string, year int) ([]Candidate, string,
	error,
) {
	item, err := chooser.readTarget(ctx, itemID)
	if err != nil {
		return nil, "", err
	}
	provider, known := chooser.matcher.providers[providerOf(item.metadataSource())]
	if !known {
		return []Candidate{}, "", nil // an NFO library's item has no provider to ask
	}
	query := item.searchQuery()
	query.Name, query.Year = strings.TrimSpace(name), year
	_, candidates, err := searchWithSpellings(ctx, provider, query)
	if err != nil {
		return nil, "", err
	}
	if err := chooser.matcher.keepCandidates(ctx, provider, itemID, candidates); err != nil {
		return nil, "", err
	}
	return candidates, provider.Name(), nil
}

// ChooseByLink matches the item to the work a pasted link or id names, as Choose: a TMDB address
// ("https://www.themoviedb.org/tv/4808-civilisation") or id ("9285"), or an IMDb address or id
// ("tt0456789"), which the provider finds its own work for. A TMDB address of the other kind, a film's
// for a series, is refused.
func (chooser *Chooser) ChooseByLink(ctx context.Context, itemID int64, link string) error {
	item, err := chooser.readTarget(ctx, itemID)
	if err != nil {
		return err
	}
	provider, known := chooser.matcher.providers[providerOf(item.metadataSource())]
	if !known {
		return fmt.Errorf("%q: %w", item.metadataSource(), ErrUnknownProvider)
	}
	kind := item.searchQuery().Kind
	source, id, found := workOfLink(link, kind)
	if !found {
		return fmt.Errorf("%q: %w", link, ErrBadLink)
	}
	if source != provider.Name() {
		id, err = provider.Find(ctx, kind, source, id)
		if errors.Is(err, ErrNoMatch) {
			return fmt.Errorf("%s %s: %w", source, id, ErrNoMatch)
		}
		if err != nil {
			return fmt.Errorf("find %s %s at %s: %w", source, id, provider.Name(), err)
		}
	}
	return chooser.Choose(ctx, itemID, provider.Name(), id)
}

// The forms a pasted link or id takes, ASCII digits spelled [0-9] as the filename expressions do
// (D49).
//
//nolint:gocritic // regexpSimplify's \d is the class D49 bans; [0-9] says the ASCII is meant
var (
	tmdbAddress = regexp.MustCompile(`themoviedb\.org/(movie|tv)/([0-9]+)`)
	imdbID      = regexp.MustCompile(`tt[0-9]{5,10}`)
	tmdbID      = regexp.MustCompile(`^[0-9]+$`)
)

// workOfLink is the provider and id a pasted link or id names for a work of kind: TMDB's address or
// its bare number, or IMDb's tt id anywhere in what was pasted. A TMDB address of the other kind
// names no work of this one.
func workOfLink(link, kind string) (source, id string, found bool) {
	link = strings.TrimSpace(link)
	if match := tmdbAddress.FindStringSubmatch(link); match != nil {
		if (match[1] == "tv") != (kind == KindSeries) {
			return "", "", false
		}
		return sourceTMDB, match[2], true
	}
	if match := imdbID.FindString(link); match != "" {
		return sourceIMDb, match, true
	}
	if tmdbID.MatchString(link) {
		return sourceTMDB, link, true
	}
	return "", "", false
}
