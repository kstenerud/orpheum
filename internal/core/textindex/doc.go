// SPDX-License-Identifier: AGPL-3.0-or-later

// Package textindex is how names are shown, found, ordered and told apart: the one display
// sanitiser, the one normaliser that folds text for search and the identity fold derived from
// it, the CLDR sort keys a library lists in, and the routing of a search to the index that can
// answer it. The identification pass writes with it; browse and search read with it.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - It is the one normaliser (D48): every script's letters survive it, marks go, and the atomic
//     letters no Unicode option decomposes are spelled out. TestFold_OneNormaliserForEveryScript.
//   - It is the one identity fold (D232): SameWork is Fold keeping only letters, numbers and
//     spacing marks, so names that differ in punctuation, spacing, case, accents or width are one
//     work. TestSameWork_NamesThatDifferOnlyInWhatFilesAndProvidersWriteTheirOwnWayAreOne.
//   - It is the one display sanitiser (D211 §2): nothing it passes can reorder a display (bidi
//     embeddings, overrides, isolates), hide in it (invisible characters, controls) or spill
//     over it (combining marks past four on one letter, unbounded length); real scripts' marks,
//     the bidi marks and the joiners survive. TestClean_NothingShownCanLieOrSpill.
//   - Sort keys follow the locale, and numbers sort as numbers (D47).
//     TestSorter_FollowsTheLocaleAndNumbers.
//   - A name's sort key leaves out its leading article, from a list per locale language; a name
//     that is only an article stays whole (D402). TestSorter_FilesANameUnderItsWordsNotItsLeadingArticle.
//   - A search goes to the index that can answer it: a folded substring under three characters, the word index
//     for Latin, the trigram index for any other script; a person's text is never an FTS5
//     operator (D48). TestPlanQuery_RoutesByLengthAndScript.
//
// # Read before changing
//
//   - D47: sort order is a materialised CLDR sort key, per item per locale.
//   - D48: two FTS5 tokenisers, routed by script and length, and exactly one normaliser.
//   - D49: Unicode discipline at the boundaries.
//   - D211: text hygiene.
//   - D232: the identity fold.
//   - docs/contributors/design/research/internationalisation.md.
//
// # Where to change
//
//   - clean.go: the display sanitiser. fold.go: the search normaliser and the identity fold. sortkey.go: the sort keys.
//     query.go: the search routing.
package textindex
