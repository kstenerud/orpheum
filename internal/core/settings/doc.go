// SPDX-License-Identifier: GPL-3.0-or-later

// Package settings is the setting registry: server-scope and user-scope values in the setting
// table, each with a declared default that is safe to be wrong and shown as a default, or else a
// question asked in the admin UI. Secrets are not settings; they live in internal/core/secrets.
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
//   - No setting holds a secret. Review-only; the canary test (D194) checks that no settings dump
//     carries one.
//
// # Read before changing
//
//   - D183: a default must be safe to be wrong and visible where it acts.
//   - D184: the defaults audit.
//   - D182: a new config requirement is a default, a derivation, or a question.
//   - D194 §3: why secrets have their own table.
//   - docs/contributors/design/plans/schema.md.
//
// # Where to change
//
//   - The registry of settings and their defaults; GET/PUT /settings and /me/settings logic.
package settings
