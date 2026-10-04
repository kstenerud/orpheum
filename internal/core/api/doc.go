// SPDX-License-Identifier: GPL-3.0-or-later

// Package api is the client API: the huma handlers under /api/v1, from which the OpenAPI document
// (api/openapi.json) is generated, committed and diffed. It authenticates, validates and
// translates; the decisions belong to the packages it calls. It also relays media bytes from the
// node to the client for direct play and prepared renditions.
//
// Every error leaves in the one shape of cut-a §5.2, with a code from its closed enumeration.
// Credentials travel in a header or a cookie, never in a URL (D154).
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
//   - Every JSON field on the wire is camelCase and every marshalled struct has explicit tags.
//     Enforced by tagliatelle (json: camel) and musttag.
//   - Never branches on the User-Agent. Enforced by the forbidigo rule tagged [client-identity].
//   - Every handler is the Server method its operation's OperationID names, so the API document
//     and the code say one name. Enforced by TestArch_APIHandlersAreNamedForTheirOperations.
//   - Every error, huma's own included, leaves as {"error": {...}} with a code from the
//     enumeration, and never echoes a submitted value. Enforced by the tests' wantError and
//     TestSecrets_AreWriteOnly.
//   - Every error the API answers with, huma's own included, is logged at debug with its operation,
//     method, path, status, code and message, never the submitted value, since a client shows only
//     words chosen by the code. Enforced by TestErrorAnswers_AreLoggedWithTheirOperationAndCode.
//   - An error's detail is typed by its code, and the document gives every code its form, so a
//     client that narrows by code reads typed detail (cut-a §5.2). An error takes its code from its
//     detail's type (newDetailedError). Enforced by
//     TestProblemBody_TheDocumentNamesEveryCodeOnceAndTypesItsDetail and
//     TestInvalidRequest_NamesEachProblemByWhereItIsInTheDetail.
//   - Every invalid_request names where each problem is: a field (body.rootPaths, query.cursor,
//     body.valuesByKey.<key>) or body for the request as a whole, so a form names the field in its
//     own words (D188 §1). A handler builds one only with invalidRequestAt. Enforced by
//     TestArch_EveryInvalidRequestSaysWhere and by the tests' wantError, which fails an
//     invalid_request without a location.
//   - A credential is read from the Authorization header only, never the URL; the byte paths
//     (under /bytes/, D216) also take the media cookie, HttpOnly and SameSite Strict, which the API never does
//     (D154). Enforced by TestSession_TheTokenWorksInTheHeaderOnlyUntilSignOut and
//     TestMediaCookie_SignInSetsItOnlyTheBytePathsTakeItAndSignOutClearsIt.
//   - Only an admin sees /doctor; its artwork finding warns at high water with the ceiling to set
//     (D212 §7). TestDoctor_AnAdminSeesTheArtworkCacheAgainstItsCeiling.
//   - Only an admin sees /system, the System page: the server's facts, doctor's findings and each
//     node's, with the encoder each recent prepare used (D225).
//     TestSystem_ShowsTheServersFactsEachNodesAndDoctorsFindings. Doctor warns when a node has a
//     GPU and no hardware encoder works on it. TestHardwareFinding_WarnsWhenANodeHasAGPUButEncodesOnTheProcessor.
//     Doctor warns when a node runs another build than the server's (D225 §5).
//     TestNodeBuildFinding_WarnsWhenANodeRunsAnotherBuildThanTheServer.
//   - The System page lists what is down, each with why, since when and its next check, and each
//     outside host's provider: its rate, its background share, its quota and its credentials
//     (D231 §5, §12). Each node says whether it is connected, silent or awaiting its hello
//     (D231 §11). TestSystem_ListsWhatIsDownAndEachOutsideHostsProvider. It lists each root
//     and disk a scan found missing with since when (D41, D231 §7).
//     TestSystem_SaysSinceWhenEachRootAndDiskHasBeenOffline.
//   - An admin is reminded of what is offline, how many folders and disks since when, until they
//     dismiss it; it comes back after offline_reminder_interval_days while anything is still
//     offline (D231 §7, P39). TestOfflineReminder_ShowsWhatIsOfflineUntilDismissedThenReturnsAfterItsInterval.
//   - A job waiting on a destination that is down says which, as a person reads it, why, since
//     when, and when it is looked at next, if a check brings it back (D231 §12).
//     TestJobs_AJobWaitingOnADestinationThatIsDownSaysWhatWhySinceWhenAndTheNextCheck.
//   - Why a subtitle could not be listed or fetched is said in OpenSubtitles' terms: no login, a
//     refused login, or a spent quota with its reset time (D224).
//     TestSubtitleCandidates_SayWhyInTheProvidersTerms.
//   - A provider that is not answering (a refused connection, a timeout, a 502, 503 or 504) is 503
//     provider_error with detail.reason unavailable and failedHop provider, whichever provider,
//     never internal (cut-a §5.2). TestSubtitleCandidates_SayWhyInTheProvidersTerms,
//     TestIdentification_AMissingOrRefusedKeySaysWhyWithTheProviderAsTheFailingLeg.
//   - A provider's rate limit leaves the same way whichever provider it is and whatever asked it:
//     rate_limited, naming the provider, with detail.retryAfterMs and failedHop provider (cut-a
//     §5.2). TestIdentification_AProvidersRateLimitSaysWhichAndHowLongAsSubtitlesDo.
//   - A person with edit_metadata may refresh a title or series whenever they like (D213 §2).
//     TestIdentification_APersonMayRefreshATitleWheneverTheyLike.
//   - /health answers before the database is ready, and the rest answers "migrating" until then.
//     Enforced by TestHealth_AnswersBeforeTheDatabaseIsReady.
//   - Secrets are write-only. Enforced by TestSecrets_AreWriteOnly.
//   - /config shows every setting beside its starting value, filters to the changed ones, seeds
//     and resets; /settings shows only what the settings page exposes (D200). Enforced by
//     TestConfig_ShowsEverySettingAndTheFileAndResets, TestConfig_ListsAndDeletesOnlyUnusedRows,
//     TestConfig_FiltersToWhatDiffersFromTheStartingValue
//     and TestSettings_OnlyAnAdminChangesTheServersAndABadValueChangesNothing.
//   - Any signed-in user sees which titles and series are ambiguous; only a role with edit_metadata sees
//     candidates, chooses a match or sets an override. Enforced by
//     TestIdentification_OnlyARoleThatEditsMetadataMayChange.
//   - An admin creates a library only over roots its node configured, lists roots and filings,
//     asks for scans and pages the scanner's view; a user sees only the libraries. Enforced by
//     TestLibraries_AnAdminCreatesOneOverTheNodesRoots,
//     TestLibraries_AUserSeesTheLibrariesButNotTheirMachinery, TestFiles_PagesWhatTheScannerSaw
//     and TestLibraries_AnAdminApprovesWithheldRemovalsAndForgetsAGoneDisk. An admin retires a
//     root or a node, or deletes a library, and its work is cancelled (D231 §7).
//     TestScenario_S22_ARetiredRootsFilesAreTombstonesItsWorkIsCancelledAndItsHistoryStays,
//     TestScenario_S23_ARetiredNodesRootsAreRetiredAndItsParkedWorkCancelled,
//     TestScenario_S24_ADeletedLibrarysWorkIsCancelledAndItsTitlesHiddenWithTheirHistory.
//   - Any signed-in user browses films and series, a title's detail, a series and search; a
//     file's path is shown to an admin only. Enforced by TestBrowse_ListsDetailsSeriesAndSearch.
//   - A chosen match is confirmed, and an override is set and removed, through the identify
//     package (D172, D183). Enforced by TestIdentification_APersonChoosesAnAmbiguousTitlesMatch and
//     TestIdentification_OverridesAreSetAndRemoved.
//
// # Read before changing
//
//   - D85: the API has a compatibility contract covering three protocols.
//   - D154: one grant table; the credential never appears in a URL.
//   - D58: never hand out a URL the client may not use.
//   - D193 §1: the core relays media bytes from the node; it never opens a media file.
//   - D194 §5: secrets are write-only over the API.
//   - D172: an ambiguous match is never guessed; choosing is permanent.
//   - docs/contributors/design/plans/cut-a.md §5: every endpoint, and the error model.
//
// # Where to change
//
//   - One file per endpoint group of cut-a §5: session.go (setup and auth), settings.go,
//     config.go, secrets.go, health.go, identification.go, libraries.go, browse.go, art.go,
//     doctor.go, system.go, playback.go, subtitle.go, subtitle_fetch.go, preferences.go, report.go,
//     home.go, accounts.go, devices.go and jobs.go.
//   - server.go holds the readiness gate and the authentication helpers: requireSignedIn and
//     requireAdmin; requireMetadataEditor is in identification.go, and requireSignedInForMedia,
//     the one sign-in of every byte path, in media_cookie.go, with the byte paths' Cache-Control
//     values.
//   - One refusal leaves with one code and one message: a package's errors are mapped by that
//     package's one mapper, which every handler calling it answers through: authError
//     (server.go), playbackError (playback.go), libraryError (libraries.go), identifyError
//     (identification.go), settingsError (settings.go) and secretError (secrets.go); a
//     provider's rate limit by providerRateLimited (errors.go). A new refusal is a case there.
//   - api/openapi.json is generated with `orpheum openapi`; regenerate it after any change here.
//   - The error codes and hops live in errors.go, the one list of them. The OpenAPI document
//     types code and failedHop as plain strings, so the SDK reads them as strings; it has no
//     generated copy of the list.
//   - idempotency.go: a POST carrying an Idempotency-Key, done once (D244 §4); backups.go:
//     listing and making backups (D242 §5); client_settings.go: the settings every client reads
//     (D237); destinations.go: a destination that is down, as what waits on it says (D231 §12);
//     offline_reminder.go: the admin's offline banner (D231 §7, P39).
package api
