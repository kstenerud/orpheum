// SPDX-License-Identifier: GPL-3.0-or-later

// Package identify matches titles to provider metadata: the filename parser, NFO read, the TMDB
// client, ranked match candidates, the ambiguous-match chooser, and per-field overrides. An
// ambiguous match is never guessed. A fetched response is stored and never refetched except by a
// user's act; re-deriving a title reads only stored input.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/datadir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - The TMDB client is the only caller of secrets.Value.Reveal here. Enforced by the forbidigo
//     rule tagged [reveal] (this package is on its path allowlist).
//   - Errors from the provider are wrapped where they enter. Enforced by wrapcheck, which is
//     scoped to this package.
//   - The filename parser is a pure function of a path, tested against a corpus that includes
//     non-Latin names, and no expression uses an ASCII-only class (D49).
//     TestParsePath_ACorpusOfNames, TestParsePath_NoExpressionUsesAnASCIIOnlyClass.
//   - An NFO's locked fields become overrides and the rest is the title's source; its watch
//     counts are not read (D182, D183). TestParseNFO_AKodiMovieWithItsCorrectionsLocked.
//   - A library asked for "TMDB only, ignore NFO files" reads none: not their values, ids or
//     locked fields (D183 §2). TestPass_ATMDBOnlyLibraryIgnoresNFOFiles.
//   - The pass reads only stored observations and reaches the same titles when run again (D182).
//     TestPass_RunningAgainChangesNothing.
//   - Films, versions, extras by containment and episodes in their series' aired order come from
//     the paths and NFO files (D155, D162, D96). TestPass_TheDevLibrarysFilms,
//     TestPass_ASeriesFromItsFoldersAndItsTvshowNFO.
//   - A renamed or moved file is identified again from its new path, unless a person confirmed
//     its match (D208 §1). TestPass_ARenamedFileIsIdentifiedAgainUnlessConfirmed.
//   - A title with no live files left follows them to another library, found by provider id
//     (D208 §2). TestPass_ATitleFollowsItsFilesToAnotherLibrary.
//   - A match is never guessed: a provider id in hand skips the search, a single result matching
//     the name and year is accepted, and anything else is ambiguous with its results kept (D172).
//     TestMatch_AProviderIDInHandSkipsTheSearch, TestMatch_AnAmbiguousSearchIsNeverGuessed,
//     TestMatch_ASingleResultMatchingNameAndYearIsAcceptedAndReplayedFromTheStoredResponse.
//   - A pass re-derives a matched title from its stored response and never calls a provider;
//     a title its provider found nothing for is not searched again (D170, D182).
//     TestMatch_ASingleResultMatchingNameAndYearIsAcceptedAndReplayedFromTheStoredResponse,
//     TestMatch_NothingFoundIsRecordedAndNotSearchedAgain.
//   - An override survives a match and every re-derivation (D183).
//     TestMatch_AnOverrideSurvivesTheMatchAndReDerivation.
//   - A person's choice of match is confirmed, over any NFO and every later pass, and replaces
//     the ids an earlier match recorded; an ambiguous title whose kept results cache.db lost is
//     searched once more (D172). TestChooser_AChosenMatchIsPermanent,
//     TestChooser_AChoiceWinsOverTheNFOAndReplacesAnEarlierOne,
//     TestChooser_SearchesAgainWhenTheCacheLostTheCandidates.
//   - A person's override holds until it is removed, then the field follows its source (D183).
//     TestChooser_AnOverrideHoldsUntilItIsRemoved.
//   - For shows, the series is what is matched, ambiguous and chosen; an episode is never
//     searched, and takes its values from its series' season listing, one fetch a season; a new
//     episode missing from a listing fetched before it arrived fetches that listing again (D209).
//     TestSeries_AMatchedSeriesNamesItsEpisodesFromOneSeasonListing,
//     TestSeries_ANewEpisodeFetchesItsSeasonAgain, TestSeries_AnAmbiguousSeriesIsChosenByAPerson,
//     TestSeries_AYearOrAnIDSettlesTheSearch.
//   - A series renamed by its match keeps the new episodes its folders name.
//     TestSeries_AnEpisodeAddedAfterTheSeriesWasRenamedJoinsIt.
//   - A provider's episode ids never find a film, nor its film ids an episode.
//     Enforced by titleByProviderID's kind filter; review.
//   - Every name the pass writes is folded for search and given a sort key in the sort_locale,
//     through writeText alone (D47, D48). TestPass_NamesAreSortedAndFoundInEveryScript.
//   - A change of sort_locale re-keys every name from what is stored, with no scan (A4).
//     TestRefreshSortKeys_ALocaleChangeTakesEffectWithoutAScan,
//     TestFollowSortLocale_KeysEveryNameAtStartAndOnEveryChange.
//   - One work found as two titles in a library becomes one, the other a tombstone pointing at it
//     (D173). TestMatch_ASecondTitleForTheSameWorkJoinsTheFirst.
//
// # Read before changing
//
//   - D170: one source per title, fetched once; every refetch is an act of the user.
//   - D172: an ambiguous match is never guessed.
//   - D182: metadata is a stack of layers; re-derivation reads only stored input.
//   - D183: an override is an override wherever its value came from.
//   - D209: for shows, the series is what is identified.
//   - D141: NFO lock markers.
//   - docs/contributors/design/plans/identification.md.
//
// # Where to change
//
//   - filename.go: the filename parser, and its corpus in filename_test.go; nfo.go: the NFO
//     reader.
//   - pass.go, load.go, titles.go, episodes.go, extras.go: the identification pass; evidence.go: what a
//     stored match says to it.
//   - provider.go: the provider interface. match.go: the matcher and the fetch job's handler;
//     match_series.go: a series' match and its season listings. series.go: a series in the pass.
//   - chooser.go: what a person does — list, choose, override. tmdb/: the TMDB client.
package identify
