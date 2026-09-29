// SPDX-License-Identifier: GPL-3.0-or-later

// Package langtag normalises a language code to BCP 47, the one form the library stores (D24,
// D206): a container's tag, which the node reads, and a subtitle file's name, which the core
// reads, both come through here, so both say "de" for "ger" and "deu" alike.
//
// # Invariants
//
//   - ISO 639-1, ISO 639-2's bibliographic and terminological codes, and BCP 47 tags all become
//     BCP 47; unknown, undetermined and empty become empty. TestBCP47_NormalisesEveryFormItMeets.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D24: language is a preference axis.
//   - D206: a language tag the scanner reads is normalised.
//
// # Where to change
//
//   - langtag.go: the normalisation, and Undetermined, the tag written where one must be.
package langtag
