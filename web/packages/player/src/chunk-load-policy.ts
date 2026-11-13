// SPDX-License-Identifier: MPL-2.0

import type { LoaderResponse, LoadPolicy } from "hls.js";

import {
  noRoomToMakeStatus,
  tooEarlyStatus,
  goneStatus,
  unansweredStatus,
  unreachableStatus,
} from "./failure.ts";

/**
 * How long a chunk may take to begin arriving: longer than the core holds a request for one a run
 * is still making, at most 30 s (chunkHoldMs; by default it does not), so a chunk on its way is
 * waited for, not given up on (D247 §10).
 */
export const chunkFirstByteWaitMs = 30_000;

/**
 * How many times a chunk the core says is being made (425) is asked for again, a second or two
 * apart: over a minute and a half, longer than a player catching up waits for the run already
 * making its quality to reach where it jumped to, at most a minute of film ahead (D250 §4, §5).
 */
export const chunkAskAgainCount = 90;

/**
 * How many times in a row a chunk's load that nothing answered (unansweredStatus) is asked for again
 * while the browser is online, at the quality chosen as before, before hls.js takes it for its
 * quality's failure and loads a lower one: its connection was lost, not its quality, and asked again,
 * hls.js waiting errorRetry's 1 s and then 2 s, it goes on the connection that replaced it, which the
 * transport holds it for until one is up. Taken for its quality's failure, a chunk of the owner's
 * Firefox on Wi-Fi dropping out, lost with its WebRTC connection (replaced within half a second),
 * forced the quality from level 5 to 4 for a minute and left a 0.17 s hole, stepped over with a
 * 252 ms stand-still (staging, 2026-11-12). Offline, hls.js asks again itself once the browser is
 * back.
 */
export const unansweredAskAgainCount = 2;

/** The core's statuses after which a chunk is never asked for again, whatever hls.js would do. */
const neverAskedAgain: ReadonlySet<number> = new Set([
  noRoomToMakeStatus, // the core has no room to make it: asking again finds none (D250 §6)
  // Its film cannot be read now: the page says why and plays again once it can, where ninety asks
  // again spun the player for minutes, saying nothing (the owner's test, 2026-10-21).
  unreachableStatus,
  goneStatus, // the core no longer has the playback: the page opens it again (goneStatus)
]);

/**
 * Whether a chunk's load is tried again: as hls.js would, after the core's 425 too, and after a load
 * nothing answered for the first unansweredAskAgainCount times in a row, count being hls.js's own (the
 * playback's chunks failed since a chunk of their quality last buffered); never after the core's
 * 404, 429 or 503, which asking again does not change.
 */
// eslint-disable-next-line max-params -- hls.js's own signature for shouldRetry
function isChunkAskedAgain(
  _config: unknown,
  count: number,
  _timedOut: unknown,
  response: LoaderResponse | undefined,
  isRetry: boolean,
): boolean {
  const code = response?.code;
  if (code !== undefined && neverAskedAgain.has(code)) {
    return false;
  }
  if (code === unansweredStatus && count < unansweredAskAgainCount) {
    return true; // its connection was lost: asked again on the next, at its quality
  }
  return isRetry || code === tooEarlyStatus;
}

/**
 * How a chunked playback loads its chunks: waiting as long as the core may hold one being made,
 * and asking again after the core's 425, "being made", every second or two for chunkAskAgainCount
 * times, and after a load nothing answered unansweredAskAgainCount times in a row, at its quality
 * (isChunkAskedAgain); hls.js's own defaults give up after ten seconds, and never ask again after a
 * 425.
 */
export const chunkLoadPolicy: LoadPolicy = {
  default: {
    maxTimeToFirstByteMs: chunkFirstByteWaitMs,
    maxLoadTimeMs: 120_000,
    timeoutRetry: { maxNumRetry: 2, retryDelayMs: 0, maxRetryDelayMs: 0 },
    errorRetry: {
      maxNumRetry: chunkAskAgainCount,
      retryDelayMs: 1000,
      maxRetryDelayMs: 2000,
      shouldRetry: isChunkAskedAgain,
    },
  },
};
