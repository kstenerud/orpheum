// SPDX-License-Identifier: MPL-2.0

/**
 * What the chunked player's bitrate controller (abr.ts) reads of its video's position: how long it
 * has played, which ends a playback's starting (D352 §1), and where, asked to play, it has stood
 * still, which says a quality cannot be played here (D266).
 */

import { isLoadedWhereItStands, isWaitingToPlay } from "./abr-choice.ts";

/** How far, in seconds, a video's position must move to be playing, not standing still. */
const movedAtLeastS = 0.1;

/**
 * How much faster than the clock a video's position may move between two looks and still count as
 * played: more is a seek forward, not playing.
 */
const playsAtMostTimesTheClock = 2;

/** A video as the controller reads its position. */
type Watched = Pick<HTMLMediaElement, "buffered" | "currentTime" | "paused" | "readyState">;

/**
 * How long a video has played, in ms: its position moving forward, while not paused, no faster than
 * playsAtMostTimesTheClock (a seek forward is not played), counted at each look.
 */
export class PlayedTime {
  #playedMs = 0;
  #lastPosition: { readonly positionS: number; readonly atMs: number } | undefined;

  /** Counts what media played since the last look, at nowMs; a look with no video starts afresh. */
  look(media: Watched | null, nowMs: number): void {
    if (media === null) {
      this.#lastPosition = undefined;
      return;
    }
    const last = this.#lastPosition;
    if (last !== undefined && !media.paused) {
      const advanceMs = (media.currentTime - last.positionS) * 1000;
      if (advanceMs > 0 && advanceMs <= (nowMs - last.atMs) * playsAtMostTimesTheClock) {
        this.#playedMs += advanceMs;
      }
    }
    this.#lastPosition = { positionS: media.currentTime, atMs: nowMs };
  }

  /** How long the video has played, in ms. */
  playedMs(): number {
    return this.#playedMs;
  }
}

/**
 * Where a video, asked to play, has stood still: since when with what it plays there loaded
 * (undefined while not), and how many chunks came meanwhile.
 */
export interface StillAt {
  readonly positionS: number;
  readonly loadedSinceMs: number | undefined;
  readonly chunksLoaded: number;
  readonly chunksNotHeld: number;
}

/** Where a video, asked to play, has stood still, and what came meanwhile (D266). */
export class StandingStill {
  #still:
    | {
        positionS: number;
        loadedSinceMs: number | undefined;
        chunksLoaded: number;
        chunksNotHeld: number;
      }
    | undefined;

  /** A chunk of picture loaded while it may stand still. */
  chunkLoaded(): void {
    if (this.#still !== undefined) {
      this.#still.chunksLoaded += 1;
    }
  }

  /** The browser took a chunk of picture of the quality it plays and held nothing of it. */
  chunkNotHeld(): void {
    if (this.#still !== undefined) {
      this.#still.chunksNotHeld += 1;
    }
  }

  /** Starts afresh: its quality was given up, and it loads again. */
  forget(): void {
    this.#still = undefined;
  }

  /**
   * Where media, waiting to play, has stood still since the last look, at nowMs, kept up to date;
   * undefined when it is not waiting, or has just moved and starts standing still afresh.
   */
  look(media: Watched | null, nowMs: number): StillAt | undefined {
    if (media === null || !isWaitingToPlay(media)) {
      this.#still = undefined;
      return undefined;
    }
    const isLoaded = isLoadedWhereItStands(media);
    const still = this.#still;
    if (still === undefined || Math.abs(media.currentTime - still.positionS) > movedAtLeastS) {
      this.#still = {
        positionS: media.currentTime,
        loadedSinceMs: isLoaded ? nowMs : undefined,
        chunksLoaded: 0,
        chunksNotHeld: 0,
      };
      return undefined;
    }
    still.loadedSinceMs = isLoaded ? (still.loadedSinceMs ?? nowMs) : undefined;
    return still;
  }
}
