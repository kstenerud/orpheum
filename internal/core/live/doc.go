// SPDX-License-Identifier: AGPL-3.0-or-later

// Package live is which of a person's signed-in devices are open and connected now, on any page,
// and the casts between them (D337; plans/cast-to-device.md §3, §4, §6): each device's live
// connection (the presence socket, internal/core/api's live_socket.go) is registered here, the
// person's every connection is told when the set of their connected devices, or of those with a tab
// in front, changes, and a cast's hand-off reaches the receiver's tab in front (D338 §2), again after
// the page that held it is loaded again. A cast's control is not here: it travels through Watch
// Together's hub, a cast being a session of one (D88; internal/core/together's cast.go). Everything
// lives in memory and ends with the process; this package holds no connection.
//
// # Invariants
//
//   - A device is listed while any connection of it is open, and only to its own person. Enforced
//     by TestRegistry_ListsAPersonsConnectedDevicesToThemAlone and, through the socket,
//     TestLive_ADeviceIsListedWhileItsSocketIsOpenAndGoneAfterTheSilenceLimit in internal/core/api.
//     A sign-in of no device (one from before D217 §8, which no sign-in makes now) is refused at the
//     socket, never registered: review-only.
//   - A cast goes only to another of the asker's own devices that is connected now (D337 §1, §2);
//     each receiver has one cast, the last one made, and each sender controls one receiver (D337 §6).
//     Enforced by TestRegistry_ACastGoesOnlyToAnotherConnectedDeviceOfTheSamePerson and
//     TestRegistry_TheLastCastWins.
//   - The hand-off reaches one connection of the receiver, its tab in front, the one that came to the
//     front last; a device with none in front is refused, and the person's connections are told which
//     devices have one (D338 §2). A connection in front once none holds the cast, loaded again or come
//     to the front, is handed it again, with a credential of its own (§4: a reload comes back
//     receiving). Enforced by TestRegistry_TheTabInFrontReceives,
//     TestRegistry_AReceiverLoadedAgainIsHandedItsCastAgain and, through the socket,
//     TestCast_GoesOnlyToADeviceWithATabInFront in internal/core/api.
//   - A cast moves on to another film, the next episode, only at its sender's or its receiver's
//     asking, in the same cast, and its receiver is handed it again (plans/up-next.md). Enforced by
//     TestRegistry_ACastMovesOnToTheNextFilmAndItsReceiverIsHandedIt.
//   - Starts no goroutine, and a Sender never blocks (the api's outbox). Review-only.
//   - Imports nothing from internal/node, never os/exec, and no database/sql. Enforced by the
//     depguard rules core-boundary and sql-only-in-store.
//
// # Read before changing
//
//   - D337: casting to one's own signed-in devices, and its §9 answers (plans/cast-to-device.md);
//     D338, the owner's changes after the first build: the tab in front receives.
//     at a film's end, the next episode as the sender would play it (plans/up-next.md).
//   - D88: a cast is Watch Together with a group of one, so control is the hub's.
//   - D154: no credential in a URL; the hello carries the session token.
//   - D165: an offline device must not rewind a person, so the sender leaves its playback.
//
// # Where to change
//
//   - registry.go: the connections, the devices listed and the casts.
//   - messages.go: what a live connection is sent and sends.
package live
