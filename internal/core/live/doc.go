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
//     each sender controls one receiver, a cast elsewhere leaving the one it controlled (D337 §6).
//     A device receiving a cast is no device's target until that cast ends, its own sender's
//     included, and the person's connections are told which devices receive (D386 §1). Enforced by
//     TestRegistry_ACastGoesOnlyToAnotherConnectedDeviceOfTheSamePerson,
//     TestRegistry_ASenderCastingElsewhereLeavesTheReceiverItControlled and
//     TestRegistry_ADeviceReceivingACastIsNoOnesTargetUntilItEnds.
//   - The hand-off reaches one connection of the receiver, its tab in front, the one that came to the
//     front last; a device with none in front is refused, and the person's connections are told which
//     devices have one (D338 §2). A connection in front once none holds the cast, loaded again or come
//     to the front, is handed it again, with a credential of its own (§4: a reload comes back
//     receiving). Enforced by TestRegistry_TheTabInFrontReceives,
//     TestRegistry_AReceiverLoadedAgainIsHandedItsCastAgain and, through the socket,
//     TestCast_GoesOnlyToADeviceWithATabInFront in internal/core/api.
//   - A cast is a session with the receiver (D364): it plays another film, or nothing between videos,
//     only at its sender's or its receiver's asking, in the same cast, its receiver handed it again;
//     only the receiver says Use, once; and the sender's every page is told the cast it sends, as a
//     page opens before its devices, and at each change and its end (plans/up-next.md).
//     Enforced by TestRegistry_ACastMovesOnToTheNextFilmAndItsReceiverIsHandedIt,
//     TestRegistry_EachFilmPlayedInACastIsHandedToItsReceiver and
//     TestRegistry_ACastIsASessionTheSenderIsToldOf.
//   - A cast ends once its receiver has had no connection for ReceiverGoneAfter, and never sooner;
//     the sender is told (D364 §6). Enforced by TestRegistry_ACastEndsOnceItsReceiverIsGoneForTwoMinutes.
//   - A receiver whose person is in a Watch Together session is handed a credential for it with each
//     cast, to take their part: their host's, to play it as their player (D364 §11), or their guest's,
//     to follow it as just another follower (D371 §4). Enforced by
//     TestRegistry_AReceiverIsHandedTheSessionItsPersonHosts and
//     TestRegistry_AGuestsReceiverIsHandedTheirPartToFollow. A cast whose sending page holds a guest's
//     membership (Cast.SendersMembership) takes that guest's part, the person's own device joined
//     as a guest of the session they host casting as its guest. Enforced by
//     TestRegistry_AReceiverTakesThePartItsSendersPageHolds and, through the API,
//     TestCast_AReceiverTakesTheGuestsPartTheSendingPageHolds in internal/core/api. A host who starts
//     Watch Together while they cast has one session, not two (D364 §11, D372 §8): the receiver
//     holding the cast is handed it again with their part in the new one and the film from where the
//     cast stands, to lead it.
//     Enforced by TestRegistry_StartingWatchTogetherWhileCastingHandsTheReceiverItsPart and, through
//     the API, TestCast_AHostStartingWatchTogetherWhileCastingHasTheReceiverLeadIt in internal/core/api.
//   - A device removed (D218 §4) is told its sign-in ended on each of its connections, and no other
//     device is, so its page goes to sign in at once. Enforced by
//     TestRegistry_ARemovedDeviceIsToldItsSignInEnded and, through the socket,
//     TestLive_ARemovedDeviceIsToldItsSignInEnded in internal/core/api.
//   - Starts no goroutine, and a Sender never blocks (the api's outbox). Review-only.
//   - Imports nothing from internal/node, never os/exec, and no database/sql. Enforced by the
//     depguard rules core-boundary and sql-only-in-store.
//
// # Read before changing
//
//   - D364: casting is a session with a device, ended only by Stop, take-back, its sender casting
//     elsewhere or the receiver gone; it revises D337, D338 and D358. D386 §1: no cast takes a
//     receiving device over.
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
