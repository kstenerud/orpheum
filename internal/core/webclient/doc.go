// SPDX-License-Identifier: GPL-3.0-or-later

// Package webclient serves the built web client at `/`: its files as they are, and its own
// addresses (`/films`, `/title/…`) with its index.html, so a reload or a shared link opens the
// right screen (D216 §2). A release build embeds web/apps/web/dist under the build tag
// `webclient`; any other build serves a page that says the client was not built in, and the dev
// loop serves the client from Vite instead.
//
// # Invariants
//
//   - A path under the API's prefix or /debug/ is never answered with index.html: an unknown API
//     path is a 404, not the client. Enforced by TestHandler_NeverAnswersAnAPIPathWithTheClient.
//   - A file whose name carries its content hash (Vite's assets/) is cached for a year and marked
//     immutable; index.html is revalidated on every load, so a new release is picked up at once.
//     Enforced by TestHandler_HashedAssetsAreImmutableAndIndexIsRevalidated.
//   - Every answer carries `X-Content-Type-Options: nosniff`. Enforced by
//     TestHandler_ServesFilesAndFallsBackToIndex.
//
// # Read before changing
//
//   - D216 §2: the binary serves the built client from M3.
//   - docs/contributors/design/plans/cut-a.md §2.
//
// # Where to change
//
//   - webclient.go: the handler. embedded.go (tag webclient) and none.go: where the files come from.
package webclient
