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
 * Whether the player has loaded enough ahead to play on after a stall or a seek: aheadMs, or
 * up to the film's end, or all the browser will load while it is paused, since the browser has
 * stopped loading and waiting longer gains nothing.
 */
export function hasRefilled(media: FollowedMedia, aheadMs = refillAheadMs): boolean {
  if (media.readyState < haveFutureData) {
    return false;
  }
  if (media.networkState === networkIdle) {
    return true;
  }
  const endS = loadedEndS(media);
  return (
    endS !== undefined && ((endS - media.currentTime) * 1000 >= aheadMs || endS >= media.duration)
  );
}

/**
 * How far before what it loaded a player's position may be and still be on it, in seconds: a
 * player held at its start through playableAt can read a hair before it. One scan, hasRefilled's,
 * once lacked it, and a player so held kept saying it buffered (the M4 closing audit, 2026-10-08).
 */
const onLoadedToleranceS = 0.001;

/**
 * The widest hole in what a player loaded that it plays across as if there were none, in seconds:
 * hls.js's own `maxBufferHole` (0.1 s by default, which the player keeps), the hole its gap
 * controller steps over as the video plays. A paused video steps over nothing (idiosyncrasies.md, "A
 * paused player is never stepped over a gap"), so a hole of 60 ms between two sound renditions'
 * chunks, 1.5 s past where a cast's receiver was held for its group's wait, ended what it had
 * loaded there: it never had enough ahead to be ready, and its group waited on it for good (the
 * owner's cast to Firefox on a Mac, staging, 2026-10-28).
 */
export const playedAcrossHoleS = 0.1;

/**
 * The end, in seconds, of what the player loaded where it is, read on across holes no wider than
 * playedAcrossHoleS; undefined where it loaded nothing.
 */
function loadedEndS(media: FollowedMedia): number | undefined {
  const { buffered, currentTime } = media;
  let endS: number | undefined = undefined;
  for (let index = 0; index < buffered.length; index++) {
    const startS = buffered.start(index);
    const isOn =
      endS === undefined
        ? startS - onLoadedToleranceS <= currentTime && currentTime <= buffered.end(index)
        : startS - endS <= playedAcrossHoleS;
    if (isOn) {
      endS = buffered.end(index);
    } else if (endS !== undefined) {
      break;
    }
  }
  return endS;
}

/** Whether the player has loaded the film from fromMs through throughMs, in one stretch. */
export function isLoadedThrough(media: FollowedMedia, fromMs: number, throughMs: number): boolean {
  const { buffered } = media;
  for (let index = 0; index < buffered.length; index++) {
    if (buffered.start(index) * 1000 <= fromMs && throughMs <= buffered.end(index) * 1000) {
      return true;
    }
  }
  return false;
}

/** The end, in ms, of what the player loaded where it is, or undefined where it loaded nothing. */
export function loadedEndMs(media: FollowedMedia): number | undefined {
  const endS = loadedEndS(media);
  return endS === undefined ? undefined : endS * 1000;
}

/** Whether the player's position is on something it loaded, to within a millisecond. */
export function isOnLoaded(media: FollowedMedia): boolean {
  return loadedEndS(media) !== undefined;
}

/**
 * Where a paused player can be held at expectedMs: there, or, when that falls in a gap just
 * before what it loaded, within the seek threshold, the start of what it loaded. A copied picture
 * is cut at its source's keyframes, so its first loaded frame can come a moment after the group's
 * position; a paused player never steps over that gap as a playing one does, is never ready, and
 * held the group up for good (the owner's test, 2026-10-06, a copied HEVC picture on a Mac). Past
 * the film's end, as a group's clock runs on a moment after it, its end: the browser keeps a seek
 * past it there, and each was followed by another (the browser tier, 2026-10-18).
 */
export function playableAt(media: FollowedMedia, expectedMs: number, tuning: SyncTuning): number {
  if (Number.isFinite(media.duration) && expectedMs > media.duration * 1000) {
    return media.duration * 1000;
  }
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

/**
 * Where what the player loaded at atMs begins, in ms: the start of the loaded stretch holding it, or
 * atMs where it loaded nothing there. A run-up starts no earlier, so it never asks for a chunk before
 * the one its group starts in, which on an encoded quality is a whole encode (D282;
 * seek-resync-2026-10-12.md).
 */
export function loadedFromMs(media: FollowedMedia, atMs: number): number {
  const { buffered } = media;
  const atS = atMs / 1000;
  for (let index = 0; index < buffered.length; index++) {
    if (buffered.start(index) - onLoadedToleranceS <= atS && atS <= buffered.end(index)) {
      return buffered.start(index) * 1000;
    }
  }
  return atMs;
}
