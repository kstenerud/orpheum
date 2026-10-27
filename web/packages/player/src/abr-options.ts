// SPDX-License-Identifier: MPL-2.0

import type { HlsProgressivePerformanceTiming, LoaderStats } from "hls.js";

import type { ChoiceLevel } from "./abr-choice.ts";

/**
 * What the chunked player's bitrate controller (abr.ts) is given: the chunks it follows, what it reads
 * of hls.js, what its playback tells it, and what it is given of the link.
 */

/** A chunk as choosing follows it: hls.js's Fragment, or a test's. */
export interface FollowedChunk {
  readonly url: string;
  readonly level: number;
  readonly duration: number;
  readonly stats: Pick<LoaderStats, "loaded" | "total" | "aborted"> & {
    readonly loading: Pick<HlsProgressivePerformanceTiming, "start" | "first" | "end">;
  };
}

/** What choosing reads of hls.js. */
export interface ChoosingHls {
  readonly levels: readonly ChoiceLevel[];
  readonly currentLevel: number;
  readonly loadLevel: number;
  readonly minAutoLevel: number;
  readonly maxAutoLevel: number;
  readonly mainForwardBufferInfo: { readonly len: number } | null;
  /** The video it plays into, whose position shows whether it plays. */
  readonly media: Pick<
    HTMLMediaElement,
    "buffered" | "currentTime" | "paused" | "readyState"
  > | null;
}

/** What a playback tells its choosing. */
export interface ChoosingOptions {
  /** The highest BANDWIDTH it starts within (D247 §6, D260 §3). */
  readonly startAtBps: number;
  /** The share of a link a quality may take, as the offer says; linkShareForQuality when absent. */
  readonly shareForQuality?: number | undefined;
  /**
   * The steadiness level it starts at, as the offer says: the one its device last ended at on its
   * network (D352 Q2); 0 when absent.
   */
  readonly startSteadiness?: number | undefined;
  readonly monotonicMs: () => number;
  /** Whether this tab's voice is live, when the film gives way to it (D262 §2). */
  readonly isVoiceLive: () => boolean;
  /** Whether it plays in a Watch Together group, which keeps the film's own picture (D286 §2). */
  readonly isInAGroup?: () => boolean;
  /** Whether the browser took the chunk at url from its cache; undefined where it does not say. */
  readonly fromCache: (url: string) => boolean | undefined;
  /**
   * Told each change of the ceiling it chooses under, and why, for the diagnosis's steps: a player
   * that dropped from the twin to 360p after a jump on a 77 Mb/s link said nothing of why (the owner's
   * Firefox on nas.example, 2026-10-20).
   */
  readonly noteStep?: (step: string) => void;
  /**
   * Told the link's last samples in a line (samplesText) each time one is taken, for every stall's
   * and seek's report (D352 §5).
   */
  readonly noteSamples?: (samples: string) => void;
  /** Told the steadiness level as it starts and at each change, for every play report (D352 Q2). */
  readonly noteSteadiness?: (level: number) => void;
  /** Told of a quality given up as one this device cannot play, and where (D266). */
  readonly onUnplayable?: (unplayable: {
    readonly qualityKbps: number;
    readonly positionMs: number;
    readonly isFilmsOwnPicture: boolean;
  }) => void;
}

/**
 * What choosing is given of the link: how to give a chunk up, and hls.js's estimator to feed, which
 * its other controllers read; choosing chooses by its own estimate (D352 §2).
 */
export interface ChoosingLink<Chunk extends FollowedChunk> {
  readonly giveUp: (chunk: Chunk) => void;
  /**
   * Takes what a chunk given up showed of the link, bytes over durationMs, as a measure of it: the
   * chunk loads no more, and nothing else would measure a link that slowed while the old measure kept
   * a group's player on the film's own picture, given up again every few seconds (the owner's group
   * test, 2026-10-14). hls.js's own abandon resets its measures instead, which ours does not (D265).
   */
  readonly measureGivenUp: (durationMs: number, bytes: number) => void;
  /**
   * Builds the video's decoding anew and loads again from positionS, where the video stood, at the
   * quality chosen next.
   */
  readonly loadAgain: (positionS: number) => void;
}
