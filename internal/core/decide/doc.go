// SPDX-License-Identifier: GPL-3.0-or-later

// Package decide is the playback decision: given a file's streams, the device's prioritised menu
// per axis and whether the link is local or remote, it answers direct play, an existing prepared
// rendition, or a new prepare job, with the transcode reason. It is a pure function of declared
// capabilities.
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
//   - Cannot see the HTTP request, so it cannot branch on client identity. Enforced by the
//     depguard rule decide-sees-capabilities (no net/http, no internal/core/api).
//   - Every switch over a codec, container or reason names every value; a new codec breaks each
//     switch that does not handle it. Enforced by exhaustive (default-signifies-exhaustive:
//     false).
//
// # Read before changing
//
//   - D2: the client decides, from a manifest.
//   - D21: a fixed, named output menu, negotiated by a prioritised client list.
//   - D22: video, audio and subtitles negotiate as independent axes.
//   - D103: a device capability is never stored.
//   - D193 §2: prepare is the one path besides direct play.
//   - docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - The per-axis resolution of cut-a §6 and the profile table; the transcode reason enumeration.
package decide
