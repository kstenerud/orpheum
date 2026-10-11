// SPDX-License-Identifier: AGPL-3.0-or-later

// Package certificatetest is a fake ACME CA for tests (RFC 8555), as small as
// golang.org/x/crypto/acme's client needs: one account, an order of one name at a time, a dns-01
// challenge it checks against a fake DNS provider, and certificates signed by its own root.
//
// # Invariants
//
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - It answers a challenge only when the fake DNS holds the record RFC 8555 §8.4 makes from the
//     token and the account's key, so a test cannot pass with the wrong record.
//     TestObtain_ProvesTheNameByDNSServesTheCertificateAndKeepsIt shows the right one passing; the
//     wrong one is refused.
//
// # Read before changing
//
//   - RFC 8555: ACME, whose shapes the fake answers in.
//   - docs/contributors/standards/go.md, Testing: hand-written fakes, no mock frameworks.
//
// # Where to change
//
//   - ca.go: the fake, its resources and its checks.
package certificatetest
