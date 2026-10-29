// SPDX-License-Identifier: MPL-2.0

/**
 * What the chunked player's bitrate controller (abr.ts) reads of its video's position: how long it
 * has played, which ends a playback's starting (D352 §1), and where, asked to play, it has stood
 * still, which says a quality cannot be played here (D266).
 */

import {
  isUnplayable,
  isWaitingToPlay,
  loadedAheadOf,
  standsStillAtMostMs,
  standsWithAheadAtLeastS,
} from "./abr-choice.ts";

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
 * Where a video, asked to play, has stood still: since when, since when with what it plays loaded
 * ahead there (undefined while not), how many chunks came meanwhile, and what last showed its loads
 * not arriving.
 */
export interface StillAt {
  readonly positionS: number;
  readonly sinceMs: number;
  readonly loadedSinceMs: number | undefined;
  readonly chunksLoaded: number;
  readonly chunksNotHeld: number;
  /** How far ahead it holds what it plays there, picture and sound (loadedAheadOf), in seconds. */
  readonly loadedAheadS: number;
  /** What last showed its loads not arriving while it stood still (linkFailed); undefined for none. */
  readonly linkWhy: string | undefined;
}

/** What standing still says of the quality loading: whether it is unplayable here, and a note why. */
export interface StillJudged {
  readonly isUnplayable: boolean;
  /** Why it was judged unplayable, or, once a standing still, why not; undefined for nothing to say. */
  readonly note: string | undefined;
}

/**
 * Where a video, asked to play, has stood still, and what came meanwhile (D266); its standing still
 * the decoder's only while what it plays is loaded ahead and its loads arrive, never the link's.
 */
export class StandingStill {
  #still: ({ -readonly [Fact in keyof StillAt]: StillAt[Fact] } & { isNoted: boolean }) | undefined;

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

  /**
   * Its loads did not arrive, for why (a chunk failed, stuck or given up, its loading stopped, the
   * browser offline): its standing still so far is the link's, so its time and its chunks count
   * afresh from the next look; what the browser took and held nothing of is the decoder's, and kept.
   * A minute's outage stood a video still at the top quality, which was given up as unplayable and
   * never chosen again (a local reproduction, 2026-10-29).
   */
  linkFailed(why: string): void {
    if (this.#still !== undefined) {
      this.#still.loadedSinceMs = undefined;
      this.#still.chunksLoaded = 0;
      this.#still.linkWhy = why;
    }
  }

  /** Starts afresh: its quality was given up, and it loads again. */
  forget(): void {
    this.#still = undefined;
  }

  /**
   * Where media, waiting to play, has stood still since the last look, at nowMs, kept up to date;
   * undefined when it is not waiting, or has just moved and starts standing still afresh. Its time
   * counts only while it holds standsWithAheadAtLeastS ahead.
   */
  look(media: Watched | null, nowMs: number): StillAt | undefined {
    if (media === null || !isWaitingToPlay(media)) {
      this.#still = undefined;
      return undefined;
    }
    const loadedAheadS = loadedAheadOf(media);
    const isLoaded = loadedAheadS >= standsWithAheadAtLeastS;
    const still = this.#still;
    if (still === undefined || Math.abs(media.currentTime - still.positionS) > movedAtLeastS) {
      this.#still = {
        positionS: media.currentTime,
        sinceMs: nowMs,
        loadedSinceMs: isLoaded ? nowMs : undefined,
        chunksLoaded: 0,
        chunksNotHeld: 0,
        loadedAheadS,
        linkWhy: undefined,
        isNoted: false,
      };
      return undefined;
    }
    still.loadedSinceMs = isLoaded ? (still.loadedSinceMs ?? nowMs) : undefined;
    still.loadedAheadS = loadedAheadS;
    return still;
  }

  /**
   * Whether the quality loading is unplayable here by the standing still the last look found, at
   * nowMs (isUnplayable), with a note why; or, once a standing still has lasted standsStillAtMostMs
   * without being so, a note why not: what showed its loads not arriving, too little loaded ahead,
   * or too few chunks arrived. It holds the film ahead as far as both hls.js's picture,
   * pictureAheadS, and the video, picture and sound, hold it.
   */
  judge(nowMs: number, pictureAheadS: number): StillJudged {
    const still = this.#still;
    if (still === undefined) {
      return { isUnplayable: false, note: undefined };
    }
    const forMs = still.loadedSinceMs === undefined ? 0 : nowMs - still.loadedSinceMs;
    const loadedAheadS = Math.min(pictureAheadS, still.loadedAheadS);
    const facts =
      `stood still at ${still.positionS.toFixed(1)} s for ${String(Math.round(nowMs - still.sinceMs))} ms, ` +
      `${String(Math.round(forMs))} ms of it with ${standsWithAheadAtLeastS.toFixed(0)} s loaded ahead; ` +
      `${still.loadedAheadS.toFixed(1)} s ahead, ${pictureAheadS.toFixed(1)} s of picture, ` +
      `${String(still.chunksLoaded)} chunks arrived, ${String(still.chunksNotHeld)} not held`;
    const { chunksLoaded, chunksNotHeld } = still;
    if (isUnplayable({ forMs, chunksLoaded, chunksNotHeld, loadedAheadS })) {
      return { isUnplayable: true, note: `judged unplayable: ${facts}` };
    }
    if (still.isNoted || nowMs - still.sinceMs < standsStillAtMostMs) {
      return { isUnplayable: false, note: undefined };
    }
    still.isNoted = true;
    return { isUnplayable: false, note: `not judged unplayable, ${whyNot(still)}: ${facts}` };
  }
}

/** Why a standing still did not make its quality unplayable: its loads, its buffer or its chunks. */
function whyNot(still: StillAt): string {
  if (still.linkWhy !== undefined) {
    return `its loads not arriving: ${still.linkWhy}`;
  }
  return still.loadedSinceMs === undefined
    ? "too little loaded ahead, the link's to bring"
    : "too few chunks arrived";
}
