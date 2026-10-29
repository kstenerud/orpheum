// SPDX-License-Identifier: MPL-2.0

/**
 * The chunk of picture on its way, as the chunked player's bitrate controller (abr.ts) follows it to
 * judge whether to give it up (D265 §3): since when it is judged, how many of its bytes were the
 * core's 425 answers, which say it is being made and are none of it, and when its bytes last grew,
 * which says whether it was arriving or stuck (bandwidth-sampling-2026-10-29.md P3).
 */

import type { FollowedChunk } from "./abr-options.ts";

/** A chunk on its way as a look judges it. */
export interface JudgedChunk<Chunk extends FollowedChunk> {
  readonly chunk: Chunk;
  /** How long it has been loading, in ms, since it was asked for or the core began making it. */
  readonly loadingMs: number;
  /** When its first byte came, in ms from when it began; undefined while none of it has. */
  readonly firstByteMs: number | undefined;
  /** How many of its bytes have arrived, its 425 answers' none of them; 0 while none of it has. */
  readonly arrivedBytes: number;
  /** How many bytes hls.js counts loaded, the 425 answers' with them, for its note. */
  readonly loadedBytes: number;
  /** How long since its bytes last grew, in ms. */
  readonly sinceArrivedMs: number;
}

/** The chunk of picture on its way, followed from its ask to its load, or until given up. */
export class ChunkOnItsWay<Chunk extends FollowedChunk> {
  #following:
    | {
        readonly chunk: Chunk;
        readonly sinceMs: number;
        readonly answeredBytes: number;
        readonly isBeingMade: boolean;
        readonly arrived: { loadedBytes: number; atMs: number };
        isStuckNoted: boolean;
      }
    | undefined;

  /** chunk began loading at nowMs. */
  loading(chunk: Chunk, nowMs: number): void {
    this.#following = {
      chunk,
      sinceMs: nowMs,
      answeredBytes: 0,
      isBeingMade: false,
      arrived: { loadedBytes: 0, atMs: nowMs },
      isStuckNoted: false,
    };
  }

  /**
   * The core answered chunk with 425 at nowMs, being made: it is judged from now, and what it counts
   * loaded so far, the answers' bodies, is none of it.
   */
  beingMade(chunk: Chunk, nowMs: number): void {
    if (this.#following?.chunk === chunk) {
      const { loaded } = chunk.stats;
      this.#following = {
        chunk,
        sinceMs: nowMs,
        answeredBytes: loaded,
        isBeingMade: true,
        arrived: { loadedBytes: loaded, atMs: nowMs },
        isStuckNoted: false,
      };
    }
  }

  /** chunk loaded, or was given up: it is followed no more. */
  ended(chunk: Chunk): void {
    if (this.#following?.chunk === chunk) {
      this.#following = undefined;
    }
  }

  /** Whether chunk is to be noted stuck now: true once for each chunk followed. */
  isStuckToNote(chunk: Chunk): boolean {
    const following = this.#following;
    if (following?.chunk !== chunk || following.isStuckNoted) {
      return false;
    }
    following.isStuckNoted = true;
    return true;
  }

  /** Follows nothing more: the quality loading was given up. */
  forget(): void {
    this.#following = undefined;
  }

  /**
   * The chunk on its way as a look at nowMs judges it, its bytes' growth noted; undefined with none,
   * one hls.js aborted, or one the core is making with nothing of it arrived, which is waited for: a
   * lower quality would be made too, and slower, and an encoder that cannot keep up moves its
   * players down itself (D286 §4, D323 §4).
   */
  look(nowMs: number): JudgedChunk<Chunk> | undefined {
    const following = this.#following;
    if (following === undefined || following.chunk.stats.aborted) {
      return undefined;
    }
    const { chunk, answeredBytes, arrived } = following;
    const { loading, loaded } = chunk.stats;
    if (loaded > arrived.loadedBytes) {
      arrived.loadedBytes = loaded;
      arrived.atMs = nowMs;
    }
    // Only bytes past the core's 425 answers are the chunk arriving.
    const hasArrived = loaded > answeredBytes && loading.first > 0;
    if (following.isBeingMade && !hasArrived) {
      return undefined;
    }
    return {
      chunk,
      loadingMs: nowMs - following.sinceMs,
      firstByteMs: hasArrived ? loading.first - loading.start : undefined,
      arrivedBytes: hasArrived ? loaded - answeredBytes : 0,
      loadedBytes: loaded,
      sinceArrivedMs: nowMs - arrived.atMs,
    };
  }
}
