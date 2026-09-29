// SPDX-License-Identifier: GPL-3.0-or-later

// Package tmdb is TMDB behind identify's provider interface: search, find by an IMDb id or a
// series' TVDB id, fetch a film's details with its credits or a series' record and its season
// listings, and parse a stored response; and TMDB's image downloader, an artwork.Fetcher. It is the
// one place the TMDB key is revealed, to TMDB (D194 §5).
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Calls go through a client from internal/platform/outbound, which takes no proxy from the
//     environment (D204). Enforced by the forbidigo rule tagged [outbound].
//   - The key is revealed only here, and never appears in an error: a failed connection's error
//     drops the URL that carries it. Enforced by the forbidigo rule tagged [reveal] and
//     TestGet_AFailedConnectionDoesNotPrintTheKey.
//   - TMDB's refusals become the errors the matcher acts on: 429 is outbound.RateLimitedError,
//     waiting a second when Retry-After names no time, 401 is ErrKeyRefused, 404 is ErrNoMatch,
//     and no key is ErrNoKey. Enforced by TestGet_TurnsTMDBsRefusalsIntoTheErrorsTheMatcherActsOn
//     and TestGet_A429WithoutRetryAfterIsARateLimitStill.
//     Its image server's are the same, with 404 as artwork.ErrNoImage. Enforced by
//     TestImageFetcher_TurnsTheImageServersRefusalsIntoTheErrorsTheAPIsAre.
//   - Tests call a fake TMDB on loopback, never the network (standards/testing.md). Enforced by
//     review; every test here builds its server with httptest.
//
// # Read before changing
//
//   - D170: a title follows one source, fetched once; the response is stored.
//   - D172: an ambiguous match is never guessed (the matcher's rule, in identify).
//   - D194 §5: only the provider client reveals a secret.
//   - docs/contributors/design/plans/identification.md, Where metadata comes from.
//
// # Where to change
//
//   - client.go: authorisation (a v3 key as api_key, a v4 token as a bearer token) and TMDB's
//     refusals. works.go: the endpoints for films, series and seasons, and the reading of each
//     response. images.go: the image downloader.
package tmdb
