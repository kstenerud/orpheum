// SPDX-License-Identifier: MPL-2.0

/**
 * What a following player says as it begins to hold its playing group up: a Fire TV reported
 * buffering some 350 ms after each nudge of its rate, its chunks loading fast, and nothing said what
 * it held or that its rate had just changed (the owner's cast, staging, 2026-10-28). Kept apart from
 * the follower, which sends it.
 */

import type { TogetherSnapshot } from "./together-connection.ts";
import type { BufferingCause, PlayerNote } from "./together-notes.ts";
import { loadedEndMs, positionMsOf, type FollowedMedia } from "./together-media.ts";
import type { Refill } from "./together-refill.ts";

/**
 * What makes a player hold its group up, as the follower reads it: not yet once where the group is,
 * seeking, a refill by what began it, or too little to play on.
 */
export function bufferingCauseOf(
  media: FollowedMedia,
  hasCaughtUp: boolean,
  refill: Pick<Refill, "isShort" | "startedBy">,
): BufferingCause {
  if (!hasCaughtUp) {
    return "notCaughtUp";
  }
  if (media.seeking) {
    return "seeking";
  }
  return refill.isShort(media) ? refill.startedBy : "notReady";
}

/**
 * When a following player notes what holds its group up: as it begins to while the group plays, and
 * while the group waits for it, once for each wait and each change of what holds it. A cast's
 * receiver held its group's wait for good, saying each second that it buffered, its film loaded and
 * readyState 4, and nothing said why (the owner's cast to Firefox, staging, 2026-10-28).
 */
export class HoldingNotes {
  #wasHolding: boolean | undefined = undefined;
  /** The wait last noted, as "<group seq>:<why>"; undefined while it holds no wait up. */
  #notedWait: string | undefined = undefined;

  /**
   * Whether to note why the player holds its group up, as its state is told, with snapshot as it
   * is; why is undefined while it holds nothing up.
   */
  isToNote(
    why: BufferingCause | undefined,
    snapshot: Pick<TogetherSnapshot, "group" | "groupSeq">,
  ): boolean {
    const wasHolding = this.#wasHolding;
    this.#wasHolding = why !== undefined;
    if (why === undefined || snapshot.group?.state !== "Waiting") {
      this.#notedWait = undefined;
      return why !== undefined && wasHolding === false && snapshot.group?.state === "Playing";
    }
    const wait = `${String(snapshot.groupSeq)}:${why}`;
    const isNew = wait !== this.#notedWait;
    this.#notedWait = wait;
    return isNew;
  }
}

/**
 * The note of a player beginning to buffer, why, its follower having last changed its rate
 * sinceRateChangeMs ago, or never.
 */
export function bufferingNote(
  media: FollowedMedia,
  why: BufferingCause,
  sinceRateChangeMs: number | undefined,
): PlayerNote {
  const positionMs = positionMsOf(media);
  return {
    kind: "buffering",
    why,
    positionMs: Math.round(positionMs),
    readyState: media.readyState,
    rate: media.playbackRate,
    loadedAheadMs: Math.round(Math.max(0, (loadedEndMs(media) ?? positionMs) - positionMs)),
    ...(sinceRateChangeMs === undefined
      ? {}
      : { sinceRateChangeMs: Math.round(sinceRateChangeMs) }),
  };
}
