// SPDX-License-Identifier: AGPL-3.0-or-later

// Package rtcapi carries the core's HTTP API over WebRTC data channels (D273 §8): the direct path to
// a core behind carrier NAT whose client lacks IPv6. It answers a client's offer, and hands every
// request a channel carries to the core's own handler, so the API, chunks, images and subtitles have
// one implementation whichever way a request arrives.
//
// # Invariants
//
//   - One request per channel: a text head (method, path, headers), the body in binary messages,
//     an empty message ending it; the response the same way back. The handler sees the method, the
//     path and query, the headers and the body as sent, and the client's address as the request's.
//     TestChannel_ARequestReachesTheHandlerAndItsResponseComesBackWhole.
//   - A channel closing ends its request: the handler's context ends with it, so a request whose
//     client gave it up, closing its channel, is not still worked on. There is no other cancel.
//     TestChannel_AChannelTheClientClosesEndsItsRequest.
//   - A response is sent in messages of at most MaxMessageBytes, which every browser carries, and
//     waits while its channel holds too much, so no film is held in memory.
//     TestChannel_ALargeResponseArrivesWholeInMessagesABrowserCarries.
//   - A response sends InitialCreditMessages messages of its body, then only as many more as its
//     client grants as it reads ({"credit": n} after the request), so a film asked for from its
//     first byte never sits whole in the browser. TestCredit_AResponseWaitsForTheClientsNextGrant.
//   - Requests on their own channels are answered at once, none waiting for another.
//     TestChannel_RequestsOnTheirOwnChannelsAreAnsweredAtOnce.
//   - A head that is not one, a body over MaxRequestBodyBytes, or a path naming another host is
//     refused. TestChannel_AHeadThatIsNotOneIsRefused.
//   - A request asking to become a WebSocket is handed to its handler with a MessageConn in its
//     context (MessageConnFrom); once accepted, a text message each way is one message and the core's
//     binary message its close, with the WebSocket's code. internal/core/api's
//     TestRTCOffer_AWatchTogetherDeviceConnectsOverADataChannel.
//   - With the STUN servers it is given (UseICEServers: the core's own, never its discovery's, the
//     Plex-chain review's L3), an answer carries the public address the core's NAT gives it.
//     TestUseICEServers_TheAnswerCarriesThePublicAddressSTUNFound.
//   - Each connection's path (direct, punched or relayed, by family; or failed) is counted and
//     logged, told by the core's end of its selected pair (D274 §3).
//     TestAnswer_CountsThePathEachConnectionTook, TestPathOf_IsToldByTheCoresEndOfThePair.
//   - A request over WebRTC carries the core's own public IPv6 /64s on its connection, from its host
//     and STUN-found candidates alone, never a private, link-local, relayed or client address (D297
//     §6). TestOwnIPv6Networks_AreThePublicIPv6CandidatesOfTheCoresOwnEnd.
//   - A core taking WebRTC at home gathers its host candidates on one UDP port and answers with the
//     house's address at it, so a device in the house reaches it directly (D300 §1).
//     TestListenLAN_AnswersWithTheHousesAddressAtItsPort.
//   - Only an offer is answered, and none past MaxConnections.
//     TestAnswer_RefusesWhatIsNotAnOfferAndPastTheLimit.
//   - An offer a discovery passed on is checked by the core before any connection is made: one
//     sha-256 fingerprint, at most MaxRelayedCandidates candidates, mDNS ones dropped and so is every
//     one naming an address that is private, loopback, link-local or in the core's own networks; its
//     connection checks no such address but one in the house's networks that a check reached the core
//     from (the security audit's A2, D300 §2). TestRelayed_AnswersAHomeAddressOnlyAsTheSourceOfItsOwnCheck,
//     TestCheckRelayedOffer_TakesOneSHA256FingerprintAndAFewCandidates,
//     TestIsFromOutside_RefusesEveryAddressOfTheCoresOwnNetworks,
//     TestAnswerRelayed_ConnectsNoClientOnTheCoresOwnNetwork. A development build alone lifts the
//     address check (AcceptAnyRelayedAddress), for browser tests on one machine; a release never
//     does. TestServer_ARelayedConnectionChecksTheCoresOwnNetworksOnlyWhenTold and
//     TestRelaxForDevelopment_ChangesNothingInARelease.
//   - Each connection is bounded: closed unless it connects within its bound, and past its network's
//     share once it does; its client's messages no larger than MaxMessageBytes and its SCTP buffer
//     bounded; its requests in flight limited, each arriving whole by a deadline.
//     TestAnswerRelayed_ConnectsNoClientOnTheCoresOwnNetwork, TestAnswer_HoldsEachClientWithinItsBounds,
//     TestChannel_ARequestThatDoesNotArriveInTimeIsCutOff.
//   - A connection is told its Verdict on its negotiation channel once it connects: held, or why it is
//     turned away (RefusedNetworkFull), which it then serves no request for, so the page can say why
//     rather than blame the device's network. TestAnswer_TellsAConnectionPastItsNetworksShareWhy.
//   - The handler believes nothing a request's head says of where it came from: Origin, Host,
//     Forwarded and X-Forwarded-* are dropped, and IsOverWebRTC tells it the request came over a
//     channel (the security audit's A1). TestChannel_TheHandlerBelievesNothingTheHeadSaysOfWhereItCameFrom.
//   - A connection grants nothing: each request carries its own sign-in, and the offer endpoint
//     answers only a signed-in client (internal/core/api's
//     TestRTCOffer_ASignedInClientReachesTheAPIOverDataChannels).
//
// # Read before changing
//
//   - D273 §7, §8: why WebRTC, and the measurement behind ClientConnections and cwndStepBytes.
//   - docs/contributors/design/plans/webrtc-transport.md: the transport's shape and phases.
//
// # Where to change
//
//   - rtcapi.go: offers, connections and their bounds; relayed.go: the core's own check of an offer
//     a discovery passed on; path.go: the paths connections take;
//     channel.go: reading a request;
//     writer.go: sending a response; message.go: a request that became a WebSocket.
package rtcapi
