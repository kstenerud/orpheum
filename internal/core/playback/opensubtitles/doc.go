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
//     §7); the login is used for its day by every request, and made again when OpenSubtitles no
//     longer accepts it. TestSearch_LogsInOnceAndAsksByTheWorksIDsAndTheFilesHash,
//     TestCurrentSession_IsUsedForItsDayAndMadeAgainWhenOpenSubtitlesNoLongerAcceptsIt.
//   - A refused login is not tried again until the admin changes it: logins are rationed
//     (opensubtitles-api.md §3). TestCurrentSession_ARefusedLoginIsNotTriedAgainUntilItChanges.
//   - A search asks by a film's ids or an episode's series and place, and the file's moviehash,
//     as OpenSubtitles asks for them (D224 §3). TestSearch_AsksForAnEpisodeByItsSeriesAndPlace.
//   - A download says what is left and when the count resets, as OpenSubtitles says; a spent
//     quota says so with that time (D224 §4).
//     TestDownload_FetchesTheFileAndSaysWhatIsLeftAndASpentQuotaWhenItResets.
//   - Only this package reveals the login (D194). Enforced by the forbidigo rule tagged [reveal].
//
// # Read before changing
//
//   - D224: fetching a subtitle on request.
//   - docs/contributors/design/research/opensubtitles-api.md: what the API allows and requires.
//   - D194: secrets.
//
// # Where to change
//
//   - client.go: the login, the key, requests and their errors. requests.go: search, download
//     and the allowance.
package opensubtitles
