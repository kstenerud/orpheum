// SPDX-License-Identifier: MPL-2.0

/**
 * The rungs the core offers for a chunked playback's film, as its master playlist offers them, the
 * film's own picture among them (D491), for the device's data cap to be chosen from (D488): one
 * entry per rung, never a list of its own, as the
 * ladder is the owner's setting (chunkQualities) and a film below a rung is offered fewer.
 */

import Hls from "hls.js";

import type { ChoiceLevel } from "./abr-choice.ts";

/** The event a chunked playback's video is told its film's rungs with, as offeredRungsOf says them. */
export const playerOfferedRungsEvent = "orpheumplayerofferedrungs";

/**
 * The rates of the rungs among levels, kbit/s, each its average with its sound's, lowest first: the
 * qualities the core made and the film's own picture, one more rung a cap may play (D491).
 */
export function rungsOf(levels: readonly ChoiceLevel[]): readonly number[] {
  return levels
    .map((level) => Math.round(level.averageBitrate / 1000))
    .sort((one, other) => one - other);
}

/** Each video's film's rungs, while a chunked playback plays in it. */
const offered = new WeakMap<EventTarget, readonly number[]>();

/** The rungs of the film video plays on its ladder (rungsOf); undefined before its playlist is read, or off a ladder. */
export function offeredRungsOf(video: EventTarget): readonly number[] | undefined {
  return offered.get(video);
}

/** What telling the rungs needs of hls.js. */
interface RungsHls {
  readonly levels: readonly ChoiceLevel[];
  on: (event: typeof Hls.Events.MANIFEST_PARSED, listener: () => void) => void;
}

/**
 * Keeps and tells video the rungs of its film once hls's master playlist is read
 * (playerOfferedRungsEvent); returns the stopping, which forgets them.
 */
export function tellOfferedRungs(hls: RungsHls, video: EventTarget): () => void {
  hls.on(Hls.Events.MANIFEST_PARSED, () => {
    const rungs = rungsOf(hls.levels);
    offered.set(video, rungs);
    video.dispatchEvent(new CustomEvent(playerOfferedRungsEvent, { detail: rungs }));
  });
  return () => {
    offered.delete(video);
  };
}
