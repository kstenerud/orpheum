// SPDX-License-Identifier: GPL-3.0-or-later

// Package tmdb is TMDB behind identify's provider interface: search, find by an IMDb id or a
// series' TVDB id, fetch a film's details with its credits or a series' record and its season
// listings, and parse a stored response; and TMDB's image downloader, an artwork.Fetcher. It is the
// one place the TMDB key is revealed, to TMDB (D194 §5). TMDB's API and its image server are each
// a provider service (APIService, ImagesService), which every call asks first.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Calls go through a client from internal/platform/outbound, which takes no proxy from the
//     environment (D204), and are sent and read by its Exchange. Enforced by the forbidigo rule
//     tagged [outbound].
//   - The key is revealed only here, and never appears in an error: a failed connection's error
//     drops the URL that carries it. Enforced by the forbidigo rule tagged [reveal] and
//     TestGet_AFailedConnectionDoesNotPrintTheKey.
//   - TMDB's refusals become the errors the matcher acts on: 429 is outbound.RateLimitedError,
//     waiting a second when Retry-After names no time, 401 is ErrKeyRefused, 404 is ErrNoMatch,
//     a 502, 503, 504 or a failed connection is outbound.ErrUnavailable (a plain 500 is the
//     request's own failure, D231 §5), and no key is ErrNoKey. Enforced by
//     TestGet_TurnsTMDBsRefusalsIntoTheErrorsTheMatcherActsOn
//     and TestGet_A429WithoutRetryAfterIsARateLimitStill.
//     Its image server's are the same, with 404 as artwork.ErrNoImage, and a 200 that is not an
//     image, such as a captive portal's page, is the host failing (outbound.ErrUnavailable),
//     not the image missing. Enforced by
//     TestImageFetcher_TurnsTheImageServersRefusalsIntoTheErrorsTheAPIsAre,
//     TestImageFetcher_A503IsTheHostDownAndA500TheImagesOwnFailure and
//     TestImageFetcher_AnAnswerThatIsNotAnImageIsTheHostFailing.
//   - Every call asks its host's provider service first, which keeps to TMDB's documented 40
//     requests a second with background work below 80% of it, and sends nothing to a parked host
//     but a person's call, which is its probe (D231 §5, §6).
//     TestGet_AsksTMDBsProviderFirstAndSendsNothingToAParkedHostForTheBackground.
//   - Tests call a fake TMDB on loopback, never the network (standards/testing.md). Enforced by
//     review; every test here builds its server with httptest.
//
// # Read before changing
//
//   - D170: a title follows one source, fetched once; the response is stored.
//   - D172: an ambiguous match is never guessed (the matcher's rule, in identify).
//   - D194 §5: only the provider client reveals a secret.
//   - D231 §5: TMDB's hosts as provider services, and what counts as a host down.
//   - docs/contributors/design/plans/identification.md, Where metadata comes from.
//
// # Where to change
//
//   - client.go: authorisation (a v3 key as api_key, a v4 token as a bearer token), TMDB's
//     refusals, and its provider services with their limits. works.go: the endpoints for films,
//     series and seasons, and the reading of each response. images.go: the image downloader.
package tmdb
