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
//     and every five minutes otherwise. TestStep_WaitsToBeClaimedThenSendsItsAddressesWhenTheyChangeOrEveryFiveMinutes.
//   - A session the discovery refuses is replaced by a new one, signed afresh.
//     TestStep_OpensANewSessionWhenTheDiscoveryRefusesTheOld.
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
//
// # Read before changing
//
//   - D273: why a discovery, the paid service and the owner's own, and the protocol's versioning.
//   - docs/contributors/design/plans/discovery-core.md: the wire and the phases.
//
// # Where to change
//
//   - discovery.go: connecting, the loop, sessions and calls; signal.go: the signalling connection.
package discovery
