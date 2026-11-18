// SPDX-License-Identifier: MPL-2.0

/**
 * How a player follows its group: the options it is given, the events it hears and the numbers it
 * follows by. Kept apart from the follower (together-follow.ts), which reads them.
 */

import type { PlaybackPicture } from "./playback-picture.ts";
import type { PlaybackSound } from "./playback-sound.ts";
import type { PlaybackReport, PlaybackRights, SyncTuning } from "./together.ts";
import type { ShownPage } from "./shown-page.ts";

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

/**
 * How far off its group's position a player whose film plays on a wireless receiver, AirPlay's, is
 * left alone, nudged or sought by nothing (D522 §2): the receiver's position reaches the page late
 * and lands its seeks short, and each seek made Safari fire a pause the guest never pressed. A local
 * player is left within the tuning's leaveDriftUpToMs.
 */
export const receiverLeavesDriftUpToMs = 2000;

/**
 * The tuning a player follows its group by (D250 §1), where isOnReceiver says its film plays on a
 * wireless receiver: left alone within receiverLeavesDriftUpToMs, by nudges and seeks alike, and
 * sought only beyond it at least (D522 §2). Else the welcome's, as it is.
 */
export function followedTuning(tuning: SyncTuning, isOnReceiver: boolean): SyncTuning {
  if (!isOnReceiver) {
    return tuning;
  }
  const leftAloneMs = receiverLeavesDriftUpToMs;
  return {
    ...tuning,
    leaveDriftUpToMs: Math.max(tuning.leaveDriftUpToMs, leftAloneMs),
    seekOnlyLeaveDriftUpToMs: Math.max(tuning.seekOnlyLeaveDriftUpToMs, leftAloneMs),
    seekDriftBeyondMs: Math.max(tuning.seekDriftBeyondMs, leftAloneMs),
  };
}

/** How many of its player's last events a leading player's command note carries. */
export const recentEventsKept = 8;

/** A player's last recentEventsKept events, as "<event>[(own)]@<monotonic ms>", for a command's note. */
export class RecentEvents {
  #events: string[] = [];

  /** Keeps event, at nowMs, made by the follower itself where isOwn. */
  keep(event: string, isOwn: boolean, nowMs: number): void {
    const atMs = Math.round(nowMs);
    this.#events = [...this.#events, `${event}${isOwn ? "(own)" : ""}@${String(atMs)}`].slice(
      -recentEventsKept,
    );
  }

  /** The events kept, oldest first, apart by a space. */
  text(): string {
    return this.#events.join(" ");
  }
}

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
  /**
   * How the player's film sounds now, for the same roster, a cast's remote showing it and changing
   * it (playback-sound.ts); absent where nothing is to be said.
   */
  readonly readSound?: (() => PlaybackSound | undefined) | undefined;
  /**
   * What the player's film shows that may be chosen, its subtitles and the original, for the same
   * roster (playback-picture.ts); absent where nothing is to be said.
   */
  readonly readPicture?: (() => PlaybackPicture | undefined) | undefined;
  /**
   * Whether the player's film plays on a wireless receiver now, AirPlay's (the player's
   * isOnWirelessTarget): while it does, its drift is left alone further (followedTuning) and the
   * pause and play its element fires in answer to the follower's own moves are not its person's
   * (OwnMoves, D522 §2). Absent where it never does.
   */
  readonly isOnWirelessTarget?: (() => boolean) | undefined;
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
