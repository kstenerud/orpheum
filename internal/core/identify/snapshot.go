// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// snapshot is what a pass reads of its library at its start, in a few statements whatever the
// library's size, and compares against before it writes (D303, DL7, DL9): over an unchanged library
// the pass then runs no statement per file and writes nothing, so it runs in a read transaction
// alone (store.WriteUnlessUnchanged). Each write the pass makes through it is noted in it, so a
// later comparison sees it. An item it does not hold, one the pass brings in from another library
// or back from a deleted one, is read from the database; one the pass adds is held from its insert,
// with nothing yet. A filer with no snapshot loaded, a match's, reads and compares nothing here.
type snapshot struct {
	isLoaded bool
	// items are the live items of the library it holds, and those the pass added: an item's
	// overrides, ids and text are all here, or it has none.
	items         map[int64]bool
	versionTitles map[int64]sqlcgen.GetVersionTitleRow // by version
	titles        map[int64]sqlcgen.ListLibraryTitlesForPassRow
	overrides     map[int64]map[string]string // by item, then field
	providerIDs   map[int64]map[providerID]bool
	itemsByID     map[providerID][]int64 // the held items each provider id names
	texts         map[int64]sqlcgen.ListLibraryOriginalTextsRow
	nfoBodies     map[int64][]byte // by NFO file
	responses     map[responseKey][]byte
	fetchedAt     map[responseKey]int64 // when each response was fetched
	// episodeSources are each live series' episodes, as ListLiveSeriesEpisodeSources lists them:
	// read once, when the first series is resolved, after its episodes are (seasonsToFetch).
	episodeSources map[int64][]sqlcgen.ListLiveSeriesEpisodeSourcesRow
	// places are each held title's place in its series' aired order; a title with two is left
	// out, and read.
	places    map[int64]sqlcgen.ListLibraryAiredPositionsRow
	orderings map[int64]int64 // each held series' aired ordering
	members   map[memberKey]*float64
	// seriesValues are each held series' resolved values, as UpdateSeriesResolved compares them.
	seriesValues map[int64]sqlcgen.UpdateSeriesResolvedParams
	// seriesPlaces are each held series' episodes in its aired order, as ListSeriesEpisodePlaces
	// lists them.
	seriesPlaces map[int64][]sqlcgen.ListSeriesEpisodePlacesRow
	seriesMatch  map[int64]sqlcgen.GetSeriesForMatchRow
	extras       map[memberKey]sqlcgen.ListLibraryExtraRelationsRow // by owner, then extra
	// otherVideos are each held other video's date and file name, as the pass last wrote them.
	otherVideos map[int64]sqlcgen.UpsertOtherVideoParams
	// credits and genres are each held item's, as the pass last wrote them.
	credits map[int64][]heldCredit
	genres  map[int64][]heldGenre
}

// responseKey names one stored provider response.
type responseKey struct {
	itemID         int64
	provider, part string
}

// memberKey names a title in a series.
type memberKey struct{ seriesID, titleID int64 }

// providerID is an id at a provider.
type providerID struct{ provider, id string }

func providerIDOf(provider, id string) providerID { return providerID{provider: provider, id: id} }

// loadSnapshot reads the library's versions, titles, overrides, provider ids, text and NFO bodies.
func (filer *libraryFiler) loadSnapshot(ctx context.Context, libraryID int64) error {
	snap := snapshot{
		isLoaded: true, items: map[int64]bool{}, versionTitles: map[int64]sqlcgen.GetVersionTitleRow{},
		titles: map[int64]sqlcgen.ListLibraryTitlesForPassRow{}, overrides: map[int64]map[string]string{},
		providerIDs: map[int64]map[providerID]bool{}, itemsByID: map[providerID][]int64{},
		texts: map[int64]sqlcgen.ListLibraryOriginalTextsRow{}, nfoBodies: map[int64][]byte{},
		responses: map[responseKey][]byte{}, fetchedAt: map[responseKey]int64{},
		places:    map[int64]sqlcgen.ListLibraryAiredPositionsRow{},
		orderings: map[int64]int64{}, members: map[memberKey]*float64{},
		seriesValues: map[int64]sqlcgen.UpdateSeriesResolvedParams{},
		seriesPlaces: map[int64][]sqlcgen.ListSeriesEpisodePlacesRow{},
		seriesMatch:  map[int64]sqlcgen.GetSeriesForMatchRow{},
		extras:       map[memberKey]sqlcgen.ListLibraryExtraRelationsRow{},
		credits:      map[int64][]heldCredit{}, genres: map[int64][]heldGenre{},
	}
	versions, err := filer.queries.ListLibraryVersionTitles(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("list the titles of its versions: %w", err)
	}
	for _, version := range versions {
		snap.versionTitles[version.ID] = sqlcgen.GetVersionTitleRow{
			TitleID: version.TitleID, IdentificationState: version.IdentificationState,
		}
	}
	titles, err := filer.queries.ListLibraryTitlesForPass(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its titles: %w", err)
	}
	for _, title := range titles {
		snap.items[title.ID], snap.titles[title.ID] = true, title
	}
	if loadErr := snap.loadItemRows(ctx, filer.queries, libraryID, filer.sorter.Locale()); loadErr != nil {
		return loadErr
	}
	if loadErr := snap.loadSeries(ctx, filer.queries, libraryID); loadErr != nil {
		return loadErr
	}
	if loadErr := snap.loadOtherVideos(ctx, filer.queries, libraryID); loadErr != nil {
		return loadErr
	}
	if loadErr := snap.loadPeople(ctx, filer.queries, libraryID); loadErr != nil {
		return loadErr
	}
	responses, err := filer.queries.ListLibraryProviderResponses(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("read its stored responses: %w", err)
	}
	for _, response := range responses {
		key := responseKey{response.ItemID, response.Provider, response.Part}
		snap.responses[key], snap.fetchedAt[key] = response.BodyGzip, response.FetchedAt
	}
	extras, err := filer.queries.ListLibraryExtraRelations(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its extras: %w", err)
	}
	for _, extra := range extras {
		snap.extras[memberKey{extra.OwnerTitle, extra.ExtraTitle}] = extra
	}
	bodies, err := filer.queries.ListLibraryNFOBodies(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("read its NFO files: %w", err)
	}
	for _, nfo := range bodies {
		snap.nfoBodies[nfo.FileID] = nfo.Body
	}
	filer.snap = snap
	return nil
}

// loadSeries reads the library's series, their aired orders, members and episodes' places.
func (snap *snapshot) loadSeries(ctx context.Context, queries *sqlcgen.Queries, libraryID int64) error {
	series, err := queries.ListLibrarySeriesForPass(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its series: %w", err)
	}
	for _, show := range series {
		snap.items[show.ID] = true
		if show.AiredOrderingID != nil {
			snap.orderings[show.ID] = *show.AiredOrderingID
		}
		snap.seriesMatch[show.ID] = sqlcgen.GetSeriesForMatchRow{
			ID: show.ID, Year: show.Year, FollowedSource: show.FollowedSource, FollowedSourceID: show.FollowedSourceID,
			IdentificationState: show.IdentificationState, FilingLibraryID: show.FilingLibraryID,
			MetadataSource: show.MetadataSource, Name: show.Name,
		}
		snap.seriesValues[show.ID] = sqlcgen.UpdateSeriesResolvedParams{
			Year: show.Year, ReleasedOn: show.ReleasedOn, FollowedSource: show.FollowedSource,
			IdentificationState: show.IdentificationState, ID: show.ID,
		}
		snap.seriesPlaces[show.ID] = nil
	}
	positions, err := queries.ListLibraryAiredPositions(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its episodes' places: %w", err)
	}
	twice := map[int64]bool{}
	for _, position := range positions {
		if _, seen := snap.places[position.TitleID]; seen {
			twice[position.TitleID] = true
		}
		snap.places[position.TitleID] = position
	}
	for titleID := range twice {
		delete(snap.places, titleID)
	}
	members, err := queries.ListLibrarySeriesMembers(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its series' members: %w", err)
	}
	for _, member := range members {
		snap.members[memberKey{member.CollectionID, *member.MemberTitle}] = member.Position
	}
	return snap.loadSeriesPlaces(ctx, queries, libraryID)
}

// loadSeriesPlaces reads every held series' episodes in its aired order.
func (snap *snapshot) loadSeriesPlaces(ctx context.Context, queries *sqlcgen.Queries, libraryID int64) error {
	places, err := queries.ListLibrarySeriesEpisodePlaces(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its series' episodes: %w", err)
	}
	for _, place := range places {
		snap.seriesPlaces[place.CollectionID] = append(snap.seriesPlaces[place.CollectionID],
			sqlcgen.ListSeriesEpisodePlacesRow{
				TitleID: place.TitleID, GroupNumber: place.GroupNumber, NumberInGroup: place.NumberInGroup,
				RemovedAt: place.RemovedAt,
			})
	}
	return nil
}

// loadItemRows reads the overrides, provider ids and text of the library's live items.
func (snap *snapshot) loadItemRows(
	ctx context.Context, queries *sqlcgen.Queries, libraryID int64, locale string,
) error {
	overrides, err := queries.ListLibraryOverrides(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its overrides: %w", err)
	}
	for _, override := range overrides {
		snap.noteOverride(override.ItemID, override.Field, override.OverrideValue)
	}
	ids, err := queries.ListLibraryProviderIDs(ctx, &libraryID)
	if err != nil {
		return fmt.Errorf("list its provider ids: %w", err)
	}
	for _, id := range ids {
		snap.noteProviderID(id.ItemID, providerID{provider: id.Provider, id: id.ProviderID})
	}
	texts, err := queries.ListLibraryOriginalTexts(ctx, sqlcgen.ListLibraryOriginalTextsParams{
		Locale: locale, LibraryID: &libraryID,
	})
	if err != nil {
		return fmt.Errorf("list its text: %w", err)
	}
	for _, text := range texts {
		snap.texts[text.ItemID] = text
	}
	return nil
}

// holdSeries adds a live series of the library to the items held.
func (snap *snapshot) holdSeries(seriesID int64) {
	if snap.isLoaded {
		snap.items[seriesID] = true
	}
}

// holdAdded holds an item the pass just added, which has nothing yet; a title with its kind.
func (snap *snapshot) holdAdded(itemID int64, titleKind string) {
	if !snap.isLoaded {
		return
	}
	snap.items[itemID] = true
	if titleKind != "" {
		snap.titles[itemID] = sqlcgen.ListLibraryTitlesForPassRow{
			ID: itemID, Kind: titleKind, IdentificationState: identificationUnidentified,
		}
	}
}

func (snap *snapshot) noteOverride(itemID int64, field, value string) {
	if snap.overrides[itemID] == nil {
		snap.overrides[itemID] = map[string]string{}
	}
	snap.overrides[itemID][field] = value
}

func (snap *snapshot) noteProviderID(itemID int64, id providerID) {
	if snap.providerIDs[itemID] == nil {
		snap.providerIDs[itemID] = map[providerID]bool{}
	}
	if !snap.providerIDs[itemID][id] {
		snap.providerIDs[itemID][id] = true
		snap.itemsByID[id] = append(snap.itemsByID[id], itemID)
	}
}

// versionTitle is the title a version is in, with that title's identification.
func (filer *libraryFiler) versionTitle(ctx context.Context, versionID int64) (sqlcgen.GetVersionTitleRow, error) {
	if row, held := filer.snap.versionTitles[versionID]; held {
		return row, nil
	}
	return filer.queries.GetVersionTitle(ctx, versionID) //nolint:wrapcheck // its callers say which file
}

// moveVersion moves a version to another title.
func (filer *libraryFiler) moveVersion(ctx context.Context, versionID, titleID int64) error {
	err := filer.queries.MoveVersionToTitle(ctx, sqlcgen.MoveVersionToTitleParams{TitleID: titleID, ID: versionID})
	delete(filer.snap.versionTitles, versionID) // read again, with its new title's identification
	return err                                  //nolint:wrapcheck // its caller says which file
}

// heldTitleByProviderID is the first live title of a kind in the library that a provider's id
// names, as ListTitlesByProviderID orders them; false when the snapshot cannot say.
func (filer *libraryFiler) heldTitleByProviderID(kind string, id providerID) (int64, bool) {
	if !filer.snap.isLoaded {
		return 0, false
	}
	var found []int64
	for _, itemID := range filer.snap.itemsByID[id] {
		if title, isTitle := filer.snap.titles[itemID]; isTitle && title.Kind == kind {
			found = append(found, itemID)
		}
	}
	if len(found) == 0 {
		return 0, false
	}
	return slices.Min(found), true
}

// titleMatchState is what a title's match says, for matchEvidence: its identification and the
// source it follows, and the library's metadata source; false for no live title.
func (filer *libraryFiler) titleMatchState(ctx context.Context, titleID int64) (titleMatchRow, bool, error) {
	if title, held := filer.snap.titles[titleID]; held {
		return titleMatchRow{
			identificationState: title.IdentificationState, followedSource: title.FollowedSource,
			metadataSource: filer.library.MetadataSource,
		}, true, nil
	}
	row, err := store.OrNotFound(filer.queries.GetTitleForMatch(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return titleMatchRow{}, false, nil
	}
	if err != nil {
		return titleMatchRow{}, false, fmt.Errorf("read title %d's match: %w", titleID, err)
	}
	return titleMatchRow{
		identificationState: row.IdentificationState, followedSource: row.FollowedSource,
		metadataSource: row.MetadataSource,
	}, true, nil
}

// titleMatchRow is what matchEvidence reads of a title's match.
type titleMatchRow struct {
	identificationState string
	followedSource      *string
	metadataSource      string
}

// itemOverrides are an item's overrides.
func (filer *libraryFiler) itemOverrides(ctx context.Context, itemID int64) ([]sqlcgen.ListOverridesRow, error) {
	if !filer.snap.items[itemID] {
		return filer.queries.ListOverrides(ctx, itemID) //nolint:wrapcheck // its callers say which item
	}
	held := filer.snap.overrides[itemID]
	overrides := make([]sqlcgen.ListOverridesRow, 0, len(held))
	for _, field := range slices.Sorted(maps.Keys(held)) {
		overrides = append(overrides, sqlcgen.ListOverridesRow{Field: field, OverrideValue: held[field]})
	}
	return overrides, nil
}

// overrideIfAbsent makes a field an override with a value, unless the item already has one for it.
func (filer *libraryFiler) overrideIfAbsent(ctx context.Context, itemID int64, field, value, origin string) error {
	if filer.snap.items[itemID] {
		if _, has := filer.snap.overrides[itemID][field]; has {
			return nil
		}
		filer.snap.noteOverride(itemID, field, value)
	}
	override := sqlcgen.InsertOverrideIfAbsentParams{
		ItemID: itemID, Field: field, OverrideValue: value, Origin: origin, SetAt: filer.nowMs,
	}
	return filer.queries.InsertOverrideIfAbsent(ctx, override) //nolint:wrapcheck // its callers say which field
}

// keepProviderID records a provider's id for an item, unless it has it.
func (filer *libraryFiler) keepProviderID(ctx context.Context, itemID int64, provider, id string) error {
	if filer.snap.items[itemID] {
		key := providerID{provider: provider, id: id}
		if filer.snap.providerIDs[itemID][key] {
			return nil
		}
		filer.snap.noteProviderID(itemID, key)
	}
	kept := sqlcgen.InsertItemProviderIDParams{ItemID: itemID, Provider: provider, ProviderID: id}
	return filer.queries.InsertItemProviderID(ctx, kept) //nolint:wrapcheck // its callers say which id
}

// resolveTitleValues writes a title's resolved values, unless they are already its own; a
// confirmed match stays confirmed (UpdateTitleResolved). It returns the title's identification.
func (filer *libraryFiler) resolveTitleValues(
	ctx context.Context, values sqlcgen.UpdateTitleResolvedParams) (string, error,
) {
	held, isHeld := filer.snap.titles[values.ID]
	if isHeld {
		resolved := held
		resolved.Kind, resolved.Year, resolved.RuntimeMs = values.Kind, values.Year, values.RuntimeMs
		resolved.ReleasedOn = values.ReleasedOn
		resolved.FollowedSource, resolved.FollowedSourceID = values.FollowedSource, values.FollowedSourceID
		if held.IdentificationState != identificationConfirmed {
			resolved.IdentificationState = values.IdentificationState
		}
		if isSameTitleValues(resolved, held) {
			return held.IdentificationState, nil
		}
		filer.snap.titles[values.ID] = resolved
	}
	if err := filer.queries.UpdateTitleResolved(ctx, values); err != nil {
		return "", fmt.Errorf("resolve title %d: %w", values.ID, err)
	}
	if isHeld {
		return filer.snap.titles[values.ID].IdentificationState, nil
	}
	identification, err := store.OrNotFound(filer.queries.GetTitleIdentification(ctx, values.ID))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", fmt.Errorf("read title %d's identification: %w", values.ID, err)
	}
	return identification, nil
}

// isSameTitleValues is whether two of a title's rows hold the same values.
func isSameTitleValues(first, second sqlcgen.ListLibraryTitlesForPassRow) bool {
	return first.ID == second.ID && first.Kind == second.Kind && ptr.Equal(first.Year, second.Year) &&
		ptr.Equal(first.ReleasedOn, second.ReleasedOn) &&
		ptr.Equal(first.RuntimeMs, second.RuntimeMs) && first.IdentificationState == second.IdentificationState &&
		ptr.Equal(first.FollowedSource, second.FollowedSource) &&
		ptr.Equal(first.FollowedSourceID, second.FollowedSourceID)
}

// isTextHeld is whether an item's text and its sort key in the pass's locale are already these.
func (filer *libraryFiler) isTextHeld(
	itemID int64, text sqlcgen.UpsertOriginalTextParams, sortKey sqlcgen.UpsertSortKeyParams,
) bool {
	held, isHeld := filer.snap.texts[itemID]
	return isHeld && held.Language == text.Language && held.Name == text.Name &&
		ptr.Equal(held.Overview, text.Overview) && ptr.Equal(held.Tagline, text.Tagline) &&
		ptr.Equal(held.SearchAlternates, text.SearchAlternates) && ptr.Equal(held.SortTitle, text.SortTitle) &&
		ptr.Equal(held.Listing, &sortKey.Listing) &&
		bytes.Equal(held.CollationKey, sortKey.CollationKey)
}

// noteText holds the text and sort key the pass just wrote of a held item.
func (filer *libraryFiler) noteText(
	itemID int64, text sqlcgen.UpsertOriginalTextParams, sortKey sqlcgen.UpsertSortKeyParams,
) {
	if !filer.snap.items[itemID] {
		return
	}
	listing := sortKey.Listing
	filer.snap.texts[itemID] = sqlcgen.ListLibraryOriginalTextsRow{
		ItemID: itemID, Language: text.Language, Name: text.Name, Overview: text.Overview, Tagline: text.Tagline,
		SearchAlternates: text.SearchAlternates, SortTitle: text.SortTitle,
		Listing: &listing, CollationKey: sortKey.CollationKey,
	}
}

// fileAs files a file, unless it is already filed so.
func (filer *libraryFiler) fileAs(ctx context.Context, file sqlcgen.ListIdentifyFilesRow, filing, reason string) error {
	if filer.snap.isLoaded && file.Filing == filing && ptr.ValueOrZero(file.FilingReason) == reason &&
		(file.FilingReason == nil) == (reason == "") {
		return nil
	}
	//nolint:wrapcheck // its caller says which file
	return library.FileAs(ctx, filer.queries, file.ID, filer.library.Kind, filing, reason)
}

// nfoBody is an NFO file's body.
func (filer *libraryFiler) nfoBody(ctx context.Context, fileID int64) ([]byte, error) {
	if filer.snap.isLoaded {
		return filer.snap.nfoBodies[fileID], nil
	}
	return filer.queries.GetNFOBody(ctx, fileID) //nolint:wrapcheck // its caller says which file
}

// episodePlace is a title's series and its place in the series' aired order; false for none.
func (filer *libraryFiler) episodePlace(ctx context.Context, titleID int64) (sqlcgen.GetEpisodePlaceRow, bool, error) {
	if filer.snap.items[titleID] {
		place, held := filer.snap.places[titleID]
		if series, isSeriesHeld := filer.snap.seriesMatch[place.CollectionID]; held && isSeriesHeld {
			return sqlcgen.GetEpisodePlaceRow{
				CollectionID: place.CollectionID, GroupNumber: place.GroupNumber, NumberInGroup: place.NumberInGroup,
				SeriesName: series.Name,
			}, true, nil
		}
	}
	place, err := store.OrNotFound(filer.queries.GetEpisodePlace(ctx, titleID))
	if errors.Is(err, store.ErrNotFound) {
		return sqlcgen.GetEpisodePlaceRow{}, false, nil
	}
	if err != nil {
		return sqlcgen.GetEpisodePlaceRow{}, false, fmt.Errorf("read title %d's series: %w", titleID, err)
	}
	return place, true, nil
}

// airedOrdering is a held series' aired order, made when it has none yet.
func (filer *libraryFiler) airedOrdering(ctx context.Context, seriesID int64) (int64, error) {
	if ordering, held := filer.snap.orderings[seriesID]; held {
		return ordering, nil
	}
	ordering, err := airedOrderingOf(ctx, filer.queries, seriesID)
	if err == nil && filer.snap.items[seriesID] {
		filer.snap.orderings[seriesID] = ordering
	}
	return ordering, err
}

// keepMember puts a title in a series at a place, unless it is there already.
func (filer *libraryFiler) keepMember(ctx context.Context, member sqlcgen.UpsertCollectionMemberParams) error {
	key := memberKey{member.CollectionID, *member.MemberTitle}
	if held, isHeld := filer.snap.members[key]; isHeld && ptr.Equal(held, member.Position) {
		return nil
	}
	if filer.snap.isLoaded {
		filer.snap.members[key] = member.Position
	}
	return filer.queries.UpsertCollectionMember(ctx, member) //nolint:wrapcheck // its callers say which title
}

// keepPosition places a title in an ordering, unless it is placed so already.
func (filer *libraryFiler) keepPosition(ctx context.Context, position sqlcgen.UpsertPositionParams) error {
	held, isHeld := filer.snap.places[position.TitleID]
	if isHeld && held.OrderingID == position.OrderingID && ptr.Equal(held.GroupNumber, position.GroupNumber) &&
		ptr.Equal(held.NumberInGroup, position.NumberInGroup) && held.SortOrder == position.SortOrder {
		return nil
	}
	if isHeld && held.OrderingID == position.OrderingID {
		held.GroupNumber, held.NumberInGroup, held.SortOrder = position.GroupNumber, position.NumberInGroup,
			position.SortOrder
		filer.snap.places[position.TitleID] = held
	}
	return filer.queries.UpsertPosition(ctx, position) //nolint:wrapcheck // its callers say which title
}

// resolveSeriesValues writes a series' resolved values, unless they are already its own; an
// identified, ambiguous or confirmed series keeps its state (UpdateSeriesResolved).
func (filer *libraryFiler) resolveSeriesValues(ctx context.Context, values sqlcgen.UpdateSeriesResolvedParams) error {
	if held, isHeld := filer.snap.seriesValues[values.ID]; isHeld {
		resolved := values
		switch ptr.ValueOrZero(held.IdentificationState) {
		case identificationIdentified, identificationAmbiguous, identificationConfirmed:
			resolved.IdentificationState = held.IdentificationState
		}
		if ptr.Equal(held.Year, resolved.Year) && ptr.Equal(held.ReleasedOn, resolved.ReleasedOn) &&
			ptr.Equal(held.FollowedSource, resolved.FollowedSource) &&
			ptr.Equal(held.IdentificationState, resolved.IdentificationState) {
			return nil
		}
		filer.snap.seriesValues[values.ID] = resolved
	}
	if err := filer.queries.UpdateSeriesResolved(ctx, values); err != nil {
		return fmt.Errorf("resolve series %d: %w", values.ID, err)
	}
	return nil
}

// seriesEpisodePlaces are a series' episodes in its aired order, as ListSeriesEpisodePlaces lists
// them.
func (filer *libraryFiler) seriesEpisodePlaces(
	ctx context.Context, seriesID int64) ([]sqlcgen.ListSeriesEpisodePlacesRow, error,
) {
	if places, held := filer.snap.seriesPlaces[seriesID]; held {
		return places, nil
	}
	return filer.queries.ListSeriesEpisodePlaces(ctx, seriesID) //nolint:wrapcheck // its caller says which series
}

// storedResponse is an item's stored response from a provider, one part of it; false for none.
func (filer *libraryFiler) storedResponse(
	ctx context.Context, itemID int64, provider, part string) ([]byte, bool, error,
) {
	if filer.snap.items[itemID] {
		body, held := filer.snap.responses[responseKey{itemID, provider, part}]
		return body, held, nil
	}
	body, err := store.OrNotFound(filer.queries.GetProviderResponse(ctx, sqlcgen.GetProviderResponseParams{
		ItemID: itemID, Provider: provider, Part: part,
	}))
	if errors.Is(err, store.ErrNotFound) {
		return nil, false, nil
	}
	return body, err == nil, err //nolint:wrapcheck // its callers say which item
}

// ownResponse is an item's stored response about itself, part "", which it must have.
func (filer *libraryFiler) ownResponse(ctx context.Context, itemID int64, provider string) ([]byte, error) {
	body, found, err := filer.storedResponse(ctx, itemID, provider, "")
	if err == nil && !found {
		err = store.ErrNotFound
	}
	return body, err
}

// liveEpisodeSources are a series' live episodes, as ListLiveSeriesEpisodeSources lists them: the
// whole library's read in one statement the first time a pass asks (seasonReads).
func (filer *libraryFiler) liveEpisodeSources(
	ctx context.Context, seriesID int64) ([]sqlcgen.ListLiveSeriesEpisodeSourcesRow, error,
) {
	if !filer.snap.isLoaded {
		return filer.queries.ListLiveSeriesEpisodeSources(ctx, seriesID) //nolint:wrapcheck // seasonsToFetch says which
	}
	if filer.snap.episodeSources == nil {
		rows, err := filer.queries.ListLibraryLiveEpisodeSources(ctx, &filer.library.ID)
		if err != nil {
			return nil, err //nolint:wrapcheck // seasonsToFetch says which
		}
		filer.snap.episodeSources = map[int64][]sqlcgen.ListLiveSeriesEpisodeSourcesRow{}
		for _, row := range rows {
			filer.snap.episodeSources[row.CollectionID] = append(filer.snap.episodeSources[row.CollectionID],
				sqlcgen.ListLiveSeriesEpisodeSourcesRow{
					TitleID: row.TitleID, GroupNumber: row.GroupNumber, NumberInGroup: row.NumberInGroup,
					FollowedSource: row.FollowedSource, AddedAt: row.AddedAt,
				})
		}
	}
	if sources, held := filer.snap.episodeSources[seriesID]; held || filer.snap.items[seriesID] {
		return sources, nil
	}
	return filer.queries.ListLiveSeriesEpisodeSources(ctx, seriesID) //nolint:wrapcheck // seasonsToFetch says which
}

// listing is a season's stored listing, parsed once a pass; nil when it was never fetched.
func (filer *libraryFiler) listing(ctx context.Context, where seasonAtProvider) ([]Episode, error) {
	episodes, _, err := filer.seasonListing(ctx, where.seriesID, where.provider, where.season)
	return episodes, err
}

// seasonsForUnplaced are every season the series' own response lists, when it has an episode with
// no number its name may place (D307 §2).
func (filer *libraryFiler) seasonsForUnplaced(ctx context.Context, provider Provider, seriesID int64) ([]int, error) {
	if filer.seriesWithUnplaced == nil {
		rows, err := filer.queries.ListLibraryUnnumberedEpisodePaths(ctx, &filer.library.ID)
		if err != nil {
			return nil, fmt.Errorf("list the episodes with no number: %w", err)
		}
		filer.seriesWithUnplaced = map[int64]bool{}
		for _, row := range rows {
			if isPlaceableByName(row.Path) {
				filer.seriesWithUnplaced[row.SeriesID] = true
			}
		}
	}
	if !filer.seriesWithUnplaced[seriesID] {
		return nil, nil
	}
	body, _, err := filer.storedResponse(ctx, seriesID, provider.Name(), "")
	if err != nil {
		return nil, fmt.Errorf("read series %d's own response: %w", seriesID, err)
	}
	return listedSeasons(provider, body), nil
}

// listingFetchedAt is when a season's stored listing was fetched.
func (filer *libraryFiler) listingFetchedAt(ctx context.Context, where seasonAtProvider) (int64, error) {
	key := responseKey{where.seriesID, where.provider.Name(), seasonPart(where.season)}
	if fetchedAt, held := filer.snap.fetchedAt[key]; held {
		return fetchedAt, nil
	}
	listing := sqlcgen.GetProviderResponseFetchedAtParams{
		ItemID: where.seriesID, Provider: where.provider.Name(), Part: seasonPart(where.season),
	}
	return filer.queries.GetProviderResponseFetchedAt(ctx, listing) //nolint:wrapcheck // isListingStale says which
}

// keepExtraRelation makes a title an extra of another, of a type, unless it is one already, or a
// person said what it is (UpsertScannerExtraRelation).
func (filer *libraryFiler) keepExtraRelation(
	ctx context.Context, relation sqlcgen.UpsertScannerExtraRelationParams,
) error {
	key := memberKey{relation.OwnerTitle, relation.ExtraTitle}
	held, isHeld := filer.snap.extras[key]
	if isHeld && (held.AssertedBy != assertedByScanner || held.ExtraType == relation.ExtraType) {
		return nil
	}
	if filer.snap.isLoaded {
		filer.snap.extras[key] = sqlcgen.ListLibraryExtraRelationsRow{
			OwnerTitle: relation.OwnerTitle, ExtraTitle: relation.ExtraTitle, ExtraType: relation.ExtraType,
			AssertedBy: assertedByScanner,
		}
	}
	return filer.queries.UpsertScannerExtraRelation(ctx, relation) //nolint:wrapcheck // its caller says which file
}

// assertedByScanner is an extra relation the scanner asserted, which a pass may change (D162).
const assertedByScanner = "scanner"
