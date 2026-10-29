// SPDX-License-Identifier: MPL-2.0

/**
 * Who a session's roster names: the devices that play beside this one, the one that leads, and
 * whether this one leads or follows (D371 §1), read from its connection's snapshot.
 */

import type { RosterDevice } from "./together.ts";
import type { TogetherSnapshot } from "./together-connection.ts";

/** The devices in a snapshot's roster that play, other than this one: none a panel. */
export function otherPlayersOf(snapshot: TogetherSnapshot): RosterDevice[] {
  return snapshot.roster.flatMap((member) =>
    member.devices.filter((device) => device.id !== snapshot.deviceId && device.role !== "panel"),
  );
}

/**
 * The device that leads a snapshot's session, the screen that is the film (D371 §1), by its roster;
 * undefined while none does.
 */
export function leaderOf(snapshot: Pick<TogetherSnapshot, "roster">): RosterDevice | undefined {
  return snapshot.roster
    .flatMap((member) => member.devices)
    .find((device) => device.isLeader === true);
}

/**
 * Whether this device leads its session, by a roster that says so (D371 §1): the leader plays as a
 * device plays a film alone, and is never held, sought to the group's place, run up or nudged.
 */
export function isLeaderOf(snapshot: TogetherSnapshot): boolean {
  const leader = leaderOf(snapshot);
  return leader !== undefined && leader.id === snapshot.deviceId;
}

/**
 * Whether this device follows its session: welcomed, with a roster that names another device its
 * leader, or none. Before its welcome a device is neither leader nor follower: a cast's receiver
 * whose start gate took it for a follower then played at once, with nothing loaded, and led its
 * session so (the owner's cast to a Fire TV, staging, 2026-10-28).
 */
export function isFollowerOf(snapshot: TogetherSnapshot): boolean {
  return snapshot.deviceId !== undefined && !isLeaderOf(snapshot);
}

/**
 * Whether this device follows a group that plays, or waits for its players to: its player, held
 * paused while it loads, is asked to play, and its slow load is a stall (D247 §2); one a paused
 * group holds is not.
 */
export function isAskedByItsGroup(snapshot: TogetherSnapshot): boolean {
  const state = snapshot.group?.state;
  return isFollowerOf(snapshot) && (state === "Playing" || state === "Waiting");
}

/**
 * Whether this device is the host's, following while a guest's device leads: the host's screen come
 * back, which takes the lead back once it plays in step, as the server judges by its states (D372
 * §3).
 */
export function isHostAwaitingLead(snapshot: TogetherSnapshot): boolean {
  const leader = leaderOf(snapshot);
  if (!snapshot.isHost || leader === undefined || leader.id === snapshot.deviceId) {
    return false;
  }
  return !snapshot.roster.some(
    (member) => member.isHost && member.devices.some((device) => device.id === leader.id),
  );
}
