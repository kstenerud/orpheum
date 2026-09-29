// SPDX-License-Identifier: GPL-3.0-or-later

// Package settingstest gives other packages' tests a settings service over a real database, with
// every starting value written and argon2id's costs lowered so a unit test hashes in
// microseconds. It is imported only by tests.
//
// # Invariants
//
//   - It seeds through settings.Service.Seed, the same path a starting server takes, and changes
//     values only through Set, so a test sees what production sees. Review-only.
//   - Imports nothing from internal/node, never os/exec. Enforced by the depguard rule
//     core-boundary, and transitively by TestArch_CoreNeverImportsNode and TestArch_CoreNeverExecs.
//
// # Read before changing
//
//   - D200: every opinionated value is a setting row, with no default in code.
//   - D194 §2: the production argon2id costs, measured by auth's benchmark instead.
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//
// # Where to change
//
//   - settingstest.go: Seeded, and the values it lowers; Set, which changes one setting.
package settingstest
