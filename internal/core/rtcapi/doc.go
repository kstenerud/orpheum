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
//   - With its discovery's STUN servers (UseICEServers), an answer carries the public address the
//     core's NAT gives it. TestUseICEServers_TheAnswerCarriesThePublicAddressSTUNFound.
//   - Each connection's path (direct, punched or relayed, by family; or failed) is counted and
//     logged, told by the core's end of its selected pair (D274 §3).
//     TestAnswer_CountsThePathEachConnectionTook, TestPathOf_IsToldByTheCoresEndOfThePair.
//   - Only an offer is answered, and none past MaxConnections.
//     TestAnswer_RefusesWhatIsNotAnOfferAndPastTheLimit.
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
//   - rtcapi.go: offers, connections and their bounds; path.go: the paths connections take;
//     channel.go: reading a request;
//     writer.go: sending a response; message.go: a request that became a WebSocket.
package rtcapi
