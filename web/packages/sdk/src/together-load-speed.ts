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
import { seekReadyAheadMs } from "./together-refill.ts";

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

/**
 * Where a catch-up seek to the group at toMs aims: loadedCatchUpLeadMs ahead where the player has
 * loaded the film from there through seekReadyAheadMs beyond and leadMs() would aim further, else
 * leadMs() ahead.
 */
export function catchUpTargetMs(
  media: FollowedMedia,
  group: { readonly toMs: number; readonly rate: number },
  leadMs: () => number,
): number {
  const aimedMs = group.toMs + leadMs();
  const nearMs = group.toMs + loadedCatchUpLeadMs * group.rate;
  return isLoadedThrough(media, nearMs, nearMs + seekReadyAheadMs)
    ? Math.min(aimedMs, nearMs)
    : aimedMs;
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
 * How the speed a player loads at is measured, as film loaded per second: over the last
 * loadWindowMs, from at least loadSpanAtLeastMs of samples, and only while it is loading, its buffer
 * short of what it fills to by loadSampleMarginMs, since a full buffer loads nothing and would read
 * as a link that stopped (D247 §4, amended 2026-10-06).
 */
export const loadWindowMs = 10_000;
export const loadSpanAtLeastMs = 3000;
export const loadSampleMarginMs = 5000;

/** How fast a player loads, from samples of how far it has loaded. */
export class LoadSpeed {
  /** The end of what it loaded, sampled while it loads: when, and to where, in ms. */
  #samples: { readonly atMs: number; readonly endMs: number }[] = [];
  /** Film loaded per second of the link, as last measured; undefined until it has been. */
  #ratio: number | undefined = undefined;

  /** A seek: what was loaded before it says nothing of loading from where it went. */
  readonly forget = (): void => {
    this.#samples = [];
  };

  /**
   * Samples how far the player has loaded, and from the samples of the last loadWindowMs measures
   * how much film it loads per second. A sample is taken only while it loads short of a full
   * buffer; a seek, or loading that stops, starts the samples again.
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
      this.#ratio = (endMs - first.endMs) / (nowMs - first.atMs);
    }
  };

  /**
   * How far ahead of the group a catch-up seek aims: by how long its last seek took to load,
   * lastLoadMs, and then by how long its link takes, as measured, to load the tuning's
   * catchUpBufferPercent of what its player loads ahead, so the group finds its buffer that full
   * (D250 §4); at most waitAheadAtMostMs, and never less than the last load's time alone, which is
   * all it aims by with no measure yet.
   */
  readonly catchUpLeadMs = (lastLoadMs: number, groupRate: number, tuning: SyncTuning): number => {
    const aimMs = Math.min(lastLoadMs, aimAheadAtMostMs) * groupRate;
    const ratio = this.#ratio;
    if (ratio === undefined) {
      return aimMs;
    }
    const fillMs = (playerLoadsAheadMs * tuning.catchUpBufferPercent) / 100;
    const fillWallMs = ratio > 0 ? fillMs / ratio : Number.POSITIVE_INFINITY;
    return Math.max(aimMs, Math.min(waitAheadAtMostMs, (lastLoadMs + fillWallMs) * groupRate));
  };
}
