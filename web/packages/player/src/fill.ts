// SPDX-License-Identifier: MPL-2.0

import Hls, { PlaylistLevelType, type Fragment } from "hls.js";

/** A quality as stepping down while filling reads it: its average bit rate, its sound's included. */
interface FillLevel {
  readonly averageBitrate: number;
}

/**
 * The share of a link's measured speed a quality may take: the core chooses a quality by the same
 * share (D247 §6, its linkShareForQuality).
 */
const linkShareForQuality = 0.7;

/** How often a filling player looks at how fast its chunk is arriving. */
export const lookWhileFillingEveryMs = 500;

/** What a filling player knows about the chunk it is loading. */
export interface Filling {
  readonly levels: readonly FillLevel[];
  /** The quality the chunk is of. */
  readonly loadingLevel: number;
  /** How long the chunk plays for, in seconds. */
  readonly durationS: number;
  /** How long it has been loading, and how many of its bytes have arrived. */
  readonly loadingMs: number;
  readonly loadedBytes: number;
}

/**
 * The quality a player filling its first buffer moves down to, or undefined to go on (D260): once
 * its chunk has loaded for half the time it plays, as hls.js waits before giving up on one, a chunk
 * arriving slower than it plays is given up, for the highest quality within linkShareForQuality of
 * the speed it arrived at, or the lowest. hls.js gives up on a slow chunk only while the video
 * plays, so a film's page filling its hidden player, and a player waiting for its first frame,
 * waited out a whole chunk of the film's own picture: half a minute on a link of 1 Mb/s, and longer
 * on a phone at a conference (staging, 2026-10-07).
 */
export function fillStepDown(filling: Filling): number | undefined {
  const { levels, loadingLevel, durationS, loadingMs, loadedBytes } = filling;
  const loading = levels[loadingLevel];
  if (loading === undefined || loadingLevel === 0 || loadingMs < (durationS * 1000) / 2) {
    return undefined;
  }
  const arrivingBps = (loadedBytes * 8 * 1000) / loadingMs;
  if (arrivingBps >= loading.averageBitrate) {
    return undefined;
  }
  const within = levels.findLastIndex(
    (level, index) =>
      index < loadingLevel && level.averageBitrate <= linkShareForQuality * arrivingBps,
  );
  return Math.max(within, 0);
}

/** What stepping down while filling needs of hls.js. */
export interface FillingHls {
  readonly levels: readonly FillLevel[];
  readonly startPosition: number;
  autoLevelCapping: number;
  nextAutoLevel: number;
  stopLoad: () => void;
  startLoad: (startPosition?: number) => void;
  on: (
    event: typeof Hls.Events.FRAG_LOADING | typeof Hls.Events.FRAG_LOADED,
    listener: (event: string, data: { readonly frag: Fragment }) => void,
  ) => void;
}

/**
 * Steps a player down while it fills its first buffer (fillStepDown): until its first chunk of
 * picture has arrived, a slow one's load is stopped, the quality capped at the one chosen, and
 * loading starts again where it starts. From its first chunk on, hls.js chooses by the speed it
 * measured, and gives up on a slow chunk itself while the video plays. Returns the stopping.
 */
export function stepDownWhileFilling(hls: FillingHls, monotonicMs: () => number): () => void {
  let loading: { readonly frag: Fragment; readonly sinceMs: number } | undefined;
  let hasFilled = false;
  hls.on(Hls.Events.FRAG_LOADING, (_event, { frag }) => {
    if (frag.type === PlaylistLevelType.MAIN && !hasFilled) {
      loading = { frag, sinceMs: monotonicMs() };
    }
  });
  hls.on(Hls.Events.FRAG_LOADED, (_event, { frag }) => {
    if (frag.type === PlaylistLevelType.MAIN) {
      hasFilled = true;
      loading = undefined;
    }
  });
  const look = setInterval(() => {
    if (loading === undefined) {
      return;
    }
    const { frag, sinceMs } = loading;
    const chosen = fillStepDown({
      levels: hls.levels,
      loadingLevel: frag.level,
      durationS: frag.duration,
      loadingMs: monotonicMs() - sinceMs,
      loadedBytes: frag.stats.loaded,
    });
    if (chosen === undefined) {
      return;
    }
    loading = undefined;
    hls.stopLoad();
    // eslint-disable-next-line no-param-reassign -- hls.js is held to, and moves to, what is set
    hls.autoLevelCapping = chosen;
    // eslint-disable-next-line no-param-reassign -- see above
    hls.nextAutoLevel = chosen;
    hls.startLoad(hls.startPosition);
  }, lookWhileFillingEveryMs);
  return () => {
    clearInterval(look);
  };
}
