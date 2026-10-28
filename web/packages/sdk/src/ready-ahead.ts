// SPDX-License-Identifier: MPL-2.0

/**
 * How much film a player must have loaded ahead of where it is before it plays, at its start and
 * after a seek: enough to outlast loading its next chunk, with a margin (the owner, 2026-10-28):
 * margin × the next chunk's expected load time, its size over the link its player measures, as its
 * player tells it (tellNextChunkLoadMs); refillAheadMs where its player can say nothing, before any
 * chunk loaded, or playing a file as it is. One rule for every player: a follower after a seek
 * (together-refill.ts), and one alone, a cast's receiver or a session's leader at its start and
 * after a seek (start-gate.ts). A Fire TV resumed with 1.4 s loaded ahead, its next chunk 10.4 s of
 * film taking 2.3 s, and ran dry (the owner's cast, staging, 2026-10-28).
 */

import { refillAheadMs } from "./together-media.ts";

/**
 * The margin on the next chunk's expected load time a player waits to have loaded ahead, in
 * percent: ready_ahead_of_next_chunk_percent's starting value, which the page sets from the
 * server's (setReadyAheadPercent); a page with no client settings, a guest's, keeps this.
 */
export const readyAheadOfNextChunkPercent = 150;

/** The margin in force on this page, in percent. */
let marginPercent = readyAheadOfNextChunkPercent;

/** Sets the margin from the server's setting, ready_ahead_of_next_chunk_percent; none below 100. */
export function setReadyAheadPercent(percent: number): void {
  if (Number.isFinite(percent) && percent >= 100) {
    marginPercent = percent;
  }
}

/** Each element's word on how long its next chunk is expected to take to load, from its player. */
const nextChunkLoads = new WeakMap<object, () => number | undefined>();

/**
 * Has media's player tell, by read, how long its next chunk is expected to take to load, in ms, or
 * undefined where it cannot say yet. Returns the end of it.
 */
export function tellNextChunkLoadMs(media: object, read: () => number | undefined): () => void {
  nextChunkLoads.set(media, read);
  return () => {
    if (nextChunkLoads.get(media) === read) {
      nextChunkLoads.delete(media);
    }
  };
}

/** What a player is ready to play on: the film loaded ahead, and the next chunk's load it outlasts. */
export interface ReadyAhead {
  /** The film loaded ahead it is ready on, in ms. */
  readonly aheadMs: number;
  /** How long its next chunk is expected to take to load, in ms; absent where its player cannot say. */
  readonly nextChunkLoadMs?: number;
}

/** What media is ready to play on now, by the rule above. */
export function readyAheadOf(media: object): ReadyAhead {
  const loadMs = nextChunkLoads.get(media)?.();
  if (loadMs === undefined || !Number.isFinite(loadMs) || loadMs < 0) {
    return { aheadMs: refillAheadMs };
  }
  return {
    aheadMs: Math.round((loadMs * marginPercent) / 100),
    nextChunkLoadMs: Math.round(loadMs),
  };
}
