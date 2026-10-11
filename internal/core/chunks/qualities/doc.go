// SPDX-License-Identifier: AGPL-3.0-or-later

// Package qualities reads the setting chunk_qualities: the lower qualities a slow link is offered,
// each a bit rate and a picture height, highest first (D247 §6). It is its own package so the
// settings registry can check a value with the same function internal/core/chunks reads it with,
// one function for both (AGENTS.md rule 11), where internal/core/chunks itself imports the settings.
//
// # Invariants
//
//   - A value is a comma-separated list of <kbps>k@<height>p, each bit rate within MinKbps..MaxKbps,
//     each height even within MinHeight..MaxHeight, written highest first: each a lower bit rate
//     than the one before, and no taller, so none is there twice; anything else is refused whole. Enforced by
//     TestParse_ReadsEachQualityHighestFirst and TestParse_RefusesAListNotOfThatShape.
//
//   - The settings registry checks chunk_qualities with Parse. Enforced by
//     TestChunkQualities_IsCheckedByTheParserThatReadsIt (internal/core/settings).
//
// # Read before changing
//
//   - D247 §6: a fixed set of standard qualities, a setting the admin can change.
//   - docs/contributors/design/plans/chunks.md §3: the qualities.
//
// # Where to change
//
//   - qualities.go: Quality and Parse.
package qualities
