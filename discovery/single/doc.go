// SPDX-License-Identifier: AGPL-3.0-or-later

// Package single is a discovery for one core (W4b; D273 §3, D274 §5): the core-facing protocol of
// discovery/discoveryproto answered for the one core it holds, and that core's signalling, through
// which a client's WebRTC offer reaches it. `orpheum discovery` serves it on the owner's VPS. It
// has no accounts, no billing and no names for many cores: those are the closed multi-core
// service's, which assembles the same public building blocks (D275 §1).
//
// # Invariants
//
//   - It holds at most one core: a key other than its core's is refused enrolling while that core
//     is claimed. Enforced by TestEnrol_HoldsOneCoreAndRefusesAnotherWhileItIsClaimed.
//   - A signature counts only over a nonce it issued, once, for the purpose it was made for; issuing
//     one keeps nothing, so no count of strangers' challenges keeps the core from its session.
//     Enforced by TestSession_ANonceWorksOnceAndOnlyForItsPurpose and
//     TestSession_NoCountOfStrangersChallengesKeepsTheCoreFromItsSession.
//   - A pending core is claimed only by its own code, typed before the code expires; a status asked
//     for after the code expired gives a new one. Enforced by
//     TestClaim_TakesOnlyThePendingCoresOwnCodeInItsTime.
//   - Every call but a challenge, an enrolment and a session carries a session's token. Enforced by
//     TestHandler_RefusesASignedCallWithoutASessionsToken.
//   - A client's offer is passed to the core over its signalling connection and its answer returned
//     under the offer's id; with no connection, the offer fails at once. Enforced by
//     TestOffer_PassesTheOfferToTheCoreAndReturnsItsAnswer.
//   - The core's real client enrols, is claimed and sends its addresses through it. Enforced by
//     TestCore_ACoreEnrolsIsClaimedAndSendsItsAddresses.
//   - Imports nothing from internal/. Enforced by TestArch_PublicPackagesImportNothingInternal.
//
// # Read before changing
//
//   - D273 §3, D274 §5, D275 §1: what a single-target discovery is, and why it is public.
//   - docs/contributors/design/plans/discovery-mode.md: its steps; discovery-core.md: the wire.
//
// # Where to change
//
//   - single.go: the protocol's handler, its nonces and sessions.
//   - record.go: the one core it holds, kept in a file under a lock.
//   - signal.go: the core's signalling connection and the offers passed over it.
package single
