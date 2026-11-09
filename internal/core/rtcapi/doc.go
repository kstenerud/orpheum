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
//   - A channel its client holds open answerLingersAfter or longer after the core's last byte for its
//     response, handed to SCTP whole or waiting for SCTP's buffer to drain, is logged as it closes:
//     its connection, SCTP stream and label, its path, what SCTP still held then and at the end, and
//     how soon after another channel of its stream closed it opened. A playback answered and never
//     received said nothing of where its bytes stopped (staging, 2026-10-30).
//     TestChannel_AChannelHeldOpenLongAfterItsAnswerIsLoggedByItsStream,
//     TestAnswerWatch_LogsAChannelOnlyOnceItLingeredPastTheCoresLastByte,
//     TestHeld_SaysHowSoonAStreamWasUsedAgain.
//   - Each connection that connected logs one line as it ends, "rtc connection ended" (event connectionEnded),
//     read before pion drops its SCTP association: its connection, path, why it ended (the browser closed it,
//     ICE failed as the browser stopped answering, or the core closed it, stopped or turned it away),
//     how long it lived since it connected,
//     and SCTP's bytes sent and received, congestion window, smoothed round trip, receiver window and
//     packet size at the end. pion keeps its retransmission and T3 timeout counts unexported, so the
//     line has none (D397). TestEnded_AConnectionLogsItsSCTPFiguresAsItEnds,
//     TestEnded_AConnectionTheCoreClosesAsItStopsSaysSo.
//   - What a connected connection sends goes through the pacer, every socket ICE opens and the LAN
//     port's under its mux: to the client's end at most twice its window a smoothed round trip, never
//     above 300 Mb/s, in bursts of at most 16 KiB, so Chrome's 64 KiB socket buffer is not overrun.
//     A relayed connection's sends to its TURN server, STUN and ICE's checks are not paced. The
//     kernel paces UDP only under fq, so this is not internal/platform/pacing's job (D397).
//     TestPacer_PacesWhatAConnectionSends, TestPacer_HoldsAFollowedAddressToItsRateAndBurst,
//     TestPaceRate_IsTwiceTheWindowARoundTripUnderTheCeiling, TestPacer_LetsAnUnfollowedAddressGoAtOnce.
//   - A response's channel waits only past 8 MiB buffered, down to 4 MiB, so the client's whole
//     credit can be in flight (D397). TestBuffered_LetsTheClientsWholeCreditBeInFlight.
//   - A client's video m-line, which it offers so Chrome grows its socket buffer from 64 KiB, is
//     answered sendonly with VP8 alone, naming no source, and the core sends nothing on it (D397).
//     TestVideo_AClientsVideoLineIsAnsweredAndNothingSentOnIt.
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
//   - Each connection's path (direct, punched or relayed, by family; hairpinned; or failed) is counted
//     and logged, told by the core's end of its selected pair (D274 §3); the log names both ends by
//     type and address. TestAnswer_CountsThePathEachConnectionTook, TestPathOf_IsToldByTheCoresEndOfThePair,
//     TestRecordPath_NamesBothEndsOfThePairByTypeAndAddress.
//   - A punched IPv4 pair whose two ends are one address is hairpinned, out to the NAT both share and
//     back, and logged as a warning; it stands as HairpinWarning, with where the core takes WebRTC at
//     home, until a direct connection from a home network, or for HairpinWarningLasts after the last
//     (D354). TestHairpinWarning_StandsUntilADirectConnectionFromTheHouse,
//     TestHairpinWarning_LapsesAWeekAfterTheLastHairpin.
//   - A connection a discovery's offer made is told to its answerer as proven at home (AnswerRelayed's
//     onProvenAtHome), and a request over any connection is marked from home (IsDirectlyFromHome),
//     only when its selected pair is direct, the core's end a host candidate, and the client's end in
//     the home rule (SetHome: internal/core/home's, the one api judges a direct request by, D452 §1) or
//     the core's own public IPv6 networks, its host and STUN-found candidates' /64s
//     (D297 §6); a relayed, punched or hairpinned one, or a direct one from anywhere else, never is
//     (D356 §1, D354 §4, D439). It is one judgement, provenAtHome's, which api reads and does not make
//     (D439 §2); a request's is made on the pair and the home rule as it finds them. The own networks
//     the last connection to connect found are kept (OwnIPv6Networks), which the home rule counts for
//     a direct request too (D452 §2).
//     TestProvenAtHome_ADirectPairIsJudgedByTheSharedRule,
//     TestOwnIPv6Networks_AreThePublicIPv6CandidatesOfTheCoresOwnEnd,
//     TestProvenAtHome_ADirectConnectionFromTheHouseProvesItself,
//     TestProvenAtHome_ARelayedPunchedOrHairpinnedConnectionProvesNothing,
//     TestAnswerRelayed_TellsAConnectionProvenAtHome, TestIsDirectlyFromHome_OnlyADirectPairFromTheHouseIs.
//   - A core taking WebRTC at home gathers its host candidates on one UDP port and answers with the
//     house's address at it, so a device in the house reaches it directly (D300 §1); its host
//     candidates there are one address of each family, the house's in place of its own IPv4 one, for
//     pion's mux gives every candidate of a family one socket (D441).
//     TestListenLAN_AnswersWithTheHousesAddressAtItsPort,
//     TestListenAddressPerFamily_IsOneOfEachFamilyNeitherLoopbackNorLinkLocal.
//   - A new address at home is offered from the next offer on, on the same socket (D452 §3).
//     TestListenLAN_ANewAddressAtHomeIsOfferedFromTheNextOffer.
//   - Only an offer is answered, and none past the most connections the core holds at once:
//     DefaultMaxConnections until SetMaxConnections names another (the config's
//     webrtc.max_connections, D348). TestAnswer_RefusesWhatIsNotAnOfferAndPastTheLimit,
//     TestSetMaxConnections_RefusesAnOfferPastTheConfiguredLimit.
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
//   - D397: the pacer, the buffered thresholds, the video m-line and the end line, for Chromium.
//   - docs/contributors/design/plans/webrtc-transport.md: the transport's shape and phases.
//
// # Where to change
//
//   - rtcapi.go: offers, connections and their bounds; relayed.go: the core's own check of an offer
//     a discovery passed on; path.go: the paths connections take; local_path.go: the hairpin warning;
//     proven_home.go: a connection proven at home (D356);
//     channel.go: reading a request;
//     writer.go: sending a response; message.go: a request that became a WebSocket; ended.go: the
//     line a connection logs as it ends; pacer.go: the pacing of what connections send; video.go: the
//     video m-line every connection answers.
package rtcapi
