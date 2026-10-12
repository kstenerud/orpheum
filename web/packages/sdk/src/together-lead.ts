// SPDX-License-Identifier: MPL-2.0

/**
 * A following player as its group sees it now, and what a leading player's own events are: the
 * commands its person gives by the browser's controls, and whether its link's rights let it. Kept
 * apart from the follower, which asks it, so the follower fits in one read.
 */

import type { GroupView, PlaybackRights, SyncTuning } from "./together.ts";
import type { GroupCommand } from "./together-connection.ts";
import { driftCorrection } from "./together-drift.ts";
import { positionMsOf, type FollowedMedia } from "./together-media.ts";

/** The group a player follows, where it is expected now, and when now is. */
export interface GroupNow {
  readonly group: GroupView;
  readonly expectedMs: number;
  readonly nowMs: number;
  /** When a playing group starts moving, on the monotonic clock (D268 §3). */
  readonly startsAtMs: number;
  /** How to follow it, as the welcome said (D250 §1). */
  readonly tuning: SyncTuning;
}

/** The command a leading player's own event is, if any: its person's play, pause or seek. */
export function commandOfEvent(
  event: string,
  media: FollowedMedia,
  where: GroupNow,
): GroupCommand | undefined {
  const { state } = where.group;
  if (event === "play" && state === "Paused") {
    return { command: "play" };
  }
  if (state === "Playing" && (event === "ended" || (event === "pause" && !media.ended))) {
    return { command: "pause" };
  }
  // A jump is past the seek threshold: the browser's player moves itself a fraction of a second to
  // step over a gap in what it loaded, and each such step, taken for its person's seek, made the
  // whole group wait and load again, without end on a slow link (the owner's test, 2026-10-05).
  const isAJump =
    driftCorrection(
      { expectedMs: where.expectedMs, actualMs: positionMsOf(media), groupRate: where.group.rate },
      where.tuning,
    ).kind === "seek";
  // A seek is the group's as its person makes it, not once it has loaded: a host on a slow link
  // would otherwise hold the group up loading a place the group was never told of, and be left
  // behind (the owner's test, 2026-10-05).
  return event === "seeking" && isAJump
    ? { command: "seek", positionMs: positionMsOf(media) }
    : undefined;
}

/** Whether rights let a player send command: play and pause by one, a seek by the other. */
export function mayLead(command: GroupCommand, rights: PlaybackRights): boolean {
  switch (command.command) {
    case "play":
    case "pause":
      return rights.canPlayPause;
    case "seek":
      return rights.canSeek;
    case "changeItem":
    case "stop":
      return false;
  }
}
