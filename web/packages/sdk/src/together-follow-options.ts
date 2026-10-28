// SPDX-License-Identifier: MPL-2.0

/**
 * How a player follows its group: the options it is given, the events it hears and the numbers it
 * follows by. Kept apart from the follower (together-follow.ts), which reads them.
 */

import type { PlaybackReport, PlaybackRights } from "./together.ts";
import type { ShownPage } from "./together-lead.ts";
import type { StartLagMemory } from "./together-start.ts";

/**
 * How often a following player checks its drift while the group plays, and reports its state while
 * the group waits.
 */
export const checkDriftEveryMs = 1000;

/**
 * How many of its rate's returns to the group's a player learns from before it may correct by
 * seeks alone: one return's loss may be a stall's, and a Chrome taken for an iPhone would hold its
 * group less tightly for nothing (D268 §2).
 */
export const returnsBeforeSeeksOnly = 2;

/** How many of its player's last events a leading player's command note carries. */
export const recentEventsKept = 8;

/** How a player follows its group. */
export interface FollowOptions {
  /** The film this player plays: a group playing another is not followed. */
  readonly fileId: number;
  /**
   * Which of its person's moves are the group's: the host's every one, a guest's those their link
   * gives (D271 §4). A move it may not make is undone, the player put back where the group is.
   */
  readonly leads: PlaybackRights;
  readonly monotonicMs: () => number;
  /** Where the device keeps how late its player starts; for the follower's life when not given (D282). */
  readonly startLag?: StartLagMemory;
  /**
   * The page the player is on where hiding it pauses its video, a handheld's: while hidden, nothing
   * moves the player. Absent on a computer, whose hidden page plays on and follows (heldByOf).
   */
  readonly page?: ShownPage | undefined;
  /**
   * How the player is given its film, for its session's roster, a cast's remote showing it (D371
   * §4): the file as it is, or converted, its ladder; absent where nothing is to be said.
   */
  readonly delivery?: PlaybackReport["delivery"] | undefined;
}

/** The events a following player tells the group of, and is moved by. */
export const followedEvents = [
  "loadedmetadata",
  "canplay",
  "playing",
  "waiting",
  "play",
  "pause",
  "seeking",
  "seeked",
  "ended",
] as const;
