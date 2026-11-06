// SPDX-License-Identifier: AGPL-3.0-or-later

// Package discovery is the core's side of the discovery protocol (D273): it enrols with a discovery
// (the paid service, or the owner's own) by its issuer and identity when an admin asks, following
// the discovery's root chain from its genesis and trusting only the address document it signs (D391
// §3, D393; the root key audit's §3), waits to be claimed, keeps a session, sends its addresses as the
// heartbeat, and answers the WebRTC offers the discovery passes on. It follows Orpheum's publisher
// root too, whose release records it reads. suggested.go is the one place Orpheum names its paid
// discovery and its publisher root.
//
// # Invariants
//
//   - Enrolling signs the discovery's nonce with the identity key, bound to the discovery's identity,
//     and the discovery, its trust state (trust-discovery) and the core's id there survive a restart
//     (discovery.json). TestConnect_EnrolsWithASignatureAndKeepsTheDiscoveryAcrossARestart.
//   - A trust state that does not hold is never used (§3.3). TestTrust_AStateThatDoesNotHoldIsNeverUsed.
//   - A discovery not suggested is enrolled only by the identity its admin confirms (§3.8).
//     TestConnect_AnOwnDiscoveryIsEnrolledOnlyByTheIdentityItsAdminConfirms.
//   - A rotation is followed, its floor lifted and a document the removed key signs refused, and the
//     heartbeat names the root version and hash held (F17).
//     TestDocument_TheCoreFollowsARotationAndTheRemovedKeySignsNoMore.
//   - Equivocation, a rollback and a conflict of root statements are refused and said.
//     TestDocument_EquivocationARollbackAndAConflictAreRefusedAndSaid.
//   - A record changing a released digest is refused and said (F11); an automatic install waits a day
//     from first sight (F4; security-updates.md Part C); a build with no publisher root reads none.
//     TestRelease_ARecordChangingAReleasedDigestIsRefusedAndSaid,
//     TestRelease_AnAutomaticInstallWaitsADayFromFirstSight,
//     TestRelease_ABuildWithNoPublisherRootChecksNothingAndSaysSo.
//   - A core refused as a copy of another that serves stands aside: it asks for no session, until an
//     admin asks again (Service.AskAgain) or it restarts; every session names the installation, and a
//     refusal while the last holder is away waits as long as the discovery says (D437 §3, §5).
//     TestStep_ACoreRefusedAsACopyStandsAsideUntilAskedAgainOrRestarted,
//     TestStep_ACoreRefusedWhileTheHolderIsAwayAsksAgainWhenTheDiscoverySays.
//   - Backing off after reaching no discovery, the core looks every reachProbeEvery whether it
//     answers again, and asks at once when it does (D437 §5).
//     TestRun_ACoreWhoseNetworkComesBackAsksAtOnce.
//   - The core holds its signalling connection, and so answers the discovery's ping, only while its
//     health check passes (D437 §2). TestSignal_AnUnhealthyCoreHoldsNoSignallingConnection.
//   - The serving core keeps and logs where and when a copy of it was refused (D437 §4).
//     TestSignal_TheServingCoreSaysACopyWasRefused.
//   - An issuer is an https origin, or http only on this machine; anything else asks nothing.
//     TestConnect_RefusesAnIssuerThatIsNotAnHTTPSOrigin.
//   - The core API, client origin and loader origin are the issuer's signed document's and no
//     other's: a document of another issuer, or one its set did not sign, enrols nothing.
//     TestConnect_TakesItsAddressesFromTheIssuersSignedDocumentAlone.
//   - The suggested issuer is trusted from the genesis this build holds, never one it shows; a build
//     holding none refuses it, asking nothing (D391 §7).
//     TestConnect_TheSuggestedIssuerIsTrustedByItsCompiledGenesisAndNoneFailsLoudly.
//   - Hourly, a newer document that holds, from any place the core has used in the past year, moves
//     the core's addresses, its API included, which it then calls under the same identity; a forged
//     one or none keeps the last good one and says why.
//     TestDocument_ANewerSignedDocumentFromAnyPlaceMovesTheCoreAndAForgedOneDoesNot.
//   - No shipped source but suggested.go names the owner's domains, and the suggestion is an issuer
//     with genesis statements that hold. TestSuggested_IsTheOnlyShippedSourceThatNamesTheOwnersDomains,
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
//   - A release build reads the release record the publisher root signed, at the publisher's places
//     and its discovery's, at its start and daily, an hour after a failure, and keeps what it says of
//     its own version for its admins; a development build reads nothing; with none served it knows
//     nothing, and a failure is never the core's problem (security-updates.md Part A).
//     TestRelease_ACoreAsksAtItsStartAndDailyAboutItsOwnVersion,
//     TestRelease_ADevelopmentBuildAsksNothing,
//     TestRelease_ADiscoveryWithoutTheCallKnowsNothingAndAFailureAsksAgainInAnHour.
//   - A record no key of the publisher root signed, a discovery's no more than a stranger's, is never
//     believed (D393 §2).
//     TestRelease_ARecordNoRootKeySignedIsNotBelieved.
//   - Leaving tells the discovery and forgets it. TestLeave_TellsTheDiscoveryAndForgetsIt.
//   - A claimed core answers each offer the discovery passes on, under its id, and refuses one past
//     rtcapi's limit saying why. TestSignal_AClaimedCoreAnswersTheOffersItsDiscoveryPassesOn.
//   - Every signature to the discovery is bound to its identity and the core, and every answer is
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
//   - A network witnessed again is written to discovery.json only a day or more after the witnessing
//     it holds (houseNetworkKeepEvery), not per connection from home; in memory every witnessing
//     counts at once. TestHouseNetworks_AWitnessingAgainIsWrittenOnlyADayOn.
//   - An offer through the discovery reaches rtcapi only by Answerer.AnswerRelayed, which checks it
//     itself first (the security audit's A2): the interface names no other way. Review, until the
//     owner adds its TestArch_ claim.
//
// # Read before changing
//
//   - D273: why a discovery, the paid service and the owner's own, and the protocol's versioning.
//   - D391: a discovery is a key set and its names are locations; enrolment by issuer.
//   - D393, and the root key audit's §3: the chains, the trust state, and following them.
//   - docs/contributors/design/plans/discovery-core.md: the wire and the phases.
//
// # Where to change
//
//   - discovery.go: connecting and the loop; reach.go: the loop's look whether an unreached discovery
//     answers again (D437 §5); discovery_call.go: sessions and the signed call they
//     carry; renewal.go: one session renewal at a time; signal.go: the signalling connection;
//     document.go: enrolment, the chain followed and the address document looked for hourly;
//     trustfile.go: the trust state kept; release.go: the publisher root and the release record;
//     suggested.go: the suggested discovery; house_networks.go: the house's IPv6 networks (D356).
package discovery
