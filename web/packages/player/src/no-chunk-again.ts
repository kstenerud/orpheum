// SPDX-License-Identifier: MPL-2.0

/**
 * A switch of quality, up as well as down, never loads again a chunk of picture already buffered
 * (D503 §4). hls.js 1.7.3 keeps the chunks it buffered by quality, and takes the next chunk after
 * the one it holds only on the quality it last loaded: on a switch, the chunk the picture's buffer
 * ends in counts as not loaded at the new quality, however near its end the buffer reaches, and is
 * loaded again over the picture held. A made quality's chunk 40, its picture 14 ms short of its
 * playlist's end, was loaded again at the copy, 22.8 MB, and the room it took stalled the next
 * (file 1504 on the Fire TV, 2026-11-15; idiosyncrasies.md). Its stream controller's choice of the
 * next chunk is so wrapped: a chunk it chose that it holds buffered at another quality gives the
 * one after it, at the quality it loads.
 */

import type Hls from "hls.js";

/** A chunk as hls.js lists it: its number on its quality's playlist, and the quality. */
interface ListedChunk {
  readonly sn: number | "initSegment";
  readonly level: number;
}

/** A quality's playlist, as hls.js keeps it once loaded. */
interface ListedLevel {
  readonly details?:
    { readonly startSN: number; readonly fragments: readonly ListedChunk[] } | undefined;
}

/** hls.js's fragment tracker's state of a chunk buffered whole (its FragmentState.OK, a const enum). */
const bufferedWhole = "OK";

/**
 * Whether a chunk, by its number and quality, is one of picture buffered whole at another of levels
 * (isBufferedWhole).
 */
function heldElsewhere(
  levels: readonly ListedLevel[],
  isBufferedWhole: (chunk: ListedChunk) => boolean,
): (sn: number, level: number) => boolean {
  return (sn, level) =>
    levels.some((each, other) => {
      const details = each.details;
      const chunk = details?.fragments[sn - details.startSN];
      return other !== level && chunk?.sn === sn && isBufferedWhole(chunk);
    });
}

/**
 * The chunk to load in place of chosen: chosen, unless it is one held at another quality
 * (isHeldElsewhere, heldElsewhere), when the one after it on chosen's playlist, and so on; null where
 * every chunk after is held.
 */
export function chunkNotHeld(
  chosen: ListedChunk | null,
  playlist: readonly ListedChunk[],
  isHeldElsewhere: (sn: number, level: number) => boolean,
): ListedChunk | null {
  let chunk = chosen;
  while (chunk !== null && typeof chunk.sn === "number" && isHeldElsewhere(chunk.sn, chunk.level)) {
    const after = chunk.sn + 1;
    chunk = playlist.find((each) => each.sn === after) ?? null;
  }
  return chunk;
}

/** owner's field of that name, or undefined where owner is no object. */
function fieldOf(owner: unknown, name: string): unknown {
  if (typeof owner !== "object" || owner === null) {
    return undefined;
  }
  const value: unknown = Reflect.get(owner, name);
  return value;
}

/** Whether value is a chunk as hls.js lists it. */
function isListedChunk(value: unknown): value is ListedChunk {
  return typeof fieldOf(value, "level") === "number" && fieldOf(value, "sn") !== undefined;
}

/**
 * Wraps hls's main stream controller's choice of the next chunk (its getNextFragment, kept by name
 * in its build) so it never loads again a chunk of picture buffered at another quality
 * (chunkNotHeld), by its fragment tracker's word, which forgets a chunk the browser evicted or
 * hls.js flushed. hls.js renamed or reshaped is an error: its pinned version is the one tested.
 */
export function loadsNoChunkAgain(hls: Hls): void {
  const controller: unknown = fieldOf(hls, "streamController");
  const choose = fieldOf(controller, "getNextFragment");
  const tracker = fieldOf(controller, "fragmentTracker");
  const getState = fieldOf(tracker, "getState");
  if (
    typeof controller !== "object" ||
    controller === null ||
    typeof choose !== "function" ||
    typeof getState !== "function"
  ) {
    throw new Error(
      "hls.js has no stream controller's getNextFragment and fragment tracker to wrap",
    );
  }
  const isBufferedWhole = (chunk: ListedChunk): boolean =>
    Reflect.apply(getState, tracker, [chunk]) === bufferedWhole;
  Reflect.set(
    controller,
    "getNextFragment",
    (position: number, details: { readonly fragments: readonly ListedChunk[] }) => {
      const chosen: unknown = Reflect.apply(choose, controller, [position, details]);
      return isListedChunk(chosen)
        ? chunkNotHeld(chosen, details.fragments, heldElsewhere(hls.levels, isBufferedWhole))
        : chosen;
    },
  );
}
