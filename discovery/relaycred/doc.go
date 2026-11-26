// SPDX-License-Identifier: AGPL-3.0-or-later

// Package relaycred is the hosted relay's credential (D558; the hosted relay's plan, §1): a TURN
// username `<expiry>:<role>:<household>:<kbps>` and its password, the standard base64, padded, of
// HMAC-SHA1 under a shared secret over the username's bytes (the TURN REST form), in the realm
// `orpheum-relay`. The paid discovery makes them and the relay (internal/relay) checks them, so the
// format is written once, here. Public, so the closed discovery imports it (D275 §1).
//
// # Invariants
//
//   - A credential made is read back as it was; a username that is not exactly the form, with a
//     role, a household of 8 to 32 of [A-Za-z0-9_-] and a rate of 1 to 1,000,000 kbit/s, written
//     canonically, is refused, and none is made. Enforced by
//     TestParse_ReadsBackWhatMakeWroteAndRefusesAnythingElse.
//   - The password is the vector's: base64 of HMAC-SHA1 over the username. Enforced by
//     TestPassword_IsTheTURNRESTFormsVector.
//   - A username changed in any part, or a password under another secret, does not match.
//     Enforced by TestMatches_RefusesAChangedUsernameOrAnotherSecret.
//   - A secret is at least 32 bytes, and nothing that prints, logs or serialises one shows it;
//     no method returns it. Enforced by TestSecret_IsLongEnoughAndNeverPrints.
//   - Imports nothing from internal/ and no TURN library. Enforced by
//     TestArch_PublicPackagesImportNothingInternal; the TURN library, review-only.
//
// # Read before changing
//
//   - D558; docs/contributors/design/plans/hosted-relay.md §1: the wire contract.
//   - D275 §1: why this package is public.
//
// # Where to change
//
//   - relaycred.go: the username, the password and the secret.
package relaycred
