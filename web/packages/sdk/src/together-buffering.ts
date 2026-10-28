// SPDX-License-Identifier: MPL-2.0

/**
 * What a following player says as it begins to hold its playing group up: a Fire TV reported
 * buffering some 350 ms after each nudge of its rate, its chunks loading fast, and nothing said what
 * it held or that its rate had just changed (the owner's cast, staging, 2026-10-28). Kept apart from
 * the follower, which sends it.
 */

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
