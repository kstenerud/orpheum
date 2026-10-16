// SPDX-License-Identifier: AGPL-3.0-or-later

// Package identify matches titles to provider metadata: the filename parser, NFO read, the TMDB
// client, ranked match candidates, the ambiguous-match chooser, and per-field overrides. Several
// works of the file's name are settled by a guess, marked as one until a person confirms or
// corrects it (D301); anything else unsettled is never guessed. A fetched response is stored and
// never refetched except by a person or the rolling refresh (D213); re-deriving a title reads only
// stored input. What it owes a provider is derived from persistent.db by its sweeps, never read
// from job history (D231 §3).
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/instancedir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - It reveals no secret: only a provider client may, and the TMDB client is
//     internal/core/providers/tmdb. Enforced by the forbidigo rule tagged [reveal], whose path
//     allowlist does not hold this package.
//   - Errors from the provider are wrapped where they enter. Enforced by wrapcheck, which is
//     scoped to this package.
//   - The filename parser is a pure function of a path, tested against a corpus that includes
//     non-Latin names, and no expression uses an ASCII-only class (D49).
//     TestParsePath_ACorpusOfNames, TestParsePath_NoExpressionUsesAnASCIIOnlyClass.
//   - Whether two names name the same work is textindex.SameWork's answer, and nothing else's: a
//     candidate against the file, a film's name and year, a series' folder against the series
//     filed, an extra's name against its folder (D232).
//     TestPass_ASeriesWhoseFoldersSpellItsNameTwoWaysIsOneSeries,
//     TestPass_ANewFolderSpellingAFiledSeriesNameAnotherWayJoinsIt,
//     TestPass_AFilmSpelledAnotherWayWithItsYearIsTheSameFilm,
//     TestMatch_ACandidateIsNamedAsTheFileIsByTheOneIdentityFold.
//   - An NFO's locked fields become overrides and the rest is the title's source; its watch
//     counts are not read (D182, D183). TestParseNFO_AKodiMovieWithItsCorrectionsLocked.
//   - A library asked for "TMDB only, ignore NFO files" reads none: not their values, ids or
//     locked fields (D183 §2). TestPass_ATMDBOnlyLibraryIgnoresNFOFiles.
//   - The pass reads only stored observations and reaches the same titles when run again (D182).
//     TestPass_RunningAgainChangesNothing. Run again over a library nothing changed in, it writes
//     no row (D303, DL9), nor gives a special no number places a new title.
//     TestPass_AgainOverAnUnchangedLibraryWritesNothing,
//     TestPass_AgainOverAnUnchangedMatchedLibraryWritesNothing,
//     TestPass_AgainOverAnUnchangedLibraryWithAnUnnumberedSpecialWritesNothing. So does a pass over
//     a random library, however its names parse, through renames, removals and NFO changes, and it
//     makes a title for a file at a path at most once (D303, M10).
//     TestPass_OverRandomLibrariesAgainWritesNothingAndMakesNoTitleEachPass.
//   - No live title is left with no version: one a file left is merged into the file's title, and
//     none is made for a file that stays where it is (D173; the store's invariant "every live title
//     has a version"). TestPass_ATitleAFileLeftIsMergedIntoItsNewOneAndNoneIsMadeForAFileThatStays,
//     TestPass_AnEpisodeRenumberedByItsNFOLeavesNoEmptyTitleAtItsNewPlace. A title the pass made and
//     gave no file is a fault, logged.
//   - A title's kind is how the pass files it: an extra with no film is a film, and a film a film
//     comes to own is an extra (D162). TestPass_AConfirmedFilmFiledAsAnExtraTakesTheKindExtra.
//   - A film whose NFO is gone is identified from its path again.
//     TestPass_AFilmWhoseNFOIsDeletedIsIdentifiedFromItsPathAgain.
//   - A pass reads its library in a constant number of statements (snapshot.go), compares before
//     it writes, and runs in a read transaction until it must write: over an unchanged library it
//     runs the same statements at twice the size and takes no writer (D303, DL7, DL9, DL11).
//     TestPass_OverAnUnchangedLibraryRunsTheSameStatementsAtTwiceItsSizeAndTakesNoWriter. What it
//     writes from the snapshot is what a pass reading each row as it goes writes (D303, M9).
//     TestPass_FromTheSnapshotWritesWhatAPassReadingAsItGoesWrites.
//   - A series' match or refresh does not run its library's pass: it marks the library as owing
//     one, and the owed passes' sweep runs each such library's pass once, however many marked it
//     (D303, DL14). TestMatch_SeriesMatchesInOneLibraryAreFollowedByOnePass.
//   - Films, versions, extras by containment and episodes in their series' aired order come from
//     the paths and NFO files (D155, D162, D96). TestPass_TheDevLibrarysFilms,
//     TestPass_ASeriesFromItsFoldersAndItsTvshowNFO.
//   - An extra is one by a vocabulary folder in its title's folder, or by being named for that
//     title with a vocabulary suffix; a suffix alone, or at a library root, makes none. It is filed
//     `extra`, with its type, its owner and the evidence (D162, D163).
//     TestPass_AFileNamedForItsFoldersTitleWithASuffixIsAnExtra, TestPass_TheDevLibrarysFilms.
//   - A renamed or moved file is identified again from its new path, unless a person confirmed
//     its match (D208 §1). TestPass_ARenamedFileIsIdentifiedAgainUnlessConfirmed.
//   - A title with no live files left follows them to another library, found by provider id
//     (D208 §2). TestPass_ATitleFollowsItsFilesToAnotherLibrary. A deleted library's film that
//     reappears in another, named by the same provider id by its NFO or by a match, brings its
//     title back with its history (D231 §7).
//     TestPass_ADeletedLibrarysFilmThatReappearsElsewhereBringsItsTitleBack,
//     TestMatch_ADeletedLibrarysFilmMatchedAgainElsewhereBringsItsTitleBack. A deleted library's
//     series comes back the same way, by its tvshow.nfo's provider id or by a match, and each
//     episode with it as its file reappears; a matched series' episodes it did not have join it.
//     TestPass_ADeletedLibrarysSeriesThatReappearsElsewhereComesBack,
//     TestMatch_ADeletedLibrarysSeriesMatchedAgainElsewhereComesBack.
//   - A provider id in hand skips the search; a single result matching the name and year is
//     accepted (D172). The one result may also be a year off, or a name the file's begins with word
//     by word in its year, each tier asked only when the one before found none (the owner,
//     2026-10-13). TestMatch_AYearOffOrASubtitleStillFindsTheOneWork,
//     TestMatch_AProviderIDInHandSkipsTheSearch,
//     TestMatch_ASingleResultMatchingNameAndYearIsAcceptedAndReplayedFromTheStoredResponse.
//   - Several of the file's name in a tier, or one for a film with no year, are settled by a guess:
//     the oldest with no year, the most voted in the year, and a year off never; the guess keeps its
//     candidates and is marked until a person chooses, whose choice clears it and drops a series'
//     season listings, so its episodes are the chosen series' (D301).
//     TestMatch_SeveralOfOneNameAreSettledByAGuessMarkedAsOne,
//     TestSeries_OfSeveralByItsNameTheOldestIsGuessedAndAPersonCorrectsIt. Anything else unsettled
//     is ambiguous with its results kept, never guessed. TestMatch_AnUnsettledSearchIsNeverGuessed.
//     A film or series left ambiguous that its kept results now settle, or whose kept results
//     cache.db lost, is owed its first match again, and a person's choice never is.
//     TestMatch_AnAmbiguousFilmTheRulesNowSettleIsMatchedAgain.
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
//   - In a series library every video is an episode: a marker, "Show - 01 - Its Name" or an "E01"
//     numbers it, and anything else, or in a specials or extras folder, is a special of season 0 in
//     the series its outermost folder names; a folder named for one episode names no series of its
//     own (the owner, 2026-10-13). TestPass_ASeriesLibraryFilesEveryVideoAsAnEpisodeOrASpecialNeverAFilm.
//     A placement its path now contradicts, in a series no one matched or confirmed, is filed again
//     though the file never moved. TestPass_EpisodesAnOlderReadingSplitIntoSeriesOfTheirOwnComeTogether.
//   - For shows, the series is what is matched, ambiguous and chosen; an episode is never
//     searched, and takes its values from its series' season listing, one fetch a season; a new
//     episode missing from a listing fetched before it arrived fetches that listing again (D209).
//     TestSeries_AMatchedSeriesNamesItsEpisodesFromOneSeasonListing,
//     TestSeries_ANewEpisodeFetchesItsSeasonAgain, TestSeries_OfSeveralByItsNameTheOldestIsGuessedAndAPersonCorrectsIt,
//     TestSeries_AYearOrAnIDSettlesTheSearch.
//   - A series renamed by its match keeps the new episodes its folders name.
//     TestSeries_AnEpisodeAddedAfterTheSeriesWasRenamedJoinsIt.
//   - A provider's episode ids never find a film, nor its film ids an episode.
//     Enforced by claimTitleByProviderID's kind filter; review.
//   - Every name the pass writes is folded for search and given a sort key in the sort_locale,
//     through writeText alone (D47, D48). TestPass_NamesAreSortedAndFoundInEveryScript.
//   - An NFO, every provider's answer and a person's override are cleaned where they enter,
//     so every name the pass writes is clean (principles §22, D211 §2). TestPass_ANameIsStoredClean,
//     TestMatch_AProvidersTextIsCleanWhereItEnters, TestChooser_AnOverrideIsCleanWhereItEnters.
//   - A change of sort_locale re-keys every name from what is stored, with no scan (A4); so does a
//     build that collates or folds a name otherwise, at its start, key by key and fold by fold
//     (D47, D48). TestRefreshSortAndSearchKeys_ALocaleChangeTakesEffectWithoutAScan,
//     TestKeepSortAndSearchKeysCurrent_KeysEveryNameAtStartAndOnEveryChange,
//     TestRefreshSortAndSearchKeys_AKeyOrFoldAnotherBuildMadeIsMadeAgain.
//   - One work found as two titles in a library becomes one, the other a tombstone pointing at it
//     (D173). TestMatch_ASecondTitleForTheSameWorkJoinsTheFirst.
//   - Images are found here and held by the artwork service: this package never imports
//     internal/core/artwork nor writes the artwork table, and reports what it found to whatever
//     follows the pass, which the server's wiring hands to the artwork service (schema.md, "Who
//     writes each table"). TestArch_EachTableHasOneWritingPackage; the import, review.
//   - The images a matched item's source names are reported by every pass and by the match
//     itself (D212); a rematch reports the new work's images, so the earlier work's go (D223 §9).
//     TestPass_ReportsTheImagesAMatchNamesOnEveryPass, TestChoose_ARematchReportsTheNewWorksImages.
//   - Images beside the media are matched to their film, series or episode by Kodi's and
//     Jellyfin's names and found in every library at once, and each pass reports all it found, so
//     one no longer found goes; every image a provider names is reported, and which are owed where
//     the library prefers the images beside the media is the artwork service's to say (D215).
//     TestPass_ImagesBesideTheMediaAreFoundByTheirConventionalNames,
//     TestPass_AnImageNoLongerBesideTheMediaIsNoLongerReported,
//     TestPass_ReportsEveryImageTheProviderNamesBesideThoseBesideTheMedia.
//   - A person's refresh, and the recheck of a missing image, ask for the item's missing images
//     again through the ImageRetrier the wiring sets (D213 §1, §2); a recheck is queued by the
//     refresh sweep from the artwork's refusal, never held as a job dated ahead (D231 §3).
//     TestRefresh_ARefreshThatLooksAgainAsksForTheItemsImagesAgain,
//     TestRefresher_ARecheckIsARefreshThatAsksForTheImagesAgainDerivedBySweep.
//   - A film or series is owed its first match until it has an answer: matched, "not found", or
//     a failure of its own; each sweep queues it again whatever became of an earlier job, so one
//     lost with cache.db, cancelled, or parked while its host was down is not lost (D231 §3,
//     replacing D170's "once per item"). The pass reports the same, from the same queries.
//     TestRefresher_AFirstMatchIsOwedUntilTheItemHasAnAnswerHoweverOftenItsJobIsLost.
//   - A fetch that fails through the item's own fault, its last attempt included, is a fact in
//     persistent.db (fetch_failure) with its reason and time: the sweeps pass the item over, the
//     identification list and its page show it, and a person's refresh, "try again", clears it,
//     as a fetch that succeeds does (D231 §4, §10).
//     TestRecordFailure_AnItemsOwnFailureIsShownPassedOverAndClearedByAPersonsRefresh.
//   - A person's refresh is a fact (refresh_request), not a job: the refresh sweep queues it until
//     a fetch that read it succeeds or fails, and whichever fetch of the item runs does what it
//     asked, images included, so a request lost with cache.db or merged into a rolling refresh
//     still happens (D231 §4). TestHandle_APersonsRefreshIsAFactWhicheverFetchRunsDoesAndAnswers.
//   - A refresh fetches the source an item follows again — a series with each season it keeps,
//     an episode through its series — takes the provider's values under a person's overrides, and
//     keeps our copy of an item the provider dropped (D213 §2, §4).
//     TestRefresh_TakesTheProvidersNewValuesAndKeepsAPersonsOverride,
//     TestRefresh_AnItemGoneFromItsProviderKeepsOurCopy,
//     TestRefresh_ASeriesFetchesItsRecordAndEachSeasonItKeeps.
//   - The rolling refresh queues an item once its data is older than its provider's setting,
//     somewhere in the period's last quarter by its id, never with the setting at 0, and never an
//     item with a failure of its own; a person's refresh is queued whenever asked (D213 §3).
//     TestRefresher_SweepQueuesWhatIsDueSpreadOverTheLastQuarterOfThePeriod.
//
// # Read before changing
//
//   - D170: one source per title, fetched once; a refetch is a person's act or, since D213, the
//     rolling refresh.
//   - D172: an ambiguous match is never guessed; D301 settles several of one name by a guess.
//   - D182: metadata is a stack of layers; re-derivation reads only stored input.
//   - D183: an override is an override wherever its value came from.
//   - D209: for shows, the series is what is identified.
//   - D213: a dead image checked once, the rolling refresh, and nothing deleted upstream.
//   - D231 §3, §4: work derived from persistent.db by sweeps; an item's own failure and a person's
//     refresh are facts, not job rows.
//   - D141: NFO lock markers.
//   - docs/contributors/design/plans/identification.md.
//
// # Where to change
//
//   - filename.go: the filename parser, and its corpus in filename_test.go; nfo.go: the NFO
//     reader.
//   - pass.go, load.go, titles.go, episodes.go, extras.go: the identification pass; evidence.go: what a
//     stored match says to it; text.go: how the pass writes a name, and sortkeys.go: keying and
//     folding every name again when the sort locale, the collation or the fold changes;
//     localart.go: the images beside the media; artwork.go: what the pass reports of the images
//     it found, for the artwork service; vocabulary.go: the schema's closed sets.
//   - provider.go: the provider interface; cleaned.go: cleaning every provider answer where it
//     enters. match.go: the matcher and the fetch_metadata job's handler; match_series.go: a series' match
//     and its season listings. series.go: a series in the pass. refresh.go: a person's refresh, the
//     rolling refresh, and the fetch sweeps (first matches, requests, rechecks).
//   - chooser.go: what a person does — list, choose, override. The TMDB client is
//     internal/core/providers/tmdb.
//   - identifytest/: the fake TMDB that this package's tests and the API's share. The tests that
//     drive a library through it are package identify_test (identifytest imports identify);
//     export_test.go names what they read of the package's own.
//   - retire.go: hiding a deleted library's titles with their history (D231 §7);
//     series_return.go: a series found again by a provider id its files give (D208 §2).
package identify
