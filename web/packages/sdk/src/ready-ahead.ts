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

import type { components } from "./gen/api.ts";
import { refillAheadMs } from "./together-media.ts";

/**
 * What a next chunk's expected load rests on, as the play report says it: its player's estimate of
 * the link from its own samples, the remembered link blended in ("confident"), the link its device
 * last measured on its network alone, before any sample ("remembered"); "none" where it waits
 * refillAheadMs, with neither (bandwidth-sampling-2026-10-29.md P5). "firstSample" is a player's of
 * before 2026-10-29, which no player sends now.
 */
export type NextChunkLoadFrom = NonNullable<components["schemas"]["SeekBody"]["nextChunkLoadFrom"]>;

/**
 * How long a next chunk is expected to take to load, in ms, and what that rests on; and, where its
 * player says them, how much film it plays, whether it is of the lowest quality the player may
 * load, and the most the player loads ahead, which a refill after a stall reads (refillAfterStallOf).
 */
export interface NextChunkLoad {
  readonly loadMs: number;
  readonly from: Exclude<NextChunkLoadFrom, "none">;
  /** How much film the chunk plays, in ms. */
  readonly filmMs?: number;
  /** Whether the chunk is of the lowest quality its player may load: no lower one can keep up. */
  readonly isLowestQuality?: boolean;
  /** The most film its player loads ahead, in ms: a refill waiting for more would never end. */
  readonly loadsAheadAtMostMs?: number;
}

/**
 * The margin on the next chunk's expected load time a player waits to have loaded ahead, in
 * percent: readyAheadOfNextChunkPercent's starting value, which the page sets from the
 * server's (setReadyAheadPercent); a page with no client settings, a guest's, keeps this.
 */
export const readyAheadOfNextChunkPercent = 150;

/** The margin in force on this page, in percent. */
let marginPercent = readyAheadOfNextChunkPercent;

/** Sets the margin from the server's setting, readyAheadOfNextChunkPercent; none below 100. */
export function setReadyAheadPercent(percent: number): void {
  if (Number.isFinite(percent) && percent >= 100) {
    marginPercent = percent;
  }
}

/** Each element's word on how long its next chunk is expected to take to load, from its player. */
const nextChunkLoads = new WeakMap<object, () => NextChunkLoad | undefined>();

/**
 * Has media's player tell, by read, how long its next chunk is expected to take to load and what
 * that rests on, or undefined where it cannot say yet. Returns the end of it.
 */
export function tellNextChunkLoadMs(
  media: object,
  read: () => NextChunkLoad | undefined,
): () => void {
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
  /** What that rests on; "none" where its player cannot say, and it waits refillAheadMs. */
  readonly nextChunkLoadFrom: NextChunkLoadFrom;
}

/** What media is ready to play on now, by the rule above. */
export function readyAheadOf(media: object): ReadyAhead {
  const told = nextChunkLoads.get(media)?.();
  if (told === undefined || !Number.isFinite(told.loadMs) || told.loadMs < 0) {
    return { aheadMs: refillAheadMs, nextChunkLoadFrom: "none" };
  }
  return {
    aheadMs: Math.round((told.loadMs * marginPercent) / 100),
    nextChunkLoadMs: Math.round(told.loadMs),
    nextChunkLoadFrom: told.from,
  };
}

/**
 * How long a run of play a refill after a stall buys, in ms, where the link is slower than the
 * lowest quality: a fixed 10 s refill on such a link plays 10 s and stalls again, without end
 * (staging, 2026-10-28: device 64 stalled at 2405, 2415, 2425 and 2435 s of its film), and people
 * mind several short stalls more than one long one (starts-chunks-remote-2026-10-29.md §1.3; D414
 * §2). A minute: low confidence, the research's, until the lab's run at 0.8× the lowest quality.
 */
export const runAfterStallMs = 60_000;

/**
 * What media is ready to play on after a stall (D414 §2): the seek's rule (readyAheadOf) where its
 * link keeps up with its next chunk, or where a lower quality may still; where it does not at the
 * lowest quality, enough for a run of runAfterStallMs while the link falls behind it, runAfterStallMs
 * × (1 − the link's share of the chunk's rate), never past the most its player loads ahead.
 */
export function refillAfterStallOf(media: object): ReadyAhead {
  const seek = readyAheadOf(media);
  const told = nextChunkLoads.get(media)?.();
  if (
    seek.nextChunkLoadFrom === "none" ||
    told?.filmMs === undefined ||
    told.isLowestQuality !== true ||
    told.loadMs <= told.filmMs
  ) {
    return seek;
  }
  const linkShare = told.filmMs / told.loadMs;
  const runMs = Math.round(runAfterStallMs * (1 - linkShare));
  return {
    ...seek,
    aheadMs: Math.min(Math.max(seek.aheadMs, runMs), told.loadsAheadAtMostMs ?? Infinity),
  };
}
