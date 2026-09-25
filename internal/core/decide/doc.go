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
//   - A stream the table cannot place is prepared, never guessed playable; each row of the
//     table has a case. Enforced by TestDecide_SortsEachVideoStreamIntoOneMenuEntry and
//     TestDecide_SortsEachAudioStreamIntoOneMenuEntry.
//   - A menu without both baselines, or naming something the menu does not have, is refused
//     (D21). Enforced by TestMenu_RefusesAMenuWithoutTheBaselinesOrWithAnUnknownName.
//   - Direct play is judged on the picture and on the track a player starts with (the default,
//     else the first), never on cover art or a second track. Enforced by
//     TestDecide_JudgesTheTrackAPlayerStartsWith.
//   - Every switch over a codec, container or reason names every value; a new codec breaks each
//     switch that does not handle it. Enforced by exhaustive (default-signifies-exhaustive:
//     false).
//
// # Read before changing
//
//   - D2: the client decides, from a manifest.
//   - D21: a fixed, named output menu, negotiated by a prioritised client list.
//   - D217 §1: the menu's wire form, the container list, and the table that sorts streams.
//   - D22: video, audio and subtitles negotiate as independent axes.
//   - D103: a device capability is never stored.
//   - D193 §2: prepare is the one path besides direct play.
//   - docs/contributors/design/plans/cut-a.md §6.
//
// # Where to change
//
//   - menu.go: the menu's names. classify.go: the table from a file's streams to those names.
//     decide.go: the decision, its reasons and its profile.
package decide
