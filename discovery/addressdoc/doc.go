// SPDX-License-Identifier: AGPL-3.0-or-later

// Package addressdoc is a discovery's address document (D391 §3, D393 §5; the root key audit's §3.1):
// its id, root version and sequence, when it was issued and expires, its name and issuer, where it is
// served, and its core API, client API, loader, client origins, account, homepage and support, in
// the strict line format, signed by a key of the discovery's root chain (discovery/trust). Every
// discovery serves it at DocumentPath under its issuer and each location, beside its root chain;
// a core is given an issuer, follows the chain, opens the document, and trusts no origin it does not
// name. A public building block, so the paid service serves the same document as discovery/single.
//
// # Invariants
//
//   - An issuer is an origin, https or on this machine, and a typed name is its https origin.
//     TestIssuer_ANameTypedIsItsHTTPSOriginAndAnythingButAnOriginIsRefused.
//   - A document is taken only signed by the chain's current set in the address namespace, naming the
//     chain's id and a root version it has reached, for exactly the issuer asked (RFC 8414 §3.3); it
//     reads back as written, and documents are ordered by (root version, sequence).
//     TestOpen_ADocumentIsTheChainsTheAskedIssuersAndNoOtherIsTaken.
//   - Every origin and URL in it is https, or http on this machine, and the client origins' pattern
//     puts a core's id in one whole label. TestCheck_EveryAddressIsHTTPSAndThePatternPutsTheCoreInALabel.
//   - Imports nothing from internal/. Enforced by TestArch_PublicPackagesImportNothingInternal.
//
// # Read before changing
//
//   - D391 §3, D393; docs/contributors/design/research/root-key-audit-2026-10-31.md §3.1, §3.5, §3.8.
//
// # Where to change
//
//   - addressdoc.go: the document, its lines, its check and opening it, and the paths it is served at.
package addressdoc
