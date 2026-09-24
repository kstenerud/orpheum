// SPDX-License-Identifier: GPL-3.0-or-later

// Package identify matches titles to provider metadata: the filename parser, NFO read, the TMDB
// client, ranked match candidates, the ambiguous-match chooser, and per-field overrides. An
// ambiguous match is never guessed. A fetched response is stored and never refetched except by a
// user's act; re-deriving a title reads only stored input.
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
//   - The TMDB client is the only caller of secrets.Value.Reveal here. Enforced by the forbidigo
//     rule tagged [reveal] (this package is on its path allowlist).
//   - Errors from the provider are wrapped where they enter. Enforced by wrapcheck, which is
//     scoped to this package.
//   - The filename parser is a pure function of a path, tested against a corpus that includes
//     non-Latin names, and no expression uses an ASCII-only class (D49).
//     TestParsePath_ACorpusOfNames, TestParsePath_NoExpressionUsesAnASCIIOnlyClass.
//
// # Read before changing
//
//   - D170: one source per title, fetched once; every refetch is an act of the user.
//   - D172: an ambiguous match is never guessed.
//   - D182: metadata is a stack of layers; re-derivation reads only stored input.
//   - D183: an override is an override wherever its value came from.
//   - D141: NFO lock markers.
//   - docs/contributors/design/plans/identification.md.
//
// # Where to change
//
//   - filename.go: the filename parser, and its corpus in filename_test.go.
//   - The NFO reader; the TMDB client (a subpackage when it grows); candidate ranking; overrides.
package identify
