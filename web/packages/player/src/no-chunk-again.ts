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
 * The chunk to load in place of chosen: chosen, unless it is a chunk of picture buffered whole at
 * another quality (isBufferedWhole), when the one after it on chosen's playlist, and so on; null
 * where every chunk after is held.
 */
export function chunkNotHeld<Chunk extends ListedChunk>(
  chosen: Chunk | null,
  levels: readonly ListedLevel[],
  playlist: readonly Chunk[],
  isBufferedWhole: (chunk: ListedChunk) => boolean,
): Chunk | null {
  const isHeldElsewhere = (sn: number, level: number): boolean =>
    levels.some((each, other) => {
      const details = each.details;
      const chunk = details?.fragments[sn - details.startSN];
      return other !== level && chunk?.sn === sn && isBufferedWhole(chunk);
    });
  let chunk = chosen;
  while (chunk !== null && typeof chunk.sn === "number" && isHeldElsewhere(chunk.sn, chunk.level)) {
    const after = chunk.sn + 1;
    chunk = playlist.find((each) => each.sn === after) ?? null;
  }
  return chunk;
}

/**
 * Wraps hls's main stream controller's choice of the next chunk (its getNextFragment, kept by name
 * in its build) so it never loads again a chunk of picture buffered at another quality
 * (chunkNotHeld), by its fragment tracker's word, which forgets a chunk the browser evicted or
 * hls.js flushed. hls.js renamed or reshaped is an error: its pinned version is the one tested.
 */
export function loadsNoChunkAgain(hls: Hls): void {
  const controller: unknown = Reflect.get(hls, "streamController");
  const choose: unknown =
    typeof controller === "object" && controller !== null
      ? Reflect.get(controller, "getNextFragment")
      : undefined;
  const tracker: unknown =
    typeof controller === "object" && controller !== null
      ? Reflect.get(controller, "fragmentTracker")
      : undefined;
  const getState: unknown =
    typeof tracker === "object" && tracker !== null ? Reflect.get(tracker, "getState") : undefined;
  if (typeof choose !== "function" || typeof getState !== "function") {
    throw new Error(
      "hls.js has no stream controller's getNextFragment and fragment tracker to wrap",
    );
  }
  Reflect.set(
    controller as object,
    "getNextFragment",
    (position: number, details: { readonly fragments: readonly ListedChunk[] }) => {
      const chosen = Reflect.apply(choose, controller, [position, details]) as ListedChunk | null;
      return chunkNotHeld(
        chosen,
        hls.levels,
        details.fragments,
        (chunk) => Reflect.apply(getState, tracker, [chunk]) === bufferedWhole,
      );
    },
  );
}
