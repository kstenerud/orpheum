// SPDX-License-Identifier: GPL-3.0-or-later

// Package build says what a binary was built from, as Go recorded it at build time: the release
// tag its commit carries, or "dev" and the commit, and whether the tree had uncommitted changes.
// It is the one source of the version every build reports, however it was built (D225): the
// server's, each node's, the User-Agent's and `orpheum version`'s.
//
// # Invariants
//
//   - A commit with a release tag is that tag; any other is "dev" with its commit; a modified
//     tree is said to be one. TestFromBuildInfo_IsTheTagOrDevAndTheCommitAndWhetherTheTreeWasModified.
//   - This package imports nothing from the module. Enforced by the depguard rule
//     platform-is-a-leaf.
//
// # Read before changing
//
//   - D225: the System page shows each build and catches a node whose build differs.
//
// # Where to change
//
//   - build.go.
package build
