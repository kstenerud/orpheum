// SPDX-License-Identifier: GPL-3.0-or-later

// Package identifytest gives tests Provider, TMDB behind identify.Provider: films, search
// results, other sources' ids and season listings held in maps, and every call recorded. It is
// imported only by tests.
//
// # Invariants
//
//   - What Fetch and FetchSeason return, Parse and ParseSeason read back unchanged: the stored
//     body is the value as JSON, so a pass that re-derives a title from its stored response sees
//     what was fetched. Review-only; every identify match test relies on it.
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - Imports nothing from internal/node, never os/exec. Enforced by the depguard rule
//     core-boundary.
//
// # Read before changing
//
//   - docs/contributors/standards/testing.md, Network: TMDB is a fake behind the provider
//     interface.
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//   - internal/core/identify/provider.go: what the real provider promises.
//
// # Where to change
//
//   - provider.go: Provider.
package identifytest
