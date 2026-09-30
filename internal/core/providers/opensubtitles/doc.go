// SPDX-License-Identifier: GPL-3.0-or-later

// Package opensubtitles is OpenSubtitles' REST API, for fetching a subtitle on request (D224):
// search by the work's ids and the file's moviehash, download one, and say what the account has
// left. It logs in with the server's login, once for the token's day, shared by every request.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec. Enforced by the depguard rule
//     core-boundary and TestArch_CoreNeverImportsNode.
//   - Every request carries Orpheum's key, which ships with the program, and no user's (D224 §2,
//     §7); the login is used for its day by every request, and made again once when OpenSubtitles no
//     longer accepts it, whether it answers 401 or, as for an expired token, 500 "invalid".
//     TestSearch_LogsInOnceAndAsksByTheWorksIDsAndTheFilesHash,
//     TestCurrentSession_IsUsedForItsDayAndMadeAgainWhenOpenSubtitlesNoLongerAcceptsIt,
//     TestCurrentSession_ATokenAnsweredInvalidWithA500IsMadeAgainOnce.
//   - The key, the token and a download are sent only to OpenSubtitles' own hosts: a login's
//     base_url and a download link are taken only on opensubtitles.com or a name under it, or the
//     base's host (go.md §21). TestCurrentSession_ALoginNamingAnotherHostThanOpenSubtitlesKeepsTheBase,
//     TestDownload_ALinkToAnotherHostThanOpenSubtitlesIsNotFetched,
//     TestIsOwnHost_IsOpenSubtitlesDomainOrTheBasesHost.
//   - A refused login is not tried again until the admin changes it: logins are rationed
//     (opensubtitles-api.md §3). TestCurrentSession_ARefusedLoginIsNotTriedAgainUntilItChanges.
//   - A search asks by a film's ids or an episode's series and place, and the file's moviehash,
//     as OpenSubtitles asks for them (D224 §3). TestSearch_AsksForAnEpisodeByItsSeriesAndPlace.
//   - A download is spent once per file while its link works (three hours,
//     opensubtitles-api.md §2): a link that fails as unavailable or rate limited is retried, with
//     back-off, within the Download and by the next Download of the file, and a link refused any
//     other way is forgotten.
//     TestDownload_ALinkThatFailsIsRetriedWhileItWorksAndSpendsNoSecondDownload.
//   - A download says what is left and when the count resets, as OpenSubtitles says; a spent
//     quota says so with that time (D224 §4).
//     TestDownload_FetchesTheFileAndSaysWhatIsLeftAndASpentQuotaWhenItResets.
//   - Every request, the download link's too, is sent and read by outbound.Exchange: a 502, 503, 504 or a
//     failed connection is outbound.ErrUnavailable, a 429 outbound.RateLimitedError, and an
//     error never repeats the URL.
//     TestRequests_AProviderThatFailsOrCannotBeReachedIsUnavailableWhereverItFails,
//     TestSearch_ARateLimitIsTheOneEveryProviderReturns.
//   - The text OpenSubtitles sends that a person is shown, a release's and a file's name and a
//     language, passes through the one display sanitiser as it is read (D211 §2).
//     TestRequests_TextOpenSubtitlesSendsIsCleanedWhereItEnters.
//   - Only this package reveals the login (D194). Enforced by the forbidigo rule tagged [reveal].
//   - Every request asks OpenSubtitles' provider service (Service) first, which keeps to
//     opensubtitles_requests_per_second and to the day's downloads it last said were left, and never
//     lets background work spend the last opensubtitles_downloads_reserved_for_people_percent of the
//     day's allowance its login said (D231 §5); a
//     spent quota is refused before it is asked.
//     TestDownload_BackgroundNeverSpendsTheDownloadsHeldBackForPeopleAndAsksNothing.
//
// # Read before changing
//
//   - D224: fetching a subtitle on request.
//   - docs/contributors/design/research/opensubtitles-api.md: what the API allows and requires.
//   - D194: secrets.
//   - D231 §5: OpenSubtitles as a provider service, with downloads held back for people.
//
// # Where to change
//
//   - client.go: the login, the key, requests and their errors, and the provider service with its
//     limits. requests.go: search, download and the allowance.
package opensubtitles
