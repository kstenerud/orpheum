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
//   - Opens no file and walks no directory; files are internal/core/datadir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - Every JSON field on the wire is camelCase and every marshalled struct has explicit tags.
//     Enforced by tagliatelle (json: camel) and musttag.
//   - Never branches on the User-Agent. Enforced by the forbidigo rule tagged [client-identity].
//   - Every error, huma's own included, leaves as {"error": {...}} with a code from the
//     enumeration, and never echoes a submitted value. Enforced by the tests' wantError and
//     TestSecrets_AreWriteOnly.
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
//     TestBuildFinding_WarnsWhenANodeRunsAnotherBuildThanTheServer.
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
//     and TestLibraries_AnAdminApprovesWithheldRemovalsAndForgetsAGoneDisk.
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
//     secrets.go, health.go, identification.go, libraries.go, browse.go, doctor.go, system.go;
//     later groups add theirs. server.go holds the readiness gate and the authentication
//     helpers. M1 builds setup, sessions, settings and secrets (D198).
//   - api/openapi.json is generated with `orpheum openapi`; regenerate it after any change here.
//   - The error enumeration lives here and is the only source the SDK's is generated from.
package api
