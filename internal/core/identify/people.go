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

// heldCredit is one credit of an item as the database holds it, with its person, or as its source
// names it, whose person is not yet found.
type heldCredit struct {
	role       string
	ordinal    int64
	character  string
	tmdbID     string
	name       string
	pictureRef string
	// personID is the person it is filed under, and namedBy the item whose response last named and
	// pictured them, 0 for none (keepPersonNewest); both 0 in a credit as a source names it.
	personID, namedBy int64
}

// heldPerson is a credited person's name and picture, and the item whose response set them.
type heldPerson struct {
	name, pictureRef string
	namedBy          int64
}

// creditSource is the response an item's credits are read from: the item's own stored one, from
// its provider.
type creditSource struct {
	itemID   int64
	provider string
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
		held := heldCredit{
			role: credit.Job, ordinal: credit.Ordinal, character: ptr.ValueOrZero(credit.Character),
			tmdbID: ptr.ValueOrZero(credit.TmdbID), name: credit.Name, pictureRef: ptr.ValueOrZero(credit.PictureRef),
			personID: credit.PersonID, namedBy: ptr.ValueOrZero(credit.NamedByItemID),
		}
		snap.credits[credit.ItemID] = append(snap.credits[credit.ItemID], held)
		snap.people[credit.PersonID] = heldPerson{name: held.name, pictureRef: held.pictureRef, namedBy: held.namedBy}
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
			role: row.Job, ordinal: row.Ordinal, character: ptr.ValueOrZero(row.Character),
			tmdbID: ptr.ValueOrZero(row.TmdbID), name: row.Name, pictureRef: ptr.ValueOrZero(row.PictureRef),
			personID: row.PersonID, namedBy: ptr.ValueOrZero(row.NamedByItemID),
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
// One with a TMDB id is its person by that id: their name and picture are the person's, which every
// item crediting them shares and the newest response sets (keepPersonNewest), so they are no part
// of the credit (the owner, 2026-11-13).
func isSameCredit(held, wanted heldCredit) bool {
	if held.role != wanted.role || held.ordinal != wanted.ordinal || held.character != wanted.character {
		return false
	}
	if wanted.tmdbID == "" {
		return textindex.Fold(held.name) == textindex.Fold(wanted.name)
	}
	return held.tmdbID == wanted.tmdbID
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

// writePeople keeps an item's genres and credits as its source, a provider's stored response or an
// NFO, names them (D186): unchanged, nothing is written; changed, each is written again whole, the
// people it names found or added.
func (filer *libraryFiler) writePeople(
	ctx context.Context, itemID int64, provider string, genres []string, credits []Credit,
) error {
	if err := filer.writeGenres(ctx, itemID, wantedGenres(genres)); err != nil {
		return err
	}
	return filer.writeCredits(ctx, creditSource{itemID: itemID, provider: provider}, wantedCredits(credits))
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

func (filer *libraryFiler) writeCredits(ctx context.Context, source creditSource, wanted []heldCredit) error {
	itemID := source.itemID
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
			return filer.keepCreditedPeopleNewest(ctx, source, held, wanted)
		}
	}
	if err := filer.queries.DeleteItemCredits(ctx, itemID); err != nil {
		return fmt.Errorf("clear item %d's credits: %w", itemID, err)
	}
	for index, credit := range wanted {
		personID, err := filer.personFor(ctx, source, credit)
		if err != nil {
			return err
		}
		wanted[index].personID = personID
		err = filer.queries.InsertCredit(ctx, sqlcgen.InsertCreditParams{
			ItemID: itemID, Job: credit.role, Ordinal: credit.ordinal, PersonID: personID,
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

// keepCreditedPeopleNewest names and pictures the TMDB people of an item's unchanged credits as its
// response does, where it is the newest to credit them (keepPersonNewest).
func (filer *libraryFiler) keepCreditedPeopleNewest(
	ctx context.Context, source creditSource, held, wanted []heldCredit,
) error {
	for index, credit := range wanted {
		if credit.tmdbID == "" {
			continue
		}
		err := filer.keepPersonNewest(ctx, source, held[index].personID, filer.personNow(held[index]), credit)
		if err != nil {
			return err
		}
	}
	return nil
}

// personFor is the person a credit names, added when there is none: one with a TMDB id is TMDB's
// person by it, named and pictured by the newest response that credits them (keepPersonNewest);
// one without, an NFO's, is the first person of that folded name from any source. A TMDB credit is
// never filed under a person an NFO alone named, whose name may be another's.
func (filer *libraryFiler) personFor(ctx context.Context, source creditSource, credit heldCredit) (int64, error) {
	key := textindex.Fold(credit.name)
	if credit.tmdbID != "" {
		found, err := store.OrNotFound(filer.queries.GetPersonByTMDBID(ctx, &credit.tmdbID))
		switch {
		case err == nil:
			person := heldPerson{
				name: ptr.ValueOrZero(found.Name), pictureRef: ptr.ValueOrZero(found.PictureRef),
				namedBy: ptr.ValueOrZero(found.NamedByItemID),
			}
			return found.ID, filer.keepPersonNewest(ctx, source, found.ID, person, credit)
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
	var namedBy *int64
	if credit.tmdbID != "" {
		namedBy = &source.itemID
	}
	err = filer.queries.InsertPerson(ctx, sqlcgen.InsertPersonParams{
		ID: personID, TmdbID: ptr.NilIfZero(credit.tmdbID), NameKey: key, PictureRef: ptr.NilIfZero(credit.pictureRef),
		NamedByItemID: namedBy,
	})
	if err != nil {
		return 0, fmt.Errorf("add the person %q: %w", credit.name, err)
	}
	filer.notePerson(personID, heldPerson{
		name: credit.name, pictureRef: credit.pictureRef, namedBy: ptr.ValueOrZero(namedBy),
	})
	return personID, filer.namePerson(ctx, personID, credit.name)
}

// personNow is a held credit's person as the pass last wrote them: the snapshot's, which a credit
// of another item may have changed since the credit was read, else the credit's own.
func (filer *libraryFiler) personNow(held heldCredit) heldPerson {
	if person, isHeld := filer.snap.people[held.personID]; isHeld {
		return person
	}
	return heldPerson{name: held.name, pictureRef: held.pictureRef, namedBy: held.namedBy}
}

// notePerson keeps a person as the pass wrote them, for the credits of other items read after.
func (filer *libraryFiler) notePerson(personID int64, person heldPerson) {
	if filer.snap.isLoaded {
		filer.snap.people[personID] = person
	}
}

// keepPersonNewest names and pictures TMDB's person as a credit says, when the response it is read
// from is at least as new as the one that last set them: of the responses that credit a person,
// whom every item crediting them shares, the newest names them (the owner, 2026-11-13). Two films
// whose stored responses, fetched at different times, pictured one person differently traded the
// picture on every pass, so a library holding both never settled and its rescan waited for ever
// (staging, 2026-11-13).
func (filer *libraryFiler) keepPersonNewest(
	ctx context.Context, source creditSource, personID int64, person heldPerson, credit heldCredit,
) error {
	if person.name == credit.name && person.pictureRef == credit.pictureRef {
		return nil
	}
	if person.namedBy != 0 && person.namedBy != source.itemID {
		own, err := filer.responseAgeOf(ctx, source.itemID, source.provider)
		if err != nil {
			return err
		}
		setter, err := filer.responseAgeOf(ctx, person.namedBy, source.provider)
		if err != nil {
			return err
		}
		if own.isOlderThan(setter) {
			return nil
		}
	}
	err := filer.queries.UpdatePerson(ctx, sqlcgen.UpdatePersonParams{
		NameKey: textindex.Fold(credit.name), PictureRef: ptr.NilIfZero(credit.pictureRef),
		NamedByItemID: &source.itemID, ID: personID,
	})
	if err != nil {
		return fmt.Errorf("rename person %d from item %d's response: %w", personID, source.itemID, err)
	}
	filer.notePerson(personID, heldPerson{name: credit.name, pictureRef: credit.pictureRef, namedBy: source.itemID})
	return filer.namePerson(ctx, personID, credit.name)
}

// responseAge orders the stored responses that credit one person: the later fetched is the newer,
// and of two fetched at the same time, the later item's. An item with no stored response is older
// than any that has one.
type responseAge struct{ fetchedAt, itemID int64 }

func (age responseAge) isOlderThan(other responseAge) bool {
	return age.fetchedAt < other.fetchedAt || age.fetchedAt == other.fetchedAt && age.itemID < other.itemID
}

// responseAgeOf is the age of an item's own stored response from a provider.
func (filer *libraryFiler) responseAgeOf(ctx context.Context, itemID int64, provider string) (responseAge, error) {
	fetchedAt, err := store.OrNotFound(filer.responseFetchedAt(ctx, responseKey{itemID, provider, ""}))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return responseAge{fetchedAt: -1, itemID: itemID}, nil
	case err != nil:
		return responseAge{}, fmt.Errorf("read when item %d's %s response was fetched: %w", itemID, provider, err)
	}
	return responseAge{fetchedAt: fetchedAt, itemID: itemID}, nil
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
