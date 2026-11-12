// SPDX-License-Identifier: AGPL-3.0-or-later

// Package webclient serves the built web client at `/`: its files as they are, and its own
// addresses (`/films`, `/titles/…`) with its index.html, so a reload or a shared link opens the
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
//   - A text file of a kilobyte or more (scripts, styles, WebAssembly, SVG) is sent gzipped to a
//     browser whose Accept-Encoding takes gzip, varying by it, and as it is to one that does not;
//     images and fonts, compressed already, as they are (D129). Enforced by
//     TestHandler_SendsTextGzippedToABrowserThatAcceptsIt.
//   - The client counts as built only when dist/ has an index.html; the placeholder alone does
//     not. Enforced by TestBuiltClient_IsTheClientOnlyWhenItHasAnIndex.
//   - Every answer carries `X-Content-Type-Options: nosniff`, a content-security policy that loads
//     nothing from elsewhere but Google's Cast SDK where the next invariants allow it and forbids
//     framing, and `Referrer-Policy: no-referrer`; one over HTTPS
//     carries `Strict-Transport-Security`. Enforced by TestHandler_ServesFilesAndFallsBackToIndex
//     and TestHandler_EveryAnswerCarriesTheSecurityHeaders.
//   - The client's pages, not the cast pages, report every string reaching a script sink without a
//     Trusted Types policy (`Content-Security-Policy-Report-Only: require-trusted-types-for
//     'script'`), which the page notes to the core's log (web/apps/web/src/policy-violations.ts);
//     the enforced policy holds no Trusted Types yet. Enforced by
//     TestHandler_TheClientsPagesReportWhatTrustedTypesWouldRefuse.
//   - Only the client's screens and the cast receiver load from elsewhere, and only Google's Cast SDK
//     (www.gstatic.com): every screen of the client its sender's, for the Cast menu's Chromecast to
//     load in Chrome at its first opening (D465 §3), since the client moves between screens in place
//     and a page keeps the policy it was loaded with, and the lab's cast probe at /lab/cast with them;
//     the receiver under /cast/ CAF's and the Cast platform's socket on the device
//     (ws://localhost:8008). The client's files and the lab's other probes load nothing from
//     elsewhere, and only the receiver reaches the device's socket. Enforced by
//     TestHandler_OnlyTheScreensAndTheReceiverMayLoadGooglesCastSDK.
//   - The cast receiver, where it is built in (dist/cast/, which web/apps/cast-receiver's build
//     writes inside the web client's dist), answers its addresses under /cast/ with its own page.
//     Enforced by TestHandler_ServesTheCastReceiverUnderCast.
//   - The client's manifest names every file it serves by its size and SHA-256, in path order, so
//     the same client is always the same bytes, which the core signs for a discovery's loader to
//     check (D308 §6). Enforced by TestManifestOf_NamesEveryFileByItsHashInPathOrder.
//
// # Read before changing
//
//   - D216 §2: the binary serves the built client from M3.
//   - D129: gzip on the client channel, negotiated by Accept-Encoding.
//   - docs/contributors/design/plans/cut-a.md §2.
//
// # Where to change
//
//   - webclient.go: the handler. gzip.go: what is sent gzipped, and when. files.go: the embedded
//     files, and whether they are a client.
//     manifest.go: the manifest of its files.
package webclient
