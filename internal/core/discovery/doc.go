// SPDX-License-Identifier: AGPL-3.0-or-later

// Package discovery is the core's side of the discovery protocol (D273): it enrols with a discovery
// (the paid service, or the owner's own) by its issuer when an admin asks, trusting the address
// document the issuer serves, signed by the discovery's root key set (D391 §3), waits to be claimed,
// keeps a session, sends its addresses as the heartbeat, and answers the WebRTC offers the discovery
// passes on. suggested.go is the one place Orpheum names its paid discovery.
//
// # Invariants
//
//   - Enrolling signs the discovery's nonce with the identity key, and the discovery, the root key set
//     pinned, its address document and the core's id there survive a restart (discovery.json).
//     TestConnect_EnrolsWithASignatureAndKeepsTheDiscoveryAcrossARestart.
//   - An issuer is an https origin, or http only on this machine; anything else asks nothing.
//     TestConnect_RefusesAnIssuerThatIsNotAnHTTPSOrigin.
//   - The core API, client origin and loader origin are the issuer's signed document's and no
//     other's: a document of another issuer, or one its set did not sign, enrols nothing.
//     TestConnect_TakesItsAddressesFromTheIssuersSignedDocumentAlone.
//   - The suggested issuer is trusted by the root keys this build names for it, never by the set it
//     shows; a build naming none refuses it, asking nothing (D391 §7).
//     TestConnect_TheSuggestedIssuerIsTrustedByItsCompiledKeysAndNoneFilledInFailsLoudly.
//   - Hourly, a newer document that holds, from the issuer or any location its document names, moves
//     the core's addresses, its API included, which it then calls under the same issuer; a forged one
//     or none keeps the last good one and says why.
//     TestDocument_ANewerSignedDocumentFromAnyLocationMovesTheCoreAndAForgedOneDoesNot.
//   - No shipped source but suggested.go names the owner's domains, and the suggestion is an issuer
//     with keys that parse. TestSuggested_IsTheOnlyShippedSourceThatNamesTheOwnersDomains,
//     TestSuggested_IsAnIssuerAndItsRootKeysAreKeys.
//   - A pending core asks whether it is claimed; a claimed one sends its addresses when they change
//     and every five minutes otherwise, each time with a new nonce for the outside probe.
//     TestStep_WaitsToBeClaimedThenSendsItsAddressesWhenTheyChangeOrEveryFiveMinutes.
//   - The signalling connection is pinged every 25-30 s, each ping given pongWait to be answered and
//     a missed one followed at once by the next, and closed and dialled again once two in a row go
//     unanswered; one that answers is kept (the protocol's §2.5). A discovery restarted behind a link
//     left open is so found again, whatever order the two restart in, and the log says the connection
//     is open again, how long it was lost and after how many dials.
//     TestSignal_AConnectionThatAnswersNoPingIsDialledAgain, TestSignal_AConnectionThatAnswersItsPingsIsKept,
//     TestSignal_ADiscoveryRestartedBehindALinkLeftOpenIsFoundAgainAndAnswered.
//   - A session the discovery refuses is replaced at once by a new one, signed afresh, and the
//     call made again with it (C11). TestStep_OpensANewSessionWhenTheDiscoveryRefusesTheOld.
//   - One session is opened at a time, however many callers ask at once (C11).
//     TestRenewal_CallersAskingAtOnceShareOneRenewal. A session's life is the duration the discovery
//     gives, counted on the core's own clock. TestSession_LastsAsLongAsTheDiscoverySaysCountedOnTheCoresOwnClock.
//   - A failed call waits longer each time in a row, doubling from the first to the most, jittered
//     so two cores differ, and a success starts over (the protocol's §2; C7).
//     TestStep_AFailedCallWaitsLongerEachTimeInARowAndASuccessStartsOver,
//     TestRetryWait_TwoCoresWaitDifferentlyAndNeverPastTheMost. A dropped signalling connection is
//     dialled again after the first wait. TestSignal_ADroppedConnectionIsDialledAgainWithinAboutASecond.
//   - Two Connects at once enrol once (C12). TestConnect_TwoAtOnceEnrolOnce.
//   - A device code the paid service issued is claimed only while the discovery publishes the core,
//     and the outcome sent opens with the device's key alone (D293 §2).
//     TestDeviceCode_IsClaimedOnlyOnceTheDiscoveryPublishesTheCore,
//     TestDeviceCode_ItsOutcomeOpensWithTheDevicesKeyAndARefusalSendsNone.
//   - A claimed core tells its discovery every link and device locator it may find the core by, as
//     their hashes, the full sets, when they change and under each new session (discovery-core.md
//     K7). TestStep_AClaimedCoreTellsItsDiscoveryItsLinksWhenTheyChangeAndUnderEachNewSession.
//   - Each run of the core names an instance of its own; refused as a copy of another, it says so
//     and asks again every few minutes (D293 §1). TestStep_ACoreRefusedAsACopySaysSoAndAsksAgainLater.
//   - A core its discovery no longer holds enrols again and shows the new code.
//     TestStep_EnrolsAgainWhenTheDiscoveryNoLongerHoldsTheCore.
//   - The core's zone, once the discovery claims it, is kept, and the claim is told once, so the
//     certificate of its names can be asked for. TestStep_SaysOnceWhenTheCoreIsClaimedAndKeepsItsZone.
//   - A claimed core asks how it stands with each heartbeat, and stops sending its addresses once
//     its subscription ends. TestStep_AClaimedCoreLearnsWhenItsSubscriptionEnds.
//   - The certificate of the core's names is proven through the discovery, which sets and clears
//     its DNS-01 record. TestSetTXT_ACertificateForTheCoresNamesIsObtainedThroughTheDiscovery.
//   - A discovery's deprecation date is kept for the admin to see.
//     TestCall_KeepsTheDeprecationTheDiscoveryNames.
//   - A release build reads its discovery's signed release record, at its issuer and its document's
//     locations, at its start and daily, an hour after a failure, and keeps what it says of its own
//     version for its admins; a development build reads nothing; a discovery serving none knows
//     nothing, and a failure is never the core's problem (security-updates.md Part A).
//     TestRelease_ACoreAsksAtItsStartAndDailyAboutItsOwnVersion,
//     TestRelease_ADevelopmentBuildAsksNothing,
//     TestRelease_ADiscoveryWithoutTheCallKnowsNothingAndAFailureAsksAgainInAnHour.
//   - A record no root key of the pinned set signed is never believed (D391 §6).
//     TestRelease_ARecordNoRootKeySignedIsNotBelieved.
//   - Leaving tells the discovery and forgets it. TestLeave_TellsTheDiscoveryAndForgetsIt.
//   - A claimed core answers each offer the discovery passes on, under its id, and refuses one past
//     rtcapi's limit saying why. TestSignal_AClaimedCoreAnswersTheOffersItsDiscoveryPassesOn.
//   - Every signature to the discovery is bound to its issuer and the core, and every answer is
//     signed over its exact SDP, its offer and the offer's fingerprint (the protocol's §2.1, §2.5).
//     TestStep_WaitsToBeClaimedThenSendsItsAddressesWhenTheyChangeOrEveryFiveMinutes (the fake
//     discovery checks the binding) and TestSignal_AClaimedCoreAnswersTheOffersItsDiscoveryPassesOn.
//   - An offer the discovery passes on reaches rtcapi only when its proof holds under the key the
//     core keeps for the hash it came under (A9). TestSignal_AnOfferWhoseProofDoesNotHoldIsRefusedBeforeRTCSeesIt.
//   - An offer whose connection proves itself from home (rtcapi.ProvenAtHome) teaches the network the
//     discovery saw it come from, a public IPv6 /64, as a house network, by the device that witnessed
//     it and when; one from no such network teaches nothing; the log says whether the device's own
//     IPv6 address in its offer agrees, and nothing acts on it (D356 §1, §2, Q5).
//     TestHouseNetworks_ADirectConnectionFromTheHouseTeachesItsOffersNetwork,
//     TestHouseNetworks_AnOfferFromNoIPv6NetworkTeachesNothing.
//   - The heartbeat carries the house networks, the learnt ones the most recently witnessed first, then
//     those the admin named, at most four; a network newly learnt sends one at once (D356 §3, §6).
//     TestHeartbeat_CarriesTheHouseNetworksMostRecentFirst, TestHouseNetworks_ANewNetworkSendsAHeartbeatAtOnce.
//   - A learnt network is kept HouseNetworkKeptFor after it was last witnessed, a fifth pushes out the
//     least recently witnessed, and they are kept in discovery.json, which a Leave removes (D356 §4).
//     TestHouseNetworks_ANetworkUnwitnessedForThirtyDaysIsDropped,
//     TestHouseNetworks_AFifthPushesOutTheLeastRecent, TestHouseNetworks_SurviveARestartAndGoWithALeave.
//   - An offer through the discovery reaches rtcapi only by Answerer.AnswerRelayed, which checks it
//     itself first (the security audit's A2): the interface names no other way. Review, until the
//     owner adds its TestArch_ claim.
//
// # Read before changing
//
//   - D273: why a discovery, the paid service and the owner's own, and the protocol's versioning.
//   - D391: a discovery is a key set and its names are locations; enrolment by issuer.
//   - docs/contributors/design/plans/discovery-core.md: the wire and the phases.
//
// # Where to change
//
//   - discovery.go: connecting and the loop; discovery_call.go: sessions and the signed call they
//     carry; renewal.go: one session renewal at a time; signal.go: the signalling connection;
//     document.go: the issuer's address document, opened at enrolment and looked for again hourly;
//     release.go: the signed release record;
//     suggested.go: the suggested discovery; house_networks.go: the house's IPv6 networks (D356).
package discovery
