// SPDX-License-Identifier: MPL-2.0

/**
 * What a following player's media holds, as together-follow.ts reads it: where it is, what it
 * loaded, and whether that is enough to play on.
 */

import type { SyncTuning } from "./together.ts";

/**
 * After a stall or a seek, a player is ready again only with this much of the film loaded ahead of
 * it. Ready at its first frame, a player whose link is slower than the film plays that frame and
 * stalls again, and a group that waits for it plays and freezes in turn without end (the owner's
 * first test, 2026-10-05). Alone, the browser waits for more itself; in a group, this is that wait.
 */
export const refillAheadMs = 10_000;

/** The part of a video or audio element that following a group uses. */
export type FollowedMedia = Pick<
  HTMLMediaElement,
  | "currentTime"
  | "muted"
  | "playbackRate"
  | "preservesPitch"
  | "paused"
  | "ended"
  | "seeking"
  | "readyState"
  | "networkState"
  | "buffered"
  | "duration"
  | "play"
  | "pause"
  | "addEventListener"
  | "removeEventListener"
  | "dispatchEvent"
>;

/**
 * HTMLMediaElement.HAVE_METADATA, HAVE_FUTURE_DATA and NETWORK_IDLE, which a plain page may not
 * define.
 */
export const haveMetadata = 1;
export const haveFutureData = 3;
export const networkIdle = 1;

/** The player's position, in ms. */
export function positionMsOf(media: FollowedMedia): number {
  return media.currentTime * 1000;
}

/** Whether the player is where the group is, near enough to play along without a seek. */
export function isInStep(media: FollowedMedia, expectedMs: number, tuning: SyncTuning): boolean {
  return (
    media.readyState >= haveFutureData &&
    !media.seeking &&
    Math.abs(positionMsOf(media) - expectedMs) <= tuning.seekDriftBeyondMs
  );
}

/**
 * Whether the player has loaded enough ahead to play on after a stall or a seek: refillAheadMs, or
 * up to the film's end, or all the browser will load while it is paused, since the browser has
 * stopped loading and waiting longer gains nothing.
 */
export function hasRefilled(media: FollowedMedia): boolean {
  if (media.readyState < haveFutureData) {
    return false;
  }
  if (media.networkState === networkIdle) {
    return true;
  }
  const { buffered, currentTime } = media;
  for (let index = 0; index < buffered.length; index++) {
    const endS = buffered.end(index);
    if (buffered.start(index) <= currentTime && currentTime <= endS) {
      return (endS - currentTime) * 1000 >= refillAheadMs || endS >= media.duration;
    }
  }
  return false;
}

/** The end, in ms, of what the player loaded where it is, or undefined where it loaded nothing. */
export function loadedEndMs(media: FollowedMedia): number | undefined {
  const { buffered, currentTime } = media;
  for (let index = 0; index < buffered.length; index++) {
    if (buffered.start(index) - 0.001 <= currentTime && currentTime <= buffered.end(index)) {
      return buffered.end(index) * 1000;
    }
  }
  return undefined;
}

/** Whether the player's position is on something it loaded, to within a millisecond. */
export function isOnLoaded(media: FollowedMedia): boolean {
  const { buffered, currentTime } = media;
  for (let index = 0; index < buffered.length; index++) {
    if (buffered.start(index) - 0.001 <= currentTime && currentTime <= buffered.end(index)) {
      return true;
    }
  }
  return false;
}

/**
 * Where a paused player can be held at expectedMs: there, or, when that falls in a gap just
 * before what it loaded, within the seek threshold, the start of what it loaded. A copied picture
 * is cut at its source's keyframes, so its first loaded frame can come a moment after the group's
 * position; a paused player never steps over that gap as a playing one does, is never ready, and
 * held the group up for good (the owner's test, 2026-10-06, a copied HEVC picture on a Mac).
 */
export function playableAt(media: FollowedMedia, expectedMs: number, tuning: SyncTuning): number {
  const { buffered } = media;
  const expectedS = expectedMs / 1000;
  for (let index = 0; index < buffered.length; index++) {
    const startS = buffered.start(index);
    if (startS <= expectedS && expectedS <= buffered.end(index)) {
      return expectedMs;
    }
    if (startS > expectedS && (startS - expectedS) * 1000 <= tuning.seekDriftBeyondMs) {
      return startS * 1000;
    }
  }
  return expectedMs;
}
