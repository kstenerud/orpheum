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
//   - A session has at most one leader, the screen that is the film (D371 §1), chosen among its
//     contenders alone: the devices whose player has the session's film, as its latest state says
//     (its fileId) or as it put it on, on a page its browser shows (D374 §1, contends); attached in
//     a playing role is not enough, and with nothing on no one leads. Every choice of leader goes
//     through the one predicate. The host's contenders are preferred (D372 §3, D374 §2): the one
//     that put the film on, and the newest as it first contends; as it leaves, the host's newest
//     other contender. While none of the host's contends, the guest's contender whose current run
//     started earliest leads (D377 §2): a run starts as the core first hears the device playing or
//     ready with the session's film, by the core's clock to the microsecond, ties to the lower id;
//     connecting alone starts none; it ends, the device to the back of the line, as the device
//     disconnects, its page is hidden, its player leaves the film, or it stays stalled or paused past
//     stalledLeaderGrace, each start and end logged (runStarted, runEnded why). A host's player
//     coming back while a guest's leads a film follows until it plays within the drift a follower
//     leaves alone, then leads (D372 §3, D377 §6). No panel, no hidden page and no player without the
//     film leads: a cast's sender whose page keeps its connection says it plays no film, and its
//     receiver contends in its place. The roster says who leads, and each change of leader is logged
//     with why, when the new leader's run started, and each player passed over and why (passedOver).
//     Enforced by TestHub_TheHostsPlayerLeadsAndAnotherTakesTheLeadAsItLeaves,
//     TestHub_NoPanelLeadsAndAGuestsLeadsWhileNoneOfTheHostsCan,
//     TestHub_TheLeadPassesToThePlayerPresentLongest,
//     TestHub_TheLineForTheLeadIsByRunNotByArrival,
//     TestHub_ADeviceThatComesBackIsPresentOnlySinceItCameBack,
//     TestHub_TheHostsPlayerComingBackFollowsUntilInStepThenLeads,
//     TestHub_ACastingSenderNeverLeadsAndItsReceiverDoes,
//     TestHub_APlayerNotPlayingTheFilmIsPassedOverForTheLead and
//     TestHub_TheHostsPlayerPuttingAFilmOnLeadsItWhileItLoads.
//   - A leader whose connection closes or goes silent, whose page its browser hides, whose member
//     is removed, or whose player no longer has the film (goneBecause noFilm or otherFilm) is gone,
//     and the lead passes at once (D372 §5, D374 §1); a Watch Together leader stalled while its
//     group plays is gone once its stall outlasts stalledLeaderGrace, and the lead passes to a
//     contender that plays on, the log saying goneBecause=stalled and how long (D373 §2). A shorter
//     stall keeps the lead, and the leader, playing again behind its group, catches up to it as a
//     follower does, its states placing the group only once in step, so the group never steps back
//     for it (D374; leaderCaughtUp). The host's player stalled follows, and takes the lead back once
//     in step; a guest's just follows (D373 §3). Leading carries the clock only: every right stays
//     the host's or the link's (D372 §4). Enforced by
//     TestHub_ALeaderHiddenOrSilentIsGoneAndAStalledOneIsNot,
//     TestHub_ALeaderStalledPastItsGracePassesTheLeadAndTheGroupPlaysOn,
//     TestHub_ALeadersStallWithinItsGraceKeepsTheLead,
//     TestHub_TheHostsStalledPlayerTakesTheLeadBackInStepAndAGuestsFollows,
//     TestHub_ACastingSenderNeverLeadsAndItsReceiverDoes and
//     TestHub_AGuestsLeadingPlayerCarriesTheClockOnly.
//   - With no one left who can lead, a Watch Together group with a film on is held Paused where it
//     stood as its last contender went, its clock still, and keeps whether it was playing (to play or
//     standing for its leader) or a person had paused it: the hold is no person's pause, and the group
//     says it (isHeldPlaying), the log too (heldPlaying). The first contender back leads it on as it
//     was, with no time limit: one held playing stands where it was held for its leader (Waiting,
//     resumedOnReturn), then plays as the leader plays; one a person paused stays paused (D375 §1,
//     §2). A held group is idle, and ends by its idle end; a cast is left to its receiver's return
//     (D372 §7, D375 §3). Enforced by TestHub_WithEveryoneGoneTheGroupPausesWhereItStandsThenEndsIdle,
//     TestHub_AGroupLeftWhilePlayingIsHeldAndPlaysOnAsAPlayerReturns,
//     TestHub_AGroupLeftWhilePausedStaysPausedAsAPlayerReturns and
//     TestHub_AGroupHeldForWantOfAPlayerEndsIdle.
//   - A device's own position becomes the group's only as it is chosen to lead a group no one leads
//     (D377 §4): every change of leader goes through one rule (placeByNewLeader), and its
//     leaderChanged line says positionFrom=leader|core. A handover from a live leader takes no
//     position, its successor in step; chosen while none led, held for want of a player or not, a
//     device presents its position if it has one of its own (its latest state has the session's film,
//     says remembersPlace, which a page loaded afresh says only once it has played along, and is no
//     older than the group's last change), else the core's stands. Enforced by
//     TestHub_AFollowerTakingOverFromALiveLeaderMovesNoOne and
//     TestHub_ALeaderlessGroupGoesOnFromThePositionItsNewLeaderPresents.
//   - Play or pause is the session's (D377 §5): a leader paused by its browser, not its person, while
//     another plays the film neither stops the group nor keeps the lead past stalledLeaderGrace
//     (goneBecause=paused). Enforced by TestHub_ALeadersOwnPauseNeitherStopsTheGroupNorHoldsTheLead.
//   - The group's position is its leader's, which nothing steers (D371 §1): each state the leader
//     reports while the group plays places the group where it is, and a group to play (a film put
//     on, a play, a seek) stands where it is until its leader plays, then plays from there at once:
//     starting still waits (D373 §4), and loading before the group plays is no stall. Enforced by
//     TestHub_AFilmPutOnStandsForItsLeaderAloneAndPlaysAsTheLeaderPlays,
//     TestHub_AHostsSeekStillWaitsForItsLeaderAndPassesNoLead and
//     TestHub_TheGroupIsWhereItsLeaderIsAndAFollowersStallChangesNothing.
//   - No one's stall holds a Watch Together group that plays while another contender plays the film
//     (D373 §1): neither its leader's nor a follower's moves it to Waiting, and each stall is logged
//     as it ends, who and how long (stallNotWaitedFor). The only contender's stall its group stands
//     for, where it stalled, its clock still, as a film played alone waits (D374 §3); a cast's
//     receiver, a session of one, is that case, one rule. Enforced by
//     TestHub_TheGroupIsWhereItsLeaderIsAndAFollowersStallChangesNothing,
//     TestHub_ALeaderStalledPastItsGracePassesTheLeadAndTheGroupPlaysOn,
//     TestHub_TheOnlyContendersStallIsWaitedFor and TestCast_AReceiversStallStandsItsGroupForIt.
//   - The group waits for no one but its leader as it starts (D371 §2): a follower loading,
//     stalling or dropping changes nothing, its trouble its own to catch up on; without a leader a
//     play or a seek plays at once. Enforced by TestHub_AJoiningViewerHoldsNoOneUp,
//     TestHub_ADroppedPlayerStopsNoOne and TestHub_ACommandOfAnotherDeviceStandsForTheLeaderToCarryItOut.
//   - A group that starts playing moves on from where it stands at once, with no lead to start on
//     and no run-up, with a leader or without, everyone else catching up (D371 §2, §6). Enforced by
//     TestHub_AGroupWithNoLeaderPlaysAtOnceFromWhereItStands and
//     TestHub_AFilmPutOnStandsForItsLeaderAloneAndPlaysAsTheLeaderPlays.
//   - Each follower's move to catch up to its leader, a seek or a nudge, is logged at info with how
//     far behind the leader it was, and the leader is named at each change of the group and of who
//     leads (D371). Enforced by TestHub_LogsEachFollowersCatchUpAndHowFarBehindItsLeader and
//     TestHub_LogsEachChangeOfTheGroupAndWhoLeadsIt.
//   - Each player's state may say how it plays the film, its delivery, quality and link, which the
//     roster shows everyone, a cast's remote among them (D371 §4), and how its film sounds, the
//     dialogue boost, levelling, the audio track among the file's own, and each channel's level and
//     mute, the film's alone so far (D440 §2). Enforced by
//     TestHub_TheRosterSaysHowEachPlayerPlaysTheFilm and TestHub_TheRosterSaysHowEachPlayersFilmSounds.
//   - A member's choice of their film's sound made on a device of theirs (MessageSound), a cast's
//     remote, is relayed to their own other devices that play it, which carry it out as their own
//     press, and to no one else's; one no remote may make is refused. The server keeps nothing of
//     it. In a cast it reaches the receiver whatever its part, and the receiver may say its playback
//     alone (MessagePlayback), so the remote works it in the cast's session while it leads its
//     person's Watch Together session. Enforced by TestCast_TheRemoteChoosesTheReceiversSound and
//     TestCast_TheRemoteWorksTheReceiverInTheCastWhileItLeadsWatchTogether.
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
//     link admits no one; an ejected member's credential admits nothing. Enforced by
//     TestHub_EachLinkAdmitsOnePersonOnce, TestHub_AnEjectedMemberIsGone and, through the API,
//     TestTogether_OnlyAHostOpensMakesLinksAndEnds.
//   - A person is in one session at most, as host or guest (D372 §8): starting one ends the one
//     they host, its devices told the host ended it, and takes them out of one they joined; a join
//     by someone in another, signed in or by the membership their browser keeps, is refused, saying
//     whose, until they say to leave it. A cast is not among a person's sessions. Enforced by
//     TestHub_StartingASessionEndsTheOneThePersonHostsAndLeavesTheOneTheyJoined,
//     TestHub_JoiningAnotherSessionIsRefusedUntilThePersonSaysToLeaveTheirs and, through the API,
//     TestTogether_JoiningAnotherSessionAsksToLeaveTheOneYouAreIn.
//   - Every way a Watch Together session ends, its host's end, its host starting or joining another,
//     or its idle end, goes through one (endSession): each device told why, and whoever registered
//     (OnSessionEnded) told the session once the hub's lock is let go, so the casts whose receiver
//     takes a part in it end with it, as sessionEnded (D389 §1); TogetherPart says which session a
//     part is in. A cast's own end is no Watch Together session's. Enforced by
//     TestHub_EveryWayASessionEndsIsToldWithThePartsCastsEndWith and, through the API,
//     TestCast_ASessionsEndEndsTheCastsThatTakeAPartInIt in internal/core/api.
//   - A Watch Together invitation is short-term (D346): a link admits its person for its lifetime
//     from when it was made, then no one, and leaves the host's list and the discovery's links; a
//     session idle for its idle end, neither played to a player nor acted in by a member, is ended
//     as the host's end would, each device told "idle". Enforced by
//     TestHub_ALinkAdmitsNoOneOnceItsLifetimeHasPassed, TestHub_ASessionIdleForItsIdleEndIsEnded and
//     TestHub_AGroupPlayingToNoPlayerIsIdle.
//   - A cast is a session of one (D88, D337): its host is the person who cast, its receiver leads,
//     playing as the device plays a film alone, and its remote, a device with no player, commands it,
//     each command standing for the receiver to carry out (D371 §4, §5); it has no links and is never among the
//     person's sessions nor their party, which is opened and kept beside it; ending it tells each
//     of its devices why. Enforced by TestCast_IsASessionOfOneNoOneElseFindsAndAPartyStaysBesideIt
//     and TestCast_TheRemoteCommandsTheReceiverAndEndingTellsBothWhy; a device saying hello with
//     one of its credentials within a minute of its end is told why too (CastEndedError), enforced
//     by TestCast_AHelloJustAfterTheEndIsToldWhyForAMinute. Ready between videos it plays
//     nothing, so the next is put on afresh (D364 §5), enforced by
//     TestCast_ReadyPlaysNothingSoTheNextIsPutOnAfresh; a receiver takes its person's part in the
//     Watch Together session they are in, a host's credential leading it (D364 §11), a signed-in
//     guest's following as just another follower (D371 §4), enforced by
//     TestCast_AHostsReceiverHasACredentialForTheirSession and
//     TestCast_AGuestsReceiverTakesTheirPartAsAFollower.
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
//   - A credential admits by what it is, never by being there: no membership is found, viewed,
//     attached to, left or joined by one empty, no one's, one character off, of the other kind (a
//     link's for a membership's and back), an ejected member's or an ended session's; a session and
//     a cast are managed by their host alone, by ID. TestHub_NoCredentialButTheLiveOneItIsAdmitsAnyone,
//     TestHub_OnlyItsHostManagesASessionByItsID, TestCast_OnlyItsSenderHasACredentialForItOrEndsIt.
//
// # Read before changing
//
//   - D62: Watch Together is first-class; the client surface is six methods.
//   - D66: no clock synchronisation.
//   - D117, D119, D122, D125: the session, its link and its guests, as D246 revised them.
//   - D246 §3, §5, §6, §8: the viewer, the host's session, joining and control, as D271 §4 revised
//     it: a guest's rights.
//   - D371: a session's leader plays as a device plays alone; everyone else catches up to it, and a
//     cast takes its sender's part. It revises D250 §2–§3 (no sync points but the leader's) and
//     D247 §4 (no group waits for a stalled viewer).
//   - D372: the lead passes to the longest-present player while the host's screen is gone, and a
//     person is in one session at most.
//   - D373: no one's stall holds a Watch Together group; a leader stalled past a short grace passes
//     the lead. It amends D372 §5 and D371's resolution that the group stands for its leader's stall.
//   - D374: only a device playing the film contends for the lead, a cast's sender never; a lone
//     contender's stall is waited for (amending D373 §1); a leader recovered within the grace catches
//     up to its group.
//   - D375: a session left with no player keeps its playing state, and goes on as it was when a
//     leader returns (amending D372 §7).
//   - D377: who leads and from where, one procedure: contenders, the line by run, a device's own
//     position official only when no one leads, play or pause the session's (replacing D372 §1–§3
//     and D376).
//   - D250 §4–§5: a member's trouble is theirs to catch up on, from what is already being made.
//   - D191: the session behaves as if nobody has an account.
//   - D264, D268 §1: a pong carries where the group is, by which a follower places it.
//   - D267: a membership kept by the browser that joined or a signed-in person's account, on
//     several devices at once.
//   - D268: drift corrected as the device measures it, and the notes that say how a follower
//     follows; its §3, the group's named start, gone with D371.
//   - D346: a link admits for an hour, and a session idle for six hours is ended.
//   - D243 §1, §3 and §9: a guest's cookie, secrets after #, and the numbers it is held to, as
//     D261 §1, §7 revised them; a signed-in member's viewing is deferred (D261 §3).
//   - docs/contributors/design/plans/watch-together.md, and cut-a §5.1: the messages.
//
// # Where to change
//
//   - messages.go: the wire messages, as cut-a §5.1 lists them; note_known.go, which of a device's
//     notes the server logs.
//   - hub.go: sessions, memberships and devices. links.go: the links a person joins by, and the
//     place each gives (D267 §4 amended, D318).
//   - discovery.go: what a discovery finds the core by: links' locate hashes and devices' locators.
//   - group.go: the group's state machine: the commands, and following the leader.
//   - leader.go: who contends for the lead, who leads, and how the lead passes (D371, D372, D374).
//   - places.go: a person in one session at most (D372 §8).
//   - lifetimes.go: how long a link admits, and when an idle session ends (D346).
//   - cast.go: a cast's session of one (D337; plans/cast-to-device.md §3).
package together
