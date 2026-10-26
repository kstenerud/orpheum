// SPDX-License-Identifier: AGPL-3.0-or-later

// Package together is Watch Together's sessions (cut-a §5.1): who is in each, what their devices
// report, the group's state, and the messages each device is sent. Sessions live in memory; a
// restart ends them. A session is the host's: it starts with nothing on, and a film the host plays
// is on for everyone (D122, D246 §5). Each person joins by a link of their own, once, which the
// host notes who it is for; the viewer it admits is an ephemeral account that sees only the
// playback, and its membership credential is what its player plays by (D246 §3). The WebSocket
// and the endpoints are internal/core/api's (together.go, together_socket.go); this package holds
// no connection.
//
// # Invariants
//
//   - Imports nothing from internal/node, never os/exec, and no image or media parser. Enforced by
//     the depguard rule core-boundary, and transitively by TestArch_CoreNeverImportsNode and
//     TestArch_CoreNeverExecs.
//   - Opens no file and walks no directory; files are internal/core/instancedir's. Enforced by the
//     forbidigo rule tagged [core-fs].
//   - Imports no database/sql or SQLite driver; it keeps nothing past the process (sessions end
//     with it). Enforced by the depguard rule sql-only-in-store and
//     TestArch_OnlyStoreImportsDatabaseSQL.
//   - Every switch over a message type or group state names every value. Enforced by exhaustive.
//   - Starts no goroutine: a device's reading and writing are the API's, and the hub sends to a
//     device through the Sender it attached with, which must never block. Review-only (standards/go.md,
//     Concurrency); the race detector runs in make test-all.
//   - The group waits for its players only at its sync points: a film put on, the host's play after
//     a pause, and the host's seek, each waiting for every player that has once been ready, those
//     catching up too; one still waiting for its tap or loading holds no one up (D246 §6, D250 §2).
//     Enforced by TestHub_TheGroupPlaysWhenEveryPlayerIsReadyAndPlaysOnWhenOneBuffers and
//     TestHub_AJoiningViewerHoldsNoOneUpUntilReady.
//   - The group never stops for one member: a stall or a drop leaves it playing, and the player
//     catches up alone (D250 §2). Enforced by TestHub_AStalledPlayerStopsNoOneAndCatchesUpAlone and
//     TestHub_ADroppedPlayerStopsNoOne.
//   - A sync point waits at most the sync wait its devices were welcomed with, then plays without
//     whoever is not ready, the host's player as anyone's, but never to no one (D250 §3). Enforced
//     by TestHub_ASyncPointWaitsAtMostTheSyncWaitThenPlaysWithoutWhoeverIsNotReady and
//     TestHub_WithNoPlayerReadyASyncPointWaitsOnPastItsTime.
//   - A group that starts playing at a sync point's end stands at its position for the start's lead
//     its devices were welcomed with, and says what is left of it, so every player starts at once;
//     one that plays on moves on as it was (D268 §3). Enforced by
//     TestHub_AGroupStartsMovingTheStartsLeadAfterItsSyncPointEnds.
//   - A guest plays and pauses, and changes the position, only as their link allows; what plays,
//     stopping it and its rate are the host's alone; each link starts with the last one's rights
//     (D271 §4). Stopping leaves nothing on; a seek made against a group another seek has changed
//     is refused as stale (D246 §5, §8). Enforced by TestHub_OnlyTheHostControlsPlayback,
//     TestHub_AGuestDoesToPlaybackWhatTheirLinkAllows, TestHub_ANewLinkStartsWithTheLastOnesRights,
//     TestHub_TheHostStoppingLeavesNothingPlaying and TestHub_ASeekAgainstAnotherSeekIsRefusedAsStale.
//   - A new link for a guest gives their place: they stay in until it is used, whoever joins by it
//     takes the place over, the devices they came in on cut off, and the place's other links end;
//     removed, their place's links admit no one (D267 §4 amended, D318). Enforced by
//     TestHub_ANewLinkForAGuestMovesTheirPlaceOnlyOnceItIsUsed.
//   - Each link admits the one person it was made for, once, its note the host's alone; a withdrawn
//     link admits no one; an ejected member's credential admits nothing; a host has one session.
//     Enforced by TestHub_EachLinkAdmitsOnePersonOnce, TestHub_AnEjectedMemberIsGone,
//     TestHub_OpeningAgainGivesTheHostTheirSession and, through the API,
//     TestTogether_OnlyAHostOpensMakesLinksAndEnds.
//   - A cast is a session of one (D88, D337): its host is the person who cast, its receiver leads
//     and its remote, a device with no player, commands it; it has no links and is never among the
//     person's sessions nor their party, which is opened and kept beside it; ending it tells each
//     of its devices why. Enforced by TestCast_IsASessionOfOneNoOneElseFindsAndAPartyStaysBesideIt
//     and TestCast_TheRemoteCommandsTheReceiverAndEndingTellsBothWhy.
//   - When the server's upload cannot carry every player at its lowest quality, the member who
//     joined most recently with a player, never the host, is told the session ended for them with
//     uploadFull, and their membership still admits them (D286 §5). Enforced by
//     TestHub_SendAwayNewestTellsTheNewestPlayerAndKeepsTheirMembership and
//     TestHub_SendAwayNewestSendsNoHost.
//   - A signed-in person's sessions follow them to any device, each device with a credential of
//     its own that cuts off no other, a member keeping the newest few (D267 §2, §3). Enforced by
//     TestMine_IsEverySessionAPersonIsInAsHostOrSignedInMember,
//     TestMine_HavingTheSessionBackOnOneDeviceCutsOffNoOther, TestMine_KeepsAMembersNewestCredentials
//     and, through the API, TestTogether_ASessionFollowsWhoeverJoinedSignedIn.
//   - The host puts on only a film their role reaches (D243 §4), and a viewer plays only what is on,
//     while they are in. Enforced by TestHub_TheHostChangesWhatPlaysOnlyToAFilmTheyMayPlay,
//     TestHub_AViewerPlaysWhatTheSessionPlaysWhileTheyAreIn and, through the API,
//     TestTogether_AViewerPlaysOnlyTheFilmOnInTheHostsLibraries.
//   - A group's change is a code a client words, never words, and a seek says where it jumped from
//     (D188). Enforced by TestHub_AnEventIsACodeAndASeekSaysWhereItJumpedFrom.
//   - The roster shows the round trip each device last reported; only the device measures it
//     (D66). Enforced by TestHub_TheRosterShowsEachDevicesRoundTripItReported.
//   - A device's state report sends the roster only when it changed what every device was last
//     told, and a message to every device is encoded once for them all. Enforced by
//     TestHub_AStateThatChangesNothingSendsNoRoster and TestBroadcast_IsEncodedOnceForEveryDevice.
//   - Voice signalling goes only to the device it names. Enforced by
//     TestHub_RelaysVoiceSignallingToTheDeviceItNames.
//   - A discovery finds the core by each unused link's locate hash, its offers proven under the link's
//     key, and by each membership device's locator, its offers proven under the device's credential;
//     a used link locates nothing, the host's device has its own session's locator, and ending a
//     session or a membership drops its own (discovery-core.md K7; A12). Enforced by
//     TestLiveLinks_AreTheUnusedLinksAndEachMembershipDeviceIsFoundByItsLocator,
//     TestLiveLocators_GoWithAMemberWhoLeaves, TestLiveLocators_EndWithTheSessionAndEachDeviceHasItsOwn
//     and, through the wiring, TestLinksOf_TheDiscoveryFindsTheCoreByWatchTogethersLinksAndLocators.
//
// # Read before changing
//
//   - D62: Watch Together is first-class; the client surface is six methods.
//   - D66: no clock synchronisation.
//   - D117, D119, D122, D125: the session, its link and its guests, as D246 revised them.
//   - D246 §3, §5, §6, §8: the viewer, the host's session, joining and control, as D271 §4 revised
//     it: a guest's rights.
//   - D250 §2–§5: the group stops only at its own sync points; a member's trouble is theirs to
//     catch up on.
//   - D191: the session behaves as if nobody has an account.
//   - D264, D268 §1: a pong carries where the group is, by which a follower places it.
//   - D267: a membership kept by the browser that joined or a signed-in person's account, on
//     several devices at once.
//   - D268: drift corrected as the device measures it, the group's named start, and the notes that
//     say how a follower follows.
//   - D243 §1, §3 and §9: a guest's cookie, secrets after #, and the numbers it is held to, as
//     D261 §1, §7 revised them; a signed-in member's viewing is deferred (D261 §3).
//   - docs/contributors/design/plans/watch-together.md, and cut-a §5.1: the messages.
//
// # Where to change
//
//   - messages.go: the wire messages, as cut-a §5.1 lists them.
//   - hub.go: sessions, memberships and devices. links.go: the links a person joins by, and the
//     place each gives (D267 §4 amended, D318).
//   - discovery.go: what a discovery finds the core by: links' locate hashes and devices' locators.
//   - group.go: the group's state machine: the host's commands, waiting for players, and stalls.
//   - cast.go: a cast's session of one (D337; plans/cast-to-device.md §3).
package together
