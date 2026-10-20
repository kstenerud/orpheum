// SPDX-License-Identifier: AGPL-3.0-or-later

// Package turn runs the TURN relay embedded in the binary (pion/turn), which carries Watch
// Together's voice when two browsers cannot reach each other directly, and issues its credentials
// (D68, D193 §4, D243 §8). A credential names one membership and an expiry and its password is
// derived from the sealed TURN secret, so the relay checks it without keeping it, and admits it
// only while the membership lasts. The server makes the secret when it has none it can open.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/instancedir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; the secret is internal/core/secrets'. Enforced by
//     the depguard rule sql-only-in-store and TestArch_OnlyStoreImportsDatabaseSQL.
//   - Derives credentials with secrets.Value.Reveal, the one use permitted by the forbidigo rule
//     tagged [reveal], and never returns the shared secret: a credential carries only a password
//     derived from it (D194). Enforced by TestIssue_GivesADerivedPasswordNeverTheSecret; that no
//     log line carries it is review-only.
//   - The relay admits the credentials it issued, while their membership lasts and until they
//     expire, and nothing else. Enforced by TestRelay_AdmitsAMembershipsCredentialsUntilItEndsOrTheyExpire.
//     The credentials a core sends its discovery name no membership and last until they expire
//     (the discovery protocol's §2.4). Enforced by TestRelay_AdmitsTheDiscoverysCredentialsUntilTheyExpire.
//     They last a quarter of an hour, and an allocation made while they were good is served after
//     (the Plex-chain review's relay credential).
//     TestRelay_KeepsServingAnAllocationTheDiscoverysCredentialsMadeAfterTheyExpire.
//   - The relay serves TCP beside UDP on its port, for a browser that cannot send UDP, and relays
//     to peers on the public internet only: never to the server's own machine or the networks it
//     is on, which would make a membership a way into the house. Enforced by
//     TestRelay_ServesTCPAndRelaysOnlyToThePublicInternet.
//   - Relayed media leaves from the listen address's host, so a relay listening on the tunnel's
//     address answers through the tunnel (D243 §7). Checked on the owner's VPS: VQ19.
//   - The secret is made when there is none, or none this server can open, and is never replaced
//     otherwise. Enforced by TestEnsureSecret_MakesTheSecretOnceAndAgainWhenItCannotBeOpened.
//
// # Read before changing
//
//   - D68: the media server is already the relay; TURN is embedded.
//   - D63: voice is WebRTC.
//   - D193 §4: browser WebRTC through embedded TURN; the ladder is deleted.
//   - D194: the TURN shared secret is sealed; rotating it is free.
//   - D243 §8: port 3478, relay ports 49160–49199, credentials per membership that end with it.
//   - docs/contributors/design/plans/watch-together.md.
//
// # Where to change
//
//   - credentials.go: the secret, the credentials issued from it, and the Issuer that hands a
//     member theirs as ICE servers (GET /together/{id}/ice; TestTogether_EachMemberIsHandedRelayCredentialsOfTheirOwn).
//   - relay.go: the relay's listener and what it admits.
//   - log.go: pion's logs, into the server's.
package turn
