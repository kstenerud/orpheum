// SPDX-License-Identifier: AGPL-3.0-or-later

// Package clocktest provides Fake, the clock.Clock that tests control: its time moves only when a
// test moves it, and its timers fire when it moves past them. It is imported only by tests;
// production code uses clock.System.
//
// # Invariants
//
//   - Fake satisfies clock.Clock; the compile-time assertion in fake.go fails the build if the
//     interface gains a method Fake lacks.
//   - This package imports nothing from the module except internal/platform/clock. Enforced by
//     the depguard rule platform-is-a-leaf.
//
// # Read before changing
//
//   - D190: every check first shows it can fail; a fake that cannot express the failing case
//     is not a fake.
//   - docs/contributors/standards/go.md, Testing: hand-written fakes, no mock frameworks.
//   - docs/contributors/standards/testing.md.
//
// # Where to change
//
//   - fake.go: add behaviour here when clock.Clock gains a method.
package clocktest
