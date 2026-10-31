// SPDX-License-Identifier: AGPL-3.0-or-later

// Package addressdoc is a discovery's address document (D391 §3): what its issuer, the origin a
// person types, names as its core API, client API, loader, client origins, account, homepage,
// support and further locations, signed by its root key set or an online key a root key authorised
// (discovery/trust). Every discovery serves it at WellKnownPath; a core is given an issuer once,
// opens the document there, and trusts no origin the document does not name. A public building
// block, so the paid service serves the same document as discovery/single (D275 §1).
//
// # Invariants
//
//   - An issuer is an origin, https or on this machine, and a typed name is its https origin.
//     TestIssuer_ANameTypedIsItsHTTPSOriginAndAnythingButAnOriginIsRefused.
//   - A document is taken only signed by the set and for exactly the issuer asked (RFC 8414 §3.3).
//     TestOpen_ADocumentSignedByTheSetIsTheAskedIssuersAndNoOtherIsTaken.
//   - The root key set's statements served with it bring a core's set up to date, and the set reached
//     is the core's to keep. TestOpen_CatchesUpTheRootSetFromTheStatementsServedWithIt.
//   - Every origin and URL in it is https, or http on this machine, and the client origins' pattern
//     puts a core's id in one whole label. TestCheck_EveryAddressIsHTTPSAndThePatternPutsTheCoreInALabel.
//   - Imports nothing from internal/. Enforced by TestArch_PublicPackagesImportNothingInternal.
//
// # Read before changing
//
//   - D391 §3, §4: the document and the core's enrolment by issuer.
//   - docs/contributors/design/research/domain-assumptions-audit-2026-10-31.md §4: the design.
//
// # Where to change
//
//   - addressdoc.go: the document, its check and opening it.
package addressdoc
