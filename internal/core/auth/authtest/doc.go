// SPDX-License-Identifier: AGPL-3.0-or-later

// Package authtest gives tests of auth and of the packages that use it SetupCodes, the state
// directory's setup-code file held in memory. It is imported only by tests.
//
// # Invariants
//
//   - SetupCodes satisfies auth.SetupCodes, and its Read answers auth.ErrNoSetupCode when no
//     code is stored, as the real file does; the compile-time assertion in setupcodes.go fails
//     the build if the interface changes. Review-only for the error.
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - auth's own tests that use it are in the external package auth_test (setup_test.go):
//     an in-package test importing it would be an import cycle.
//
// # Read before changing
//
//   - D223 §2: the setup code and where it is kept.
//   - docs/contributors/standards/go.md, Testing: shared fakes live in a <package>test package.
//
// # Where to change
//
//   - setupcodes.go: SetupCodes.
package authtest
