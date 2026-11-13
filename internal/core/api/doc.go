// SPDX-License-Identifier: AGPL-3.0-or-later

// Package api is the client API: the huma handlers under /api/v1, from which the OpenAPI document
// (api/openapi.json) is generated, committed and diffed. It authenticates, validates and
// translates; the decisions belong to the packages it calls. It also relays media bytes from the
// node to the client for direct play and chunks.
//
// Every error leaves in the one shape of cut-a §5.2, with a code from its enumeration, which a later
// server may extend (D432 §2). Credentials travel in a header or a cookie, never in a URL (D154).
//
// # Invariants
//
//   - A page on the client origin the core's discovery names may call the API and open the session
//     socket, by header alone, and so may a Cast receiver at its loader's origin, which may not read
//     the core's client (D467); no other origin may, nor "null", and no answer allows credentials
//     (the discovery protocol's §3.4; discovery-core.md K10). Enforced by
//     TestCrossOrigin_LetsTheDiscoverysClientCallTheAPIByHeaderAlone,
//     TestCrossOrigin_TheLoadersOriginCallsTheAPIAsACastReceiverThere,
//     TestCrossOrigin_TheClientOriginReadsTheCoresClientAndTheLoadersDoesNot and
//     TestCrossOrigin_TheSessionSocketOpensFromTheDiscoverysClientAlone.
//   - The plain-HTTP listener answers only the names a household reaches it by (its addresses,
//     localhost, names public DNS never answers, the configured names and the core's names at its
//     discovery) and refuses any other with 421, against DNS rebinding (S15). Enforced by
//     TestLANHandler_AnswersTheHousesNamesAndRefusesAnyOther.
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - A POST carrying an Idempotency-Key is done once and its answer kept for repeats, at most
//     idempotentAnswersAtMost answers, idempotentAnswersSignedOutAtMost of them for requests no one
//     signed in made; past a bound it is done without keeping its answer (D244 §4). Enforced by
//     TestIdempotentAnswers_AreBounded and
//     TestIdempotentAnswers_PastTheirTimeAreSweptOut.
//   - Opens no file and walks no directory; files are internal/core/instancedir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - A direct read whose node has no file at the place read, or not the one named, goes on to the
//     next place its holder reaches, which the reads after it go to first; with none left the file
//     is no longer there (D363 §8). TestPlayback_ADirectReadOfAFileGoneFromItsPlaceReadsItsCopy.
//   - Imports no database/sql or SQLite driver; it calls internal/store. Enforced by the depguard
//     rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - Every JSON field on the wire is camelCase and every marshalled struct has explicit tags.
//     Enforced by tagliatelle (json: camel) and musttag.
//   - Never branches on the User-Agent. Enforced by the forbidigo rule tagged [client-identity].
//   - Being at home, or on the port only the server's own machine reaches, is a condition on a
//     signed-in session, never one: every function asking isFromHome takes an auth.Principal or
//     authenticates first (the log's pathOf alone excepted). Enforced by
//     TestArch_HomeIsNeverACredential.
//   - Every handler is the Server method its operation's OperationID names, so the API document
//     and the code say one name. Enforced by TestArch_APIHandlersAreNamedForTheirOperations.
//   - Every operation declares who may call it where it is registered (callableBy, access.go): its
//     capability as x-orpheum-capability (signedIn, admin, accountSecurity, editMetadata,
//     hostTogether, deviceTests, member, media, or none for a secret in the request itself) and
//     its OpenAPI security, of the schemes session, membership and media; the Authorization
//     header is read for the handler and
//     documented by the security alone (D425 §6). Each handler checks its caller as its operation
//     declares, and one declaring none names the secret it checks itself. Enforced by
//     TestArch_EveryHandlerChecksTheCallersItsOperationDeclares.
//   - Every error, huma's own included, leaves as {"error": {...}} with a code from the
//     enumeration, and never echoes a submitted value. Enforced by the tests' wantError and
//     TestSecrets_AreWriteOnly.
//   - Responses are open and requests closed: no schema only a response uses refuses a field it
//     does not name, its kind enumerations and the error codes are marked x-extensible-enum, and
//     the document says a client ignores what it does not know; a request body still refuses an
//     unknown field (D432 §2; openResponses). Enforced by
//     TestDocument_ResponsesAreOpenAndRequestsClosed.
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
//     (under /bytes/, D216) also take the media cookie, HttpOnly and SameSite Strict, which the API
//     never does (D154). Enforced by TestSession_TheTokenWorksInTheHeaderOnlyUntilSignOut and
//     TestMediaCookie_SignInSetsItOnlyTheBytePathsTakeItAndSignOutClearsIt.
//   - A byte path takes a cookie only on a load from the core's own pages, and over TLS only by its
//     host-only name, so no other core, all being one site, can use or set one (the D308 audit's
//     F-6). Enforced by TestMediaCookie_ALoadFromAnotherSitesPageIsNotSignedInByIt and
//     TestMediaCookie_OverTLSItIsHostOnlyAndThePlainNameNoLongerCounts.
//   - An admin administers only from the house's network: at home a sign-in from any origin carries
//     the whole role, directly or over a WebRTC connection whose selected pair is direct and reaches
//     the core from the house, which rtcapi alone judges (rtcapi.IsDirectlyFromHome, D439); from any
//     other network, over WebRTC too, it carries a person's (D297 §6, D300 §3, revising D294 §1, §4),
//     unless the admin set administering from away to always, from home (D302).
//     TestAdmin_AdministersFromAwayOnlyByTheirOwnSettingSetFromHome. Enforced by
//     TestSignIn_AnAdminAdministersFromAnyOrigin and
//     TestAdmin_AdministersOnlyFromTheHousesNetworkDirectlyOrOverWebRTCFromIt. A punched, hairpinned
//     or relayed pair is never from home (D439 §1, D354 §4). Nor is a house network the discovery is
//     told, which only places the LAN name among its paths (D356 §7).
//     TestMarkingHome_AHouseNetworkGrantsNoAdmin. The house is what internal/core/home's rule holds,
//     the one rtcapi judges a pair by: any private address but the tunnel's and the core's own
//     interfaces' (Docker's bridge), and the core's own public IPv6 /64, never a typed /24 alone, so a
//     new lease locks no admin out (D452 §1, §2). TestMarkingHome_ADirectRequestIsHomeByTheSharedRule.
//   - The System page is told the house's IPv6 networks, how each was had, and for a learnt one by
//     which device and when (D356 §10). TestDiscoveryStatusBody_CarriesTheHouseNetworks.
//   - Doctor warns while devices reach the server through the provider and back, naming the UDP port
//     and the house's address a device at home could not reach (D354 §2).
//     TestHairpinFinding_WarnsThatDevicesAtHomeReachTheServerThroughTheProvider.
//   - A page's choice of its path to the core, with its reasons and its WebRTC pairs as the browser sees
//     them, is logged in one bounded line from a signed-in page (D354 §5).
//     TestClientPaths_APagesChoiceOfPathIsLoggedWithItsReasonsAndItsPairs.
//   - Only an admin rescans: every library of films or series, one, or one film or series; the
//     Libraries page shows an admin each library's rescan as it runs (D357 §1, §5).
//     TestRescan_IsForAnAdminOnly. Each rescan's request with full set is its full rescan, every file
//     in its scope read again first (D392 §1, §6). TestRescan_AFullRescanReadsEveryFileOfItsLibraryAgain.
//   - Only an admin sees doctor's findings; its artwork finding warns at high water with the ceiling to set
//     (D212 §7). TestDoctor_AnAdminSeesTheArtworkCacheAgainstItsCeiling.
//   - Only an admin reads and keeps the house's network settings, a wrong one refused naming its
//     setting, kept ones pending until the next start, and a port that would not bind at the start
//     said to have fallen back; only an admin restarts a server a compose file started (D462 §2,
//     §10.11). TestNetwork_AnAdminKeepsTheSettingsForTheNextStartAndIsToldWhatIsWrong,
//     TestNetwork_APortThatWouldNotBindAtTheStartIsSaidToHaveFallenBack,
//     TestNetwork_AnAdminRestartsAServerItsComposeFileStarted.
//   - Only an admin's Check now has the server read the release record at once, one under a minute
//     after the last refused as too soon; a compose file pinning a version is said (D462 §8, §10.7).
//     TestUpdate_AnAdminChecksNowAndIsToldWhatTheServerFound,
//     TestUpdate_SaysWhenTheComposeFilePinsAVersion.
//   - Only an admin sees /system, the System page: the server's facts, doctor's findings and each
//     node's, with the encoder trials each node made (D225).
//     TestSystem_ShowsTheServersFactsEachNodesAndDoctorsFindings. Doctor warns when a node has a
//     GPU and no hardware encoder works on it. TestHardwareFinding_WarnsWhenANodeHasAGPUButEncodesOnTheProcessor.
//     Doctor warns when a node runs another build than the server's (D225 §5).
//     TestNodeBuildFinding_WarnsWhenANodeRunsAnotherBuildThanTheServer. Doctor warns when a node's
//     FFmpeg reads films by their paths, without the dead-share protection (D330).
//     TestFilmReadingFinding_WarnsOfANodeWhoseFFmpegReadsFilmsByTheirPaths.
//   - The System page lists what is down, each with why, since when and its next check, and each
//     outside host's provider: its rate, its background share, its quota and its credentials
//     (D231 §5, §12). Each node says whether it is connected, silent or awaiting its hello
//     (D231 §11). TestSystem_ListsWhatIsDownAndEachOutsideHostsProvider. It lists each root
//     and disk a scan found missing with since when (D41, D231 §7).
//     TestSystem_SaysSinceWhenEachRootAndDiskHasBeenOffline.
//   - An admin is told how many folders and disks are offline, wait for them or could not be read,
//     which the status tray's warning sign shows (D280). TestNeedsAttention_CountsTheFoldersAndDisksOffline.
//   - A job waiting on a destination that is down says which, as a person reads it, why, since
//     when, and when it is looked at next, if a check brings it back (D231 §12).
//     TestJobs_AJobWaitingOnADestinationThatIsDownSaysWhatWhySinceWhenAndTheNextCheck.
//   - Why a subtitle could not be listed or fetched is said in OpenSubtitles' terms: no login, a
//     refused login, or a spent quota with its reset time (D224).
//     TestSubtitleCandidates_SayWhyInTheProvidersTerms.
//   - A provider that is not answering (a refused connection, a timeout, a 502, 503 or 504) is 503
//     providerError with detail.reason unavailable and failedHop provider, whichever provider,
//     never internal (cut-a §5.2). TestSubtitleCandidates_SayWhyInTheProvidersTerms,
//     TestIdentification_AMissingOrRefusedKeySaysWhyWithTheProviderAsTheFailingLeg.
//   - A provider's rate limit leaves the same way whichever provider it is and whatever asked it:
//     rate_limited, naming the provider, with detail.retryAfterMs and failedHop provider (cut-a
//     §5.2). TestIdentification_AProvidersRateLimitSaysWhichAndHowLongAsSubtitlesDo.
//   - A person with editMetadata may refresh a title or series whenever they like (D213 §2).
//     TestIdentification_APersonMayRefreshATitleWheneverTheyLike.
//   - /health answers before the database is ready, and the rest answers "migrating" until then.
//     Enforced by TestHealth_AnswersBeforeTheDatabaseIsReady.
//   - Secrets are write-only. Enforced by TestSecrets_AreWriteOnly.
//   - /config shows every setting beside its starting value, filters to the changed ones, seeds
//     and resets, and only an admin changes it (D200). Enforced by
//     TestConfig_ShowsEverySettingAndTheFileAndResets, TestConfig_ListsAndDeletesOnlyUnusedRows,
//     TestConfig_FiltersToWhatDiffersFromTheStartingValue
//     and TestSettings_OnlyAnAdminChangesTheServersAndABadValueChangesNothing.
//   - Any signed-in user sees which titles and series are ambiguous; only a role with editMetadata sees
//     candidates, chooses a match or sets an override. Enforced by
//     TestIdentification_OnlyARoleThatEditsMetadataMayChange.
//   - An admin creates a library only over roots its node configured, lists roots and filings,
//     asks for scans and pages the scanner's view; a user sees only the libraries. Enforced by
//     TestLibraries_AnAdminCreatesOneOverTheNodesRoots,
//     TestLibraries_AUserSeesTheLibrariesButNotTheirMachinery, TestFiles_PagesWhatTheScannerSaw
//     and TestLibraries_AnAdminApprovesWithheldRemovalsAndForgetsAGoneDisk. An admin retires a
//     root or a node, or deletes a library, and its work is cancelled (D231 §7); a retired folder's
//     or deleted library's files are left to no library and no item is deleted (D363 §10).
//     TestScenario_S22_ARetiredRootsFilesAreLeftToNoLibraryItsWorkIsCancelledAndItsHistoryStays,
//     TestScenario_S23_ARetiredNodesRootsAreRetiredAndItsParkedWorkCancelled,
//     TestScenario_S24_ADeletedLibrarysWorkIsCancelledAndItsTitlesListedByNoLibraryWithTheirHistory.
//     The doctor names each file found at more than one place and warns of an item whose places' NFO
//     files disagree (D363 §8, §9).
//     TestDoctor_NamesAFileAtTwoPlacesAndWarnsWhenTheirNFOFilesDisagree. An admin,
//     and no one else, browses the folders inside a node's configured roots, makes a library over
//     one and adds one to a library, which is scanned (D359).
//     TestLibraries_AnAdminBrowsesTheFoldersInsideANodesRoots,
//     TestLibraries_AnAdminAddsAFolderInsideANodesRootToALibrary.
//   - Any signed-in user browses films and series, a title's detail, a series and search; a
//     file's path is shown to an admin only. Enforced by TestBrowse_ListsDetailsSeriesAndSearch.
//     A list of one library is of one the user reaches, and a search finds only what they reach
//     (D349). Enforced by TestBrowse_OneLibrarysListAndALibraryNotReached.
//   - Home's rows, Continue Watching and the newest (GET /items?home=true), draw on every library
//     the person reaches less those they left out of Home, apart from those left out of Films and
//     Series; an item two libraries reach is on Home while either is left in (D366 §5). Enforced
//     by TestHome_ALibraryLeftOutOfHomeAddsNothingToItsRows.
//   - A chosen match is confirmed, and an override is set and removed, through the identify
//     package (D172, D183). Enforced by TestIdentification_APersonChoosesAnAmbiguousTitlesMatch and
//     TestIdentification_OverridesAreSetAndRemoved.
//   - What a lab page measured on a device is taken from a signed-in page only, at most
//     labResultBytesAtMost bytes, and logged at info as one line under the name it answers with, the
//     result whole inside it, for the device tests to be read from the log (plans/tv-device-tests.md).
//     Enforced by TestLab_AResultIsLoggedAsOneLineNamedByItsIDAndIsBounded. While the admin has the
//     device tests off (shouldOfferDeviceTests, its start), it is refused as notFound, and
//     /client-settings says whether they are offered. Enforced by
//     TestLab_ResultsAreRefusedAndTheTestsNotOfferedUntilTheAdminTurnsThemOn.
//   - An AirPlay grant (GET /bytes/media/g/{grant}/..., D473) is made only for a playback the
//     caller's session holds (POST /airplay/grants) and serves that playback's film alone, with no
//     credential, its playlists' URIs relative so the grant carries on; it signs no one in, and is
//     refused on every other path. Stop (DELETE /airplay/grants) revokes the session's grants and
//     says how many. It lasts a day, ends with the sign-in that made it, and is bound to the
//     connection's address that first presents it, never a header's: from another it is answered
//     as a grant unknown. These are the one exception to D154 §6: no other operation callable with
//     no credential reads a path or query parameter but the reachability echo's nonce. A master or
//     chunk not made yet is held for airPlayHold under a grant, as AVPlayer does not honour a 425.
//     Each request under a grant, a refused one's too, is logged at debug with its file, outcome,
//     status, bytes and times, a receiver gone before its answer as gone with no status, and the
//     grant, a bearer credential, is in no log line, an error's path included (loggingAirPlay).
//     Enforced by TestAirPlay_AGrantInThePathServesItsPlaybackToAReceiverWithNoCredential,
//     TestAirPlay_AGrantIsForItsSessionsPlaybackAloneAndSignsNoOneIn,
//     TestAirPlay_StopRevokesTheGrant, TestAirPlay_AGrantEndsWithTheSignInThatMadeIt,
//     TestAirPlay_AGrantIsBoundToTheConnectionsAddressAndNoHeaders,
//     TestAirPlay_OnlyTheMediaUnderGTakesACredentialInThePath,
//     TestAirPlay_AMasterWithNoGridIsHeldForTheReceiver,
//     TestAirPlay_AChunkNotMadeYetIsHeldUntilItIsMade, TestAirPlay_AReceiverGoneWhileHeldIsLoggedAsGone,
//     TestAirPlay_AReceiverGoneBeforeItsAnswerIsNoInternalError and
//     TestAirPlay_EachRequestUnderAGrantIsLoggedWithoutTheGrant.
//   - The throughput test's body (GET /lab/throughput) is the 1 to 256 MiB asked for, of bytes no
//     gzip shrinks, never cached, behind the same sign-in and setting, each one logged as sent with
//     the path it came over; a throughput test's figures are logged as "a throughput test ran"
//     (webrtc-throughput-2026-10-28.md). Enforced by
//     TestLab_TheThroughputTestsBodyIsTheSizeAskedForAndLoggedAsSent and
//     TestLab_AThroughputTestsFiguresAreLoggedUnderTheirOwnWords.
//   - The button test's events (POST /lab/input-events) are logged at info a line each, flat, by
//     the run's name the page made, with a line for the run's start; a request carries 1 to 200
//     events, each field named and bounded (a kind, a button, a controller's layout and mapping and
//     a full screen's target from their lists, a key's key and code at most 40 characters, numbers
//     in their ranges), so a page writes no attribute of its own and no long value into the log; its
//     text field's events carry the value's length, never its text; behind the same sign-in, setting
//     and body bound as a result (requireDeviceTests). Enforced by
//     TestLab_TheButtonTestsEventsAreLoggedALineEachAndBounded,
//     TestLab_TheButtonTestsFieldFullScreenAndControllerEventsAreLoggedFlat and
//     TestLab_TheButtonTestsEventsAreRefusedUntilTheAdminTurnsTheTestsOn.
//   - The presence socket is the session socket's address with a live hello (live_socket.go): a
//     signed-in page's device is listed to its person while it is open and gone after the silence
//     limit, and a hello of no sign-in is closed for policy (D337 §2). Enforced by
//     TestLive_ADeviceIsListedWhileItsSocketIsOpenAndGoneAfterTheSilenceLimit.
//   - A device casts only to another of its person's devices connected now; the receiver is handed
//     the cast on its live socket and leads the cast's session of one, the sender commands it, and
//     either may end it, the other told why (D337). A device receiving a cast is no device's target
//     until it ends, refused as receiving_a_cast (D386 §1), and a cast back to a device casting to
//     the asker is refused as casting_here. Enforced by
//     TestCast_ADeviceCastsOnlyToAnotherOfItsPersonsConnectedNow,
//     TestCast_ADeviceReceivingIsNoOnesTargetUntilItTakesControlBack and
//     TestCast_AReceiversCastBackToItsSenderIsRefused.
//   - Every operation checks the caller's reach, is an admin's, or holds no library content
//     (D243 §4). TestReach_EveryOperationChecksReachIsAnAdminsOrHoldsNoLibraryContent.
//   - Every answer names its request's ID, the core's own, in X-Request-ID (RequestIDHeader), and every
//     line the request causes, logged with its context, carries it as requestId (D347). Enforced by
//     TestRequestID_EveryAnswerNamesItAndTheLinesItCausedCarryIt.
//   - The answers of compressedOperations alone (lists, titles, search, the admin's views) are sent
//     gzipped to a client that accepts it, varying by Accept-Encoding; none of them holds a
//     credential, so no answer carrying a token, a session's, a device's, a pairing's or a link's
//     is ever compressed beside text a request chose (BREACH, D342).
//     TestCompress_AListIsGzippedToAClientThatAcceptsIt, TestCompress_ASignInIsNeverGzipped and
//     TestArch_NoCompressedOperationAnswersWithACredential.
//   - Every operation answers a wrong credential, in its header, its media or viewer cookie or its
//     viewer header, no better than none: a token no one was given, one off a live one, a signed-out,
//     lapsed or removed person's session, or a device secret.
//     TestCredentials_EveryOperationAnswersAWrongCredentialNoBetterThanNone.
//   - The session socket acts on no message whose v is not together.ProtocolVersion: it is
//     answered rejected, versionMismatch (together.VersionRefusal), and a hello of another version
//     attaches the device to nothing (cut-a §5.1). The core's version is the web client's.
//     TestTogether_AHelloOfAnotherVersionIsRefusedAndAttachesNothing,
//     TestTogether_AMessageOfAnotherVersionIsRefusedAndActedOnNot and
//     TestTogether_TheCoreAndTheClientSpeakVersion3.
//   - A guest receiver's session (D465 §2) is refused at every route requireSignedIn guards; only
//     its cast's routes (accept, end, playback of its cast's films, the byte paths, reports and the
//     sockets) take it, through requireSignedInOrGuest, and the grant that made it is in no log.
//     TestGuestReceiver_ReachesItsCastAlone,
//     TestGuestReceiver_EndsItsOwnCastAloneAsAReceiverDoes and
//     TestGuestReceiver_IsSignedInByAGrantTradedOnceAndCastToByItsSenderAlone.
//   - Cast to this device (D469 §1, §4) signs the device that showed a code its person scanned in as
//     a guest receiver the answering device alone casts to; GET /me and its session's body say it is
//     one and nothing of the person.
//     TestGuestReceiver_ByCodeIsCastToByTheDeviceThatAnsweredTheCodeAlone.
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
//   - reach.go: sign-in for a request about one file or item, which the caller must reach.
//     roles.go: the roles and a person's role (admin).
//   - One file per endpoint group of cut-a §5: session.go (setup and auth), settings.go,
//     config.go, secrets.go, health.go, identification.go, libraries.go, browse.go, item.go (one
//     item of any kind, D416 §2), art.go, doctor.go, system.go, playback.go, subtitle.go,
//     subtitle_fetch.go, preferences.go, report.go, home.go, watched.go (marking by hand, D370),
//     accounts.go, devices.go and jobs.go; lab.go, the lab pages' results and the button test's
//     events.
//   - access.go: who may call an operation, its capability and security schemes (callableBy, D425 §6).
//   - server.go holds the readiness gate and the authentication helpers: requireSignedIn,
//     requireAdmin and requireAccountManager; requireMetadataEditor is in identification.go,
//     requireTogetherHost in together.go, and requireSignedInForMedia, the one sign-in of every
//     byte path, in media_cookie.go, with the byte paths' Cache-Control values.
//   - One refusal leaves with one code and one message: a package's errors are mapped by that
//     package's one mapper, which every handler calling it answers through: authError
//     (server.go), playbackError (playback.go), libraryError (libraries.go), identifyError
//     (identification.go), settingsError (settings.go) and secretError (secrets.go); a
//     provider's rate limit by providerRateLimited (errors_provider.go). A new refusal is a case
//     there.
//   - api/openapi.json is generated with `orpheum openapi`; regenerate it after any change here.
//   - The error codes and hops live in errors.go, the one list of them. The OpenAPI document
//     types code and failedHop as plain strings, so the SDK reads them as strings; it has no
//     generated copy of the list.
//   - idempotency.go: a POST carrying an Idempotency-Key, done once (D244 §4); backups.go:
//     listing and making backups (D242 §5); client_settings.go: the settings every client reads
//     (D237); destinations.go: a destination that is down, as what waits on it says (D231 §12);
//     needs_attention.go: what needs the admin, counted for the status tray (D280);
//     compress.go: which answers are sent gzipped (D342); request_id.go: each request's ID (D347).
//   - The tests are package api_test, so their harness (api_test.go's newHarness) builds the API's
//     dependencies by internal/core/assembly, as the server does, with fakes of the node, its bytes
//     and OpenSubtitles and TMDB (D351 §4); export_test.go names what they read of the package's
//     internals. A test of the internals alone, needing no harness, stays in package api.
package api
