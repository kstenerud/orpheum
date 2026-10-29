// SPDX-License-Identifier: MPL-2.0

/**
 * Where a chunked playback resumes from: the one answer for every rebuild of its decoding and every
 * ask to load on (loadAgainFrom, loadOnAfterFailures, recoverFromDecodingErrors, a quality given up),
 * so none of them reads a position the rebuild itself reset.
 */

import type Hls from "hls.js";

/** What resuming reads of hls.js: whether it has loaded enough to start, from where, and its video. */
export type ResumingHls = Pick<Hls, "hasEnoughToStart" | "startPosition"> & {
  readonly media: Pick<HTMLMediaElement, "currentTime"> | null;
};

/**
 * The position each hls.js asked to load again from, from its rebuild until its new media source is
 * attached (loadAgainFrom): between the two, hls.js's own record still holds the position before.
 */
const rebuilding = new WeakMap<object, number>();

/** Marks hls's decoding as being rebuilt, to load again from positionS once attached. */
export function rebuildingFrom(hls: object, positionS: number): void {
  rebuilding.set(hls, positionS);
}

/** Marks hls's rebuild done: its new media source is attached, and hls.js holds the position. */
export function rebuilt(hls: object): void {
  rebuilding.delete(hls);
}

/**
 * The position to resume hls's playback from, in seconds: while its decoding is being rebuilt, the
 * position it was rebuilt from; then, until hls.js has loaded enough to start, the position hls.js was
 * asked to start from, which hls.js moves itself to any seek meanwhile and seeks the video to once it
 * has loaded there; after, where the video stands. A rebuild's new media source puts the video at 0,
 * and asking hls.js to load on from there restarted a film from its beginning after a minute's outage,
 * the top quality given up at 68 s (a local reproduction, headless Chrome, 2026-10-29); -1 with no
 * video and nothing asked.
 */
export function resumePositionOf(hls: ResumingHls): number {
  const rebuildingFromS = rebuilding.get(hls);
  if (rebuildingFromS !== undefined) {
    return rebuildingFromS;
  }
  if (!hls.hasEnoughToStart && hls.startPosition >= 0) {
    return hls.startPosition;
  }
  return hls.media?.currentTime ?? -1;
}
