// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/platform/langtag"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// heldCredit is one credit of an item as the database holds it, with its person.
type heldCredit struct {
	role       string
	ordinal    int64
	character  string
	tmdbID     string
	name       string
	pictureRef string
}

// heldGenre is one genre of an item as the database holds it, by its folded name.
type heldGenre struct {
	nameKey string
}

// loadPeople reads the credits and genres of the library's live items, in two statements.
func (snap *snapshot) loadPeople(ctx context.Context, queries *sqlcgen.Queries, libraryID int64) error {
	credits, err := queries.ListLibraryCredits(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its credits: %w", err)
	}
	for _, credit := range credits {
		snap.credits[credit.ItemID] = append(snap.credits[credit.ItemID], heldCredit{
			role: credit.Role, ordinal: credit.Ordinal, character: ptr.ValueOrZero(credit.Character),
			tmdbID: ptr.ValueOrZero(credit.TmdbID), name: credit.Name, pictureRef: ptr.ValueOrZero(credit.PictureRef),
		})
	}
	genres, err := queries.ListLibraryItemGenres(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its genres: %w", err)
	}
	for _, genre := range genres {
		snap.genres[genre.ItemID] = append(snap.genres[genre.ItemID], heldGenre{nameKey: textindex.Fold(genre.Name)})
	}
	return nil
}

// heldCredits are an item's credits: the snapshot's for an item it holds, else the database's.
func (filer *libraryFiler) heldCredits(ctx context.Context, itemID int64) ([]heldCredit, error) {
	if filer.snap.items[itemID] {
		return filer.snap.credits[itemID], nil
	}
	rows, err := filer.queries.ListItemCredits(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("read item %d's credits: %w", itemID, err)
	}
	held := make([]heldCredit, 0, len(rows))
	for _, row := range rows {
		held = append(held, heldCredit{
			role: row.Role, ordinal: row.Ordinal, character: ptr.ValueOrZero(row.Character),
			tmdbID: ptr.ValueOrZero(row.TmdbID), name: row.Name, pictureRef: ptr.ValueOrZero(row.PictureRef),
		})
	}
	return held, nil
}

// heldGenres are an item's genres: the snapshot's for an item it holds, else the database's.
func (filer *libraryFiler) heldGenres(ctx context.Context, itemID int64) ([]heldGenre, error) {
	if filer.snap.items[itemID] {
		return filer.snap.genres[itemID], nil
	}
	rows, err := filer.queries.ListItemGenres(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("read item %d's genres: %w", itemID, err)
	}
	held := make([]heldGenre, 0, len(rows))
	for _, row := range rows {
		held = append(held, heldGenre{nameKey: textindex.Fold(row.Name)})
	}
	return held, nil
}

// wantedCredits are a source's credits as rows: each role's in its order, numbered from 0 within it.
func wantedCredits(credits []Credit) []heldCredit {
	next := map[string]int64{}
	wanted := make([]heldCredit, 0, len(credits))
	for _, role := range []string{RoleCast, RoleDirector, RoleWriter} {
		for _, credit := range credits {
			if credit.Role != role || credit.Name == "" {
				continue
			}
			wanted = append(wanted, heldCredit{
				role: role, ordinal: next[role], character: credit.Character, tmdbID: credit.PersonID,
				name: credit.Name, pictureRef: credit.PictureRef,
			})
			next[role]++
		}
	}
	return wanted
}

// isSameCredit reports whether a held credit is the one wanted. A credit with no TMDB id, an NFO's,
// names its person by their folded name alone, so the person it was filed under, whoever's, is it.
func isSameCredit(held, wanted heldCredit) bool {
	if held.role != wanted.role || held.ordinal != wanted.ordinal || held.character != wanted.character {
		return false
	}
	if wanted.tmdbID == "" {
		return textindex.Fold(held.name) == textindex.Fold(wanted.name)
	}
	return held.tmdbID == wanted.tmdbID && held.name == wanted.name && held.pictureRef == wanted.pictureRef
}

// wantedGenres are a source's genres, each once by its folded name, in its order.
func wantedGenres(genres []string) []string {
	seen := map[string]bool{}
	wanted := make([]string, 0, len(genres))
	for _, genre := range genres {
		key := textindex.Fold(genre)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		wanted = append(wanted, genre)
	}
	return wanted
}

// writePeople keeps an item's genres and credits as its source names them (D186): unchanged, nothing
// is written; changed, each is written again whole, the people it names found or added.
func (filer *libraryFiler) writePeople(ctx context.Context, itemID int64, genres []string, credits []Credit) error {
	if err := filer.writeGenres(ctx, itemID, wantedGenres(genres)); err != nil {
		return err
	}
	return filer.writeCredits(ctx, itemID, wantedCredits(credits))
}

func (filer *libraryFiler) writeGenres(ctx context.Context, itemID int64, wanted []string) error {
	held, err := filer.heldGenres(ctx, itemID)
	if err != nil {
		return err
	}
	if len(held) == len(wanted) {
		same := true
		for index, genre := range wanted {
			same = same && held[index].nameKey == textindex.Fold(genre)
		}
		if same {
			return nil
		}
	}
	if err := filer.queries.DeleteItemGenres(ctx, itemID); err != nil {
		return fmt.Errorf("clear item %d's genres: %w", itemID, err)
	}
	noted := make([]heldGenre, 0, len(wanted))
	for ordinal, genre := range wanted {
		key := textindex.Fold(genre)
		err := filer.queries.InsertGenreIfAbsent(ctx, sqlcgen.InsertGenreIfAbsentParams{Name: genre, NameKey: key})
		if err != nil {
			return fmt.Errorf("add the genre %q: %w", genre, err)
		}
		genreID, err := filer.queries.GetGenreIDByKey(ctx, key)
		if err != nil {
			return fmt.Errorf("find the genre %q: %w", genre, err)
		}
		err = filer.queries.InsertItemGenre(ctx, sqlcgen.InsertItemGenreParams{
			ItemID: itemID, GenreID: genreID, Ordinal: int64(ordinal),
		})
		if err != nil {
			return fmt.Errorf("file item %d under %q: %w", itemID, genre, err)
		}
		noted = append(noted, heldGenre{nameKey: key})
	}
	if filer.snap.items[itemID] {
		filer.snap.genres[itemID] = noted
	}
	return nil
}

func (filer *libraryFiler) writeCredits(ctx context.Context, itemID int64, wanted []heldCredit) error {
	held, err := filer.heldCredits(ctx, itemID)
	if err != nil {
		return err
	}
	if len(held) == len(wanted) {
		same := true
		for index := range wanted {
			same = same && isSameCredit(held[index], wanted[index])
		}
		if same {
			return nil
		}
	}
	if err := filer.queries.DeleteItemCredits(ctx, itemID); err != nil {
		return fmt.Errorf("clear item %d's credits: %w", itemID, err)
	}
	for _, credit := range wanted {
		personID, err := filer.personFor(ctx, credit)
		if err != nil {
			return err
		}
		err = filer.queries.InsertCredit(ctx, sqlcgen.InsertCreditParams{
			ItemID: itemID, Role: credit.role, Ordinal: credit.ordinal, PersonID: personID,
			Character: ptr.NilIfZero(credit.character),
		})
		if err != nil {
			return fmt.Errorf("credit %q on item %d: %w", credit.name, itemID, err)
		}
	}
	if filer.snap.items[itemID] {
		filer.snap.credits[itemID] = wanted
	}
	return nil
}

// personFor is the person a credit names, added when there is none: one with a TMDB id is TMDB's
// person by it, named and pictured as TMDB now has them; one without, an NFO's, is the first person
// of that folded name from any source. A TMDB credit is never filed under a person an NFO alone
// named, whose name may be another's.
func (filer *libraryFiler) personFor(ctx context.Context, credit heldCredit) (int64, error) {
	key := textindex.Fold(credit.name)
	if credit.tmdbID != "" {
		found, err := store.OrNotFound(filer.queries.GetPersonByTMDBID(ctx, &credit.tmdbID))
		switch {
		case err == nil:
			if found.NameKey != key || ptr.ValueOrZero(found.PictureRef) != credit.pictureRef {
				err = filer.queries.UpdatePerson(ctx, sqlcgen.UpdatePersonParams{
					NameKey: key, PictureRef: ptr.NilIfZero(credit.pictureRef), ID: found.ID,
				})
				if err != nil {
					return 0, fmt.Errorf("rename person %d: %w", found.ID, err)
				}
			}
			return found.ID, filer.namePerson(ctx, found.ID, credit.name)
		case !errors.Is(err, store.ErrNotFound):
			return 0, fmt.Errorf("find TMDB's person %s: %w", credit.tmdbID, err)
		}
	} else {
		found, err := store.OrNotFound(filer.queries.GetPersonIDByNameKey(ctx, key))
		switch {
		case err == nil:
			return found, nil
		case !errors.Is(err, store.ErrNotFound):
			return 0, fmt.Errorf("find the person %q: %w", credit.name, err)
		}
	}
	personID, err := filer.queries.InsertItem(ctx, sqlcgen.InsertItemParams{Kind: "person", AddedAt: filer.nowMs})
	if err != nil {
		return 0, fmt.Errorf("add the person %q: %w", credit.name, err)
	}
	err = filer.queries.InsertPerson(ctx, sqlcgen.InsertPersonParams{
		ID: personID, TmdbID: ptr.NilIfZero(credit.tmdbID), NameKey: key, PictureRef: ptr.NilIfZero(credit.pictureRef),
	})
	if err != nil {
		return 0, fmt.Errorf("add the person %q: %w", credit.name, err)
	}
	return personID, filer.namePerson(ctx, personID, credit.name)
}

// namePerson writes a person's name as their item's text, folded for search as a title's is (D48);
// a name already so is not written.
func (filer *libraryFiler) namePerson(ctx context.Context, personID int64, name string) error {
	err := filer.queries.UpsertOriginalText(ctx, sqlcgen.UpsertOriginalTextParams{
		ItemID: personID, Language: langtag.Undetermined, Name: name,
		SearchAlternates: ptr.NilIfZero(textindex.Fold(name)),
	})
	if err != nil {
		return fmt.Errorf("name person %d: %w", personID, err)
	}
	return nil
}
