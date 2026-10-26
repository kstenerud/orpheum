// SPDX-License-Identifier: AGPL-3.0-or-later

// Package single is a discovery for one core (W4b; D273 §3, D274 §5): the core-facing protocol of
// discovery/discoveryproto answered for the one core it holds, and that core's signalling, through
// which a client's WebRTC offer reaches it. `orpheum discovery` serves it on the owner's VPS. It
// has no accounts, no billing and no names for many cores: those are the closed multi-core
// service's, which assembles the same public building blocks (D275 §1).
//
// # Invariants
//
//   - A realm sign-in wrong in any part, its realm unknown included, answers the same not-found no
//     sooner than a fixed wait after it was asked, a core slow to refuse at the same moment, the wait
//     for the core being no longer; a right one is what a link's locate gives, its offers passed
//     under the person (D297 §2, §3, D312 §17). Enforced by
//     TestRealmSignIn_AnyPartWrongIsTheSameNotFoundAfterTheSameWait,
//     TestRealmSignIn_ACoreSlowToRefuseIsAnsweredWhenAnUnknownRealmIs and
//     TestRealmSignIn_ARightOneIsTheCoresPathsKeyAndTagAndItsOffersGoUnderThePerson.
//
//   - It holds at most one core: a key other than its core's is refused enrolling while that core
//     is claimed or its claim code lives; a pending core whose code expired is replaced, and every
//     session it had ends. Enforced by TestEnrol_HoldsOneCoreAndRefusesAnotherWhileItIsClaimed.
//
//   - A session's token works only for the core it was opened for, by its id and key, checked on
//     every call (the security audit's A3). Enforced by
//     TestEnrol_HoldsOneCoreAndRefusesAnotherWhileItIsClaimed.
//
//   - A pending core publishes no address, gets no certificate record and holds no signalling
//     connection. Enforced by TestHandler_APendingCorePublishesNothingUntilItIsClaimed.
//
//   - One instance of the core is live: another is admitted when the live one holds no signalling
//     connection or does not answer a ping on it, and refused as a copy while it does (D293 §1).
//     Enforced by TestSession_ASecondInstanceTakesThePlaceOfOneThatDoesNotAnswerAndIsRefusedWhileItDoes.
//
//   - A session for a core it does not hold is refused saying to enrol again. Enforced by
//     TestSession_ForACoreItDoesNotHoldSaysToEnrolAgain.
//
//   - A signature counts only over a nonce it issued, once, for the purpose it was made for; issuing
//     one keeps nothing, so no count of strangers' challenges keeps the core from its session. The
//     nonces are discovery/proof's Nonces, which the paid service holds too (D343). Enforced by
//     TestSession_ANonceWorksOnceAndOnlyForItsPurpose and
//     TestSession_NoCountOfStrangersChallengesKeepsTheCoreFromItsSession.
//
//   - A pending core is claimed only by its own code, typed before the code expires; a status asked
//     for after the code expired gives a new one. Enforced by
//     TestClaim_TakesOnlyThePendingCoresOwnCodeInItsTime.
//
//   - Every call but a challenge, an enrolment and a session carries a session's token. Enforced by
//     TestHandler_RefusesASignedCallWithoutASessionsToken.
//
//   - A client's offer is passed to the core over its signalling connection and its answer returned
//     under the offer's id; with no connection, the offer fails at once. Enforced by
//     TestOffer_PassesTheOfferToTheCoreAndReturnsItsAnswer.
//
//   - The core's real client enrols, is claimed and sends its addresses through it. Enforced by
//     TestCore_ACoreEnrolsIsClaimedAndSendsItsAddresses.
//
//   - Imports nothing from internal/. Enforced by TestArch_PublicPackagesImportNothingInternal.
//
//   - A client finds the claimed core only by what the core registered: a link's hash, by its use
//     policy and its life, which gives a bootstrap token of five minutes, never renewed; or a
//     device's locator. Anything else is not_found, without saying why.
//     TestLocate_ALinksHashFindsTheCoresPathsAndABootstrapTokenAndNothingElseDoes,
//     TestLocate_ALinkOfOneUseLocatesOnceAndAnyLinkNoLongerOnceItEnds,
//     TestPaths_ALocatorOrALiveBootstrapTokenFindsTheCoreAndSaysHowItStands.
//
//   - An offer reaches the core with the hash the device found it by and its proof, which the core
//     checks; a request names at most four, and one hash holds at most eight open.
//     TestOffers_ReachTheCoreWithTheHashTheDeviceFoundItByAndComeBackInOrder,
//     TestOffers_ARefusedRequestNeverReachesTheCore, TestOpenOffers_AHashHoldsAtMostEightOpenAtOnce.
//
//   - A dead end is counted for the core's week, keeping only when, and told in the heartbeat's
//     answer. TestDeadEnd_IsCountedForTheCoresWeekAndToldInTheHeartbeatsAnswer.
//
//   - A core is online while it holds its signalling connection, offline since it last closed.
//     TestPaths_ALocatorOrALiveBootstrapTokenFindsTheCoreAndSaysHowItStands.
//
//   - The record is written whole: a new file renamed over it. TestFileStore_AWriteReplacesTheRecordWithAWholeNewFile.
//
// # Read before changing
//
//   - D273 §3, D274 §5, D275 §1: what a single-target discovery is, and why it is public.
//   - docs/contributors/design/plans/discovery-mode.md: its steps; discovery-core.md: the wire.
//
// # Where to change
//
//   - single.go: the protocol's handler, enrolment and claiming; its nonces are discovery/proof's.
//   - session.go: the sessions, bound to the core and its live instance.
//   - record.go: the one core it holds, kept in a file under a lock.
//   - signal.go: the core's signalling connection and the offers passed over it, as Signal, the
//     building block the paid service holds one of for each of its cores (D275 §1).
package single
