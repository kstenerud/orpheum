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
//
// # Read before changing
//
//   - D85: the API has a compatibility contract covering three protocols.
//   - D154: one grant table; the credential never appears in a URL.
//   - D58: never hand out a URL the client may not use.
//   - D193 §1: the core relays media bytes from the node; it never opens a media file.
//   - D194 §5: secrets are write-only over the API.
//   - docs/contributors/design/plans/cut-a.md §5: every endpoint, and the error model.
//
// # Where to change
//
//   - One file per endpoint group of cut-a §5 (setup and auth, settings, libraries,
//     identification, browse, play state, playback, subtitles, Watch Together, running it).
//   - The error enumeration lives here and is the only source the SDK's is generated from.
package api
