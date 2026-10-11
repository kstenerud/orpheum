// SPDX-License-Identifier: AGPL-3.0-or-later

// Package loudness says the gains that level each file's sounds (D226), from the measurements a
// read-through keeps by the file's payload hash (internal/core/readthrough, which owns the job that
// measures them). The arithmetic of a gain is internal/core/decide's (levels.go); a playback reads
// the levels through Service.Levels.
//
// # Invariants
//
//   - A sound plays at the level its measurement gives, and one not measured, or that FFmpeg could
//     not measure, plays as it is. Enforced by TestLoudness_AFileIsMeasuredOnceAndPlaysAtTheLevel
//     and TestLoudness_APreparedFilmIsLevelledAsItsStereoMixAndItsCopyCarriesTheGain
//     (internal/core/api), and end to end by TestPlayback_AFilmIsMeasuredOnItsNodeAndPlaysLevelled
//     (cmd/orpheum).
//
//   - It only reads the loudness table; internal/core/readthrough writes it. Enforced by
//     TestArch_EachTableHasOneWritingPackage.
//
//   - Imports nothing from internal/node, never os/exec, no database/sql: the depguard rules
//     core-boundary and sql-only-in-store, TestArch_CoreNeverImportsNode and
//     TestArch_OnlyStoreImportsDatabaseSQL.
//
// # Read before changing
//
//   - D226: loudness is measured by the node and levelled by the player; the gain's rule.
//
// # Where to change
//
//   - loudness.go: the levels a file's measurements give, and the settings they aim by.
package loudness
