// SPDX-License-Identifier: AGPL-3.0-or-later

// Package providerstest is what tests of a provider service and its clients share: the health
// the services keep their hosts' states in, as the job queue keeps them, over no database.
//
// # Invariants
//
//   - Test support only: no production package imports it. Enforced by review and
//     go-speculative's scan, which counts no test's call as a production one.
//
// # Read before changing
//
//   - internal/core/providers/doc.go: what a provider service promises.
//
// # Where to change
//
//   - providerstest.go: the health, a job queue in memory.
package providerstest
