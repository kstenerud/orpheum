// SPDX-License-Identifier: GPL-3.0-or-later

// Package together is Watch Together's sessions (cut-a §5.1): who is in each, what their devices
// report, the group's state, and the messages each device is sent. Sessions live in memory; a
// restart ends them, and their members join again by the link. A session is the object and what
// plays is state on it (D122); its link is the grant (D117), a membership is what persists past a
// rotated link (D119), and a guest is not a principal (D125). The WebSocket and the endpoints are
// internal/core/api's (together.go, together_socket.go); this package holds no connection.
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
//   - The group plays once every player is ready, and waits while any player buffers or is gone.
//     Enforced by TestHub_TheGroupPlaysWhenEveryPlayerIsReadyAndWaitsForOneThatBuffers and
//     TestHub_ADroppedPlayerMakesTheGroupWaitUntilItIsBack.
//   - What the abilities keep for the host only the host does, and a seek made against a group
//     another seek has changed is refused as stale. Enforced by
//     TestHub_OnlyTheHostDoesWhatTheAbilitiesKeepForTheHost and
//     TestHub_ASeekAgainstAnotherSeekIsRefusedAsStale.
//   - A link admits up to the cap; a rotated link admits no one and keeps everyone in; an ejected
//     member's credential admits nothing. Enforced by
//     TestHub_TheLinkAdmitsToTheCapRotatesAndAnEjectedMemberIsGone and, through the API,
//     TestTogether_OnlyAHostStartsRotatesAndEnds.
//   - The host changes what plays only to a film their role reaches (D243 §4). Enforced by
//     TestHub_TheHostChangesWhatPlaysOnlyToAFilmTheyMayPlay; starting a session, by
//     TestReach_ALibraryTakenBackIsGoneAtTheNextRequestEverywhere.
//   - The roster shows the round trip each device last reported; only the device measures it
//     (D66). Enforced by TestHub_TheRosterShowsEachDevicesRoundTripItReported.
//   - Voice signalling goes only to the device it names. Enforced by
//     TestHub_RelaysVoiceSignallingToTheDeviceItNames.
//
// # Read before changing
//
//   - D62: Watch Together is first-class; the client surface is six methods.
//   - D66: no clock synchronisation.
//   - D117, D119, D121, D122, D125: the session, its link, its abilities and its guests.
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
