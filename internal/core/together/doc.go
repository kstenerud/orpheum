// SPDX-License-Identifier: GPL-3.0-or-later

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
//   - What the host puts on waits until every player that has once been ready is, then plays, and
//     the group waits while any of them buffers or is gone; one still choosing its speakers or
//     loading holds no one up (D246 §6, §9). Enforced by
//     TestHub_TheGroupPlaysWhenEveryPlayerIsReadyAndWaitsForOneThatBuffers,
//     TestHub_AJoiningViewerHoldsNoOneUpUntilReady and
//     TestHub_ADroppedPlayerMakesTheGroupWaitUntilItIsBack.
//   - Only the host controls playback in Cut A, and stopping leaves nothing on; a seek made against
//     a group another seek has changed is refused as stale (D246 §5, §8). Enforced by
//     TestHub_OnlyTheHostControlsPlayback, TestHub_TheHostStoppingLeavesNothingPlaying and
//     TestHub_ASeekAgainstAnotherSeekIsRefusedAsStale.
//   - Each link admits the one person it was made for, once, its note the host's alone; a withdrawn
//     link admits no one; an ejected member's credential admits nothing; a host has one session.
//     Enforced by TestHub_EachLinkAdmitsOnePersonOnce, TestHub_AnEjectedMemberIsGone,
//     TestHub_OpeningAgainGivesTheHostTheirSession and, through the API,
//     TestTogether_OnlyAHostOpensMakesLinksAndEnds.
//   - The host puts on only a film their role reaches (D243 §4), and a viewer plays only what is on,
//     while they are in. Enforced by TestHub_TheHostChangesWhatPlaysOnlyToAFilmTheyMayPlay,
//     TestHub_AViewerPlaysWhatTheSessionPlaysWhileTheyAreIn and, through the API,
//     TestTogether_AViewerPlaysOnlyTheFilmOnInTheHostsLibraries.
//   - A group's change is a code a client words, never words, and a seek says where it jumped from
//     (D188). Enforced by TestHub_AnEventIsACodeAndASeekSaysWhereItJumpedFrom.
//   - The roster shows the round trip each device last reported; only the device measures it
//     (D66). Enforced by TestHub_TheRosterShowsEachDevicesRoundTripItReported.
//   - Voice signalling goes only to the device it names. Enforced by
//     TestHub_RelaysVoiceSignallingToTheDeviceItNames.
//
// # Read before changing
//
//   - D62: Watch Together is first-class; the client surface is six methods.
//   - D66: no clock synchronisation.
//   - D117, D119, D122, D125: the session, its link and its guests, as D246 revised them.
//   - D246 §3, §5, §6, §8, §9: the viewer, the host's session, joining, control and waiting.
//   - D191: the session behaves as if nobody has an account.
//   - D243 §1–§3 and §9: a guest's cookie, a signed-in member's viewing, secrets after #, and the
//     numbers it is held to.
//   - docs/contributors/design/plans/watch-together.md, and cut-a §5.1: the messages.
//
// # Where to change
//
//   - messages.go: the wire messages, as cut-a §5.1 lists them.
//   - hub.go: sessions, memberships, devices and the group's state machine.
package together
