// SPDX-License-Identifier: MPL-2.0

/**
 * How fast a following player loads, as film loaded per second, and how far ahead of its group a
 * player catching up alone aims by it (D250 §4). Kept apart from the follower, which asks it.
 */

import type { SyncTuning } from "./together.ts";
import {
  isLoadedThrough,
  loadedEndMs,
  positionMsOf,
  type FollowedMedia,
} from "./together-media.ts";
import type { CatchUpAimedBy } from "./together-notes.ts";
import type { ChunkLoaded } from "./together-player-events.ts";
import { readyAheadOf } from "./ready-ahead.ts";

/**
 * The furthest a catch-up seek aims ahead of the group: by the time a seek has loaded the group has
 * moved on, so it aims ahead by how long the last one took to load, up to this (D247 §4,
 * Silo-Server's number).
 */
export const aimAheadAtMostMs = 10_000;

/**
 * How far ahead of the group a catch-up seek aims where the player has loaded the film there
 * already: a seek within what it holds is ready in a moment. Two phones paused with the group and
 * locked came back after it had played on, and each jumped 11 to 12 s ahead to wait, by how long
 * its last seek had taken to load, though they held that stretch already (the owner's test,
 * 2026-10-12).
 */
export const loadedCatchUpLeadMs = 1000;

/** Where a catch-up seek aims: ahead of its group by aheadForLoadMs, by what aimedBy says. */
export interface CatchUpAim {
  readonly targetMs: number;
  readonly aheadForLoadMs: number;
  readonly aimedBy: CatchUpAimedBy;
}

/**
 * Where a catch-up seek to the group at toMs aims: loadedCatchUpLeadMs ahead where the player has
 * loaded the film from there through as much beyond as it is ready on after a seek (readyAheadOf),
 * and lead() would aim further, else lead() ahead.
 */
export function catchUpTargetMs(
  media: FollowedMedia,
  group: { readonly toMs: number; readonly rate: number },
  lead: () => CatchUpLead,
): CatchUpAim {
  const aimed = lead();
  const nearMs = loadedCatchUpLeadMs * group.rate;
  const nearAtMs = group.toMs + nearMs;
  if (
    aimed.leadMs > nearMs &&
    isLoadedThrough(media, nearAtMs, nearAtMs + readyAheadOf(media).aheadMs)
  ) {
    return { targetMs: nearAtMs, aheadForLoadMs: nearMs, aimedBy: "loaded" };
  }
  return { targetMs: group.toMs + aimed.leadMs, aheadForLoadMs: aimed.leadMs, aimedBy: aimed.by };
}

/**
 * How much a player loads ahead of where it is, and then stops: hls.js's maxBufferLength (30 s),
 * which nothing here changes, and the most an iPhone's ManagedMediaSource lets it load
 * (playback-in-practice.md §1). A player catching up aims to have the tuning's percent of it loaded
 * when the group arrives (D250 §4).
 */
export const playerLoadsAheadMs = 30_000;

/**
 * The furthest a player catching up alone jumps ahead of the group to wait for it there: a person
 * waits paused for at most about a minute of film, however slow their link (D250 §4).
 */
export const waitAheadAtMostMs = 60_000;

/**
 * How the speed a player loads at is measured, as film loaded per second, where its player tells
 * none of its chunks (a video playing its HLS itself): over the last loadWindowMs, from at least
 * loadSpanAtLeastMs of samples, and only while it is loading, its buffer short of what it fills to
 * by loadSampleMarginMs, since a full buffer loads nothing and would read as a link that stopped
 * (D247 §4, amended 2026-10-06).
 */
export const loadWindowMs = 10_000;
export const loadSpanAtLeastMs = 3000;
export const loadSampleMarginMs = 5000;

/**
 * How many of the chunks of picture its player loaded last measure the speed a player loads at,
 * where its player tells them: one alone says little, a slow one among a few is what the link
 * does, and they are recent enough to be the link it has now.
 */
export const chunksMeasuredAtMost = 3;

/**
 * A chunk loaded faster than this, in ms, came from the browser's cache, not over the link, and
 * measures nothing: the bitrate controller's own guess where the browser says nothing of it (D247
 * §9; the player's cacheHitUnderMs).
 */
export const fromCacheUnderMs = 10;

/** How far ahead of the group a catch-up seek aims, and what that rests on. */
export interface CatchUpLead {
  readonly leadMs: number;
  readonly by: Exclude<CatchUpAimedBy, "loaded">;
}

/** How fast a player loads, from the chunks its player loaded, else from how far it has loaded. */
export class LoadSpeed {
  /** The end of what it loaded, sampled while it loads: when, and to where, in ms. */
  #samples: { readonly atMs: number; readonly endMs: number }[] = [];
  /** Film loaded per second of the link, as its samples last measured; undefined until they have. */
  #sampledRatio: number | undefined = undefined;
  /** Its last chunksMeasuredAtMost chunks of picture loaded over the link, as its player told them. */
  #chunks: { readonly filmMs: number; readonly loadMs: number }[] = [];

  /** A seek: what was loaded before it says nothing of loading from where it went. */
  readonly forget = (): void => {
    this.#samples = [];
  };

  /**
   * Its player loaded a chunk: one of picture over the link measures how fast the link carries the
   * film, from when it was asked for to its last byte. A load that failed is no chunk loaded, so a
   * link that was down measures nothing: its samples of how far it had loaded, standing still
   * through a minute without Wi-Fi, read as a link that loads nothing, and the owner's Chrome, back,
   * jumped a minute ahead of its group (staging, 2026-10-29; VQ133).
   */
  readonly chunkLoaded = (loaded: ChunkLoaded): void => {
    if (loaded.track === "video" && loaded.loadMs >= fromCacheUnderMs && loaded.filmMs > 0) {
      this.#chunks = [
        ...this.#chunks.slice(1 - chunksMeasuredAtMost),
        { filmMs: loaded.filmMs, loadMs: loaded.loadMs },
      ];
    }
  };

  /**
   * Samples how far the player has loaded, and from the samples of the last loadWindowMs measures
   * how much film it loads per second, for a player that tells none of its chunks. A sample is taken
   * only while it loads short of a full buffer; a seek, or loading that stops, starts the samples
   * again.
   */
  readonly sample = (media: FollowedMedia, nowMs: number): void => {
    const endMs = loadedEndMs(media);
    const isLoading =
      endMs !== undefined &&
      endMs < media.duration * 1000 &&
      endMs - positionMsOf(media) < playerLoadsAheadMs - loadSampleMarginMs;
    const last = this.#samples.at(-1);
    if (!isLoading || (last !== undefined && endMs < last.endMs)) {
      this.#samples = [];
      return;
    }
    this.#samples = [
      ...this.#samples.filter((sample) => nowMs - sample.atMs <= loadWindowMs),
      { atMs: nowMs, endMs },
    ];
    const first = this.#samples[0];
    if (first !== undefined && nowMs - first.atMs >= loadSpanAtLeastMs) {
      this.#sampledRatio = (endMs - first.endMs) / (nowMs - first.atMs);
    }
  };

  /**
   * Film loaded per second of the link: over its last chunks of picture where its player tells them,
   * else as its samples measured; undefined with neither.
   */
  ratio(): number | undefined {
    if (this.#chunks.length === 0) {
      return this.#sampledRatio;
    }
    const filmMs = this.#chunks.reduce((sum, chunk) => sum + chunk.filmMs, 0);
    const loadMs = this.#chunks.reduce((sum, chunk) => sum + chunk.loadMs, 0);
    return filmMs / loadMs;
  }

  /**
   * How far ahead of the group a catch-up seek aims: by how long its last seek took to load,
   * lastLoadMs, up to aimAheadAtMostMs, and then by how long its link takes, as measured, to load the
   * tuning's catchUpBufferPercent of what its player loads ahead, so the group finds its buffer that
   * full (D250 §4); at most waitAheadAtMostMs, and never less than the last load's time alone, which
   * is all it aims by with no measure yet. The last load's own bound holds in the sum too: a seek
   * whose load waited out a minute without a link took a minute, which is no time to load.
   */
  readonly catchUpLeadMs = (
    lastLoadMs: number,
    groupRate: number,
    tuning: SyncTuning,
  ): CatchUpLead => {
    const loadMs = Math.min(lastLoadMs, aimAheadAtMostMs);
    const aimMs = loadMs * groupRate;
    const ratio = this.ratio();
    if (ratio === undefined) {
      return { leadMs: aimMs, by: "lastLoad" };
    }
    const fillMs = (playerLoadsAheadMs * tuning.catchUpBufferPercent) / 100;
    const fillWallMs = ratio > 0 ? fillMs / ratio : Number.POSITIVE_INFINITY;
    const leadMs = (loadMs + fillWallMs) * groupRate;
    if (leadMs >= waitAheadAtMostMs) {
      return { leadMs: Math.max(aimMs, waitAheadAtMostMs), by: "mostAhead" };
    }
    return leadMs > aimMs ? { leadMs, by: "fill" } : { leadMs: aimMs, by: "lastLoad" };
  };
}
