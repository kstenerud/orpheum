// SPDX-License-Identifier: AGPL-3.0-or-later

// Package discovery is the core's side of the discovery protocol (D273): it enrols with a discovery
// (the paid service, or the owner's own) when an admin asks, waits to be claimed, keeps a session,
// sends its addresses as the heartbeat, and answers the WebRTC offers the discovery passes on.
//
// # Invariants
//
//   - Enrolling signs the discovery's nonce with the identity key, and the discovery and the core's
//     id there survive a restart (discovery.json). TestConnect_EnrolsWithASignatureAndKeepsTheDiscoveryAcrossARestart.
//   - A pending core asks whether it is claimed; a claimed one sends its addresses when they change
//     and every five minutes otherwise, each time with a new nonce for the outside probe.
//     TestStep_WaitsToBeClaimedThenSendsItsAddressesWhenTheyChangeOrEveryFiveMinutes.
//   - The signalling connection is pinged every 25-30 s, and closed and dialled again once two pings
//     in a row go unanswered; one that answers is kept (the protocol's §2.5).
//     TestSignal_AConnectionThatAnswersNoPingIsDialledAgain, TestSignal_AConnectionThatAnswersItsPingsIsKept.
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
//   - Leaving tells the discovery and forgets it. TestLeave_TellsTheDiscoveryAndForgetsIt.
//   - A discovery is reached over HTTPS, or plain HTTP only on this machine.
//     TestCheckBaseURL_RefusesAnythingButHTTPSOrThisMachine.
//   - A claimed core answers each offer the discovery passes on, under its id, and refuses one past
//     rtcapi's limit saying why. TestSignal_AClaimedCoreAnswersTheOffersItsDiscoveryPassesOn.
//   - Every signature to the discovery is bound to its base URL and the core, and every answer is
//     signed over its exact SDP, its offer and the offer's fingerprint (the protocol's §2.1, §2.5).
//     TestStep_WaitsToBeClaimedThenSendsItsAddressesWhenTheyChangeOrEveryFiveMinutes (the fake
//     discovery checks the binding) and TestSignal_AClaimedCoreAnswersTheOffersItsDiscoveryPassesOn.
//   - The client origin the discovery names is kept only as an https origin (D293 §4).
//     TestStep_KeepsTheClientOriginTheDiscoveryNamesWhenItIsAnHTTPSOrigin.
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
//   - docs/contributors/design/plans/discovery-core.md: the wire and the phases.
//
// # Where to change
//
//   - discovery.go: connecting, the loop, sessions and calls; renewal.go: one session renewal at a
//     time; signal.go: the signalling connection; urls.go: which discovery addresses and client
//     origins the core takes; house_networks.go: the house's IPv6 networks (D356).
package discovery
