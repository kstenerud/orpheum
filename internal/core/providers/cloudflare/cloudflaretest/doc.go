// SPDX-License-Identifier: AGPL-3.0-or-later

// Package cloudflaretest is a fake of Cloudflare's DNS API for tests: the zones a token reaches, and
// the TXT records set in them.
//
// # Invariants
//
//   - Only tests import it. Enforced by TestArch_TestSupportIsImportedOnlyByTests.
//   - It refuses any token but Token, as Cloudflare refuses one it does not know.
//     TestClient_SaysWhyItCannotSetARecord.
//
// # Read before changing
//
//   - D243 §6: the record a DNS-01 challenge needs, set through Cloudflare.
//   - docs/contributors/standards/go.md, Testing: hand-written fakes, no mock frameworks.
//
// # Where to change
//
//   - fake.go: the fake's zones and records, and the three calls it answers.
package cloudflaretest
