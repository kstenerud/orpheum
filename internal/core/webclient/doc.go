// SPDX-License-Identifier: GPL-3.0-or-later

// Package webclient serves the built web client at `/`: its files as they are, and its own
// addresses (`/films`, `/title/…`) with its index.html, so a reload or a shared link opens the
// right screen (D216 §2). `make build` copies web/apps/web/dist into dist/ before building the
// binary, which embeds it; a build without it serves a page that says the client was not built
// in, and the dev loop serves the client from Vite instead.
//
// # Invariants
//
//   - A path under the API's prefix or /debug/ is never answered with index.html: an unknown API
//     path is a 404, not the client. Enforced by TestHandler_NeverAnswersAnAPIPathWithTheClient.
//   - A file whose name carries its content hash (Vite's assets/) is cached for a year and marked
//     immutable; index.html is revalidated on every load, so a new release is picked up at once.
//     Enforced by TestHandler_HashedAssetsAreImmutableAndIndexIsRevalidated.
//   - The client counts as built only when dist/ has an index.html; the placeholder alone does
//     not. Enforced by TestBuiltIn_IsTheClientOnlyWhenItHasAnIndex.
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
//   - webclient.go: the handler. files.go: the embedded files, and whether they are a client.
package webclient
