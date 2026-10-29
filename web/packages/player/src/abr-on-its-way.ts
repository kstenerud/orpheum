// SPDX-License-Identifier: MPL-2.0

/**
 * The chunk of picture on its way, as the chunked player's bitrate controller (abr.ts) follows it to
 * judge whether to give it up (D265 §3): since when it is judged, how many of its bytes were the
 * core's 425 answers, which say it is being made and are none of it, and when its bytes last grew,
 * which says whether it was arriving or stuck (bandwidth-sampling-2026-10-29.md P3).
 */

import { asksAgainWaitMs, givenUpText, isStuck, isToAskAgain } from "./abr-choice.ts";
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
  /** Whether the core answered it, its head come, rather than its ask still waiting. */
  readonly isAnswered: boolean;
}

/** The playback asking again: after how long, and which time in a row since a chunk loaded, from 1. */
export interface AskingAgain {
  readonly waitMs: number;
  readonly inARow: number;
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

  /**
   * How many times in a row the playback asked again, since a chunk last loaded: for a chunk stuck on
   * its way (stuck), or after hls.js stopped loading for a load that failed (askingAgain).
   */
  #asksAgain = 0;

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

  /**
   * The playback asks again, this time in a row (from 1), after waitMs: asksAgainWaitMs of the times
   * before, so a chunk stuck and a load hls.js stopped wait alike, each in a row twice as long.
   */
  askingAgain(): AskingAgain {
    const waitMs = asksAgainWaitMs(this.#asksAgain);
    this.#asksAgain += 1;
    return { waitMs, inARow: this.#asksAgain };
  }

  /** chunk loaded: followed no more, and the next one stuck is asked for again after the first wait. */
  loaded(chunk: Chunk): void {
    this.#asksAgain = 0;
    this.ended(chunk);
  }

  /**
   * chunk's load failed or timed out, hls.js's details say which: followed no more until hls.js asks
   * for it again, and its note, with carried, what carried it, which says where the transport ended
   * it itself, so a loader waiting on a dead connection shows in the core's log.
   */
  failed(chunk: Chunk, details: string, carried: string): string {
    this.ended(chunk);
    return `a chunk of ${String(chunk.level)} failed to load (${details}); ${carried}`;
  }

  /**
   * A chunk judged on its way and not given up, as one of the lowest quality is not, when it is stuck:
   * its note, with carried(), read before it is aborted, and whether it is to be asked for again
   * (isToAskAgain), with which time in a row it is and how long it waited, followed no more; else,
   * once for each chunk, that it is stuck (isStuck). Undefined when it is neither. Waited for, a chunk
   * stuck after 19 bytes over WebRTC held the owner's Chrome for good (staging, 2026-10-29).
   */
  stuck(
    judged: JudgedChunk<Chunk>,
    carried: () => string,
  ): { readonly note: string; readonly isAskedAgain: boolean } | undefined {
    const { chunk } = judged;
    const came = givenUpText({ ...judged, isSample: false });
    if (isToAskAgain(judged, this.#asksAgain)) {
      const { waitMs, inARow } = this.askingAgain();
      this.ended(chunk);
      const note =
        `asked again for a chunk of ${String(chunk.level)}, time ${String(inARow)} in a` +
        ` row, after ${String(waitMs)} ms with nothing: ${came}; ${carried()}`;
      return { note, isAskedAgain: true };
    }
    if (isStuck(judged) && this.isStuckToNote(chunk)) {
      return { note: `chunk of ${String(chunk.level)} ${came}; ${carried()}`, isAskedAgain: false };
    }
    return undefined;
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

  /**
   * Whether the chunk on its way is one the core answered 425, being made, with nothing of it
   * arrived since: its wait is the core's, not the link's.
   */
  isAwaitingTheCore(): boolean {
    const following = this.#following;
    return (
      following !== undefined &&
      !following.chunk.stats.aborted &&
      following.isBeingMade &&
      !hasArrived(following.chunk, following.answeredBytes)
    );
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
    const isArriving = hasArrived(chunk, answeredBytes);
    if (following.isBeingMade && !isArriving) {
      return undefined;
    }
    return {
      chunk,
      loadingMs: nowMs - following.sinceMs,
      firstByteMs: isArriving ? loading.first - loading.start : undefined,
      arrivedBytes: isArriving ? loaded - answeredBytes : 0,
      loadedBytes: loaded,
      sinceArrivedMs: nowMs - arrived.atMs,
      isAnswered: loading.first > 0,
    };
  }
}

/** Whether any of chunk has arrived: only bytes past the core's 425 answers, answeredBytes, are it. */
function hasArrived(chunk: FollowedChunk, answeredBytes: number): boolean {
  const { loading, loaded } = chunk.stats;
  return loaded > answeredBytes && loading.first > 0;
}
