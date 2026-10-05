// SPDX-License-Identifier: MPL-2.0

import Hls, { type LoaderResponse, type LoadPolicy } from "hls.js";

import { hlsErrorFailure, type PlaybackFailure } from "./failure.ts";
import { hlsMethod, hlsSupport, playNative, startWithAudio, type HlsPlayback } from "./hls.ts";
import { nextCap } from "./step-up.ts";

/**
 * How long a chunk may take to begin arriving: longer than the core holds a request for one a run
 * is still making (internal/core/api's chunkWaitAtMost, 20 s), so a chunk on its way is waited
 * for, not given up on (D247 §10).
 */
export const chunkFirstByteWaitMs = 30_000;

/** HTTP's 425 Too Early: the core's word that a chunk is being made, and to ask again. */
const tooEarly = 425;

/** Whether a chunk's load is tried again: as hls.js would, and after the core's 425 too. */
// eslint-disable-next-line max-params -- hls.js's own signature for shouldRetry
function retriesTooEarly(
  _config: unknown,
  _count: number,
  _timedOut: unknown,
  response: LoaderResponse | undefined,
  isRetry: boolean,
): boolean {
  return isRetry || response?.code === tooEarly;
}

/**
 * How a chunked playback loads its chunks: waiting as long as the core does for one being made,
 * and asking again after the core's 425, "being prepared", as it says to; hls.js's own defaults
 * give up after ten seconds, and never ask again after a 425.
 */
export const chunkLoadPolicy: LoadPolicy = {
  default: {
    maxTimeToFirstByteMs: chunkFirstByteWaitMs,
    maxLoadTimeMs: 120_000,
    timeoutRetry: { maxNumRetry: 2, retryDelayMs: 0, maxRetryDelayMs: 0 },
    errorRetry: {
      maxNumRetry: 12,
      retryDelayMs: 1000,
      maxRetryDelayMs: 4000,
      shouldRetry: retriesTooEarly,
    },
  },
};

/** How often a chunked playback looks at whether it may step up a quality. */
export const lookForStepUpEveryMs = 10_000;

/**
 * A quality hls.js plays, as the playlist says: its peak bit rate, which hls.js steps down by, its
 * average, the sound's included, and its picture's height.
 */
export interface PlayedLevel {
  readonly bitrate: number;
  readonly averageBitrate: number;
  readonly height: number;
}

/** What adapting a chunked playback's quality needs of hls.js. */
export interface AdaptingHls {
  readonly levels: readonly PlayedLevel[];
  readonly currentLevel: number;
  readonly bandwidthEstimate: number;
  startLevel: number;
  autoLevelCapping: number;
  on: (
    event: typeof Hls.Events.MANIFEST_PARSED | typeof Hls.Events.LEVEL_SWITCHED,
    listener: () => void,
  ) => void;
}

/** The part of a video element adapting reads: how much is loaded ahead. */
export type AdaptedMedia = Pick<HTMLMediaElement, "buffered" | "currentTime">;

/** How much is loaded ahead of the position, in ms. */
function bufferedAheadMs(media: AdaptedMedia): number {
  const { buffered, currentTime } = media;
  for (let index = 0; index < buffered.length; index++) {
    if (buffered.start(index) <= currentTime && currentTime <= buffered.end(index)) {
      return (buffered.end(index) - currentTime) * 1000;
    }
  }
  return 0;
}

/**
 * Starts a chunked playback at the highest quality within startAtBps, the one the server chose from
 * the measured link (D247 §6), and steps it back up one quality at a time as nextCap allows (D247
 * §9); hls.js steps down by itself. Returns the stopping.
 */
export function adaptQuality(
  hls: AdaptingHls,
  media: AdaptedMedia,
  options: { readonly startAtBps: number; readonly monotonicMs: () => number },
): () => void {
  let changedAtMs = options.monotonicMs();
  hls.on(Hls.Events.MANIFEST_PARSED, () => {
    const within = hls.levels.findLastIndex((level) => level.bitrate <= options.startAtBps);
    const start = Math.max(within, 0);
    // eslint-disable-next-line no-param-reassign -- hls.js starts at, and is held to, what is set
    hls.startLevel = start;
    // eslint-disable-next-line no-param-reassign -- see above
    hls.autoLevelCapping = start;
    changedAtMs = options.monotonicMs();
  });
  hls.on(Hls.Events.LEVEL_SWITCHED, () => {
    changedAtMs = options.monotonicMs();
  });
  const look = setInterval(() => {
    const nowMs = options.monotonicMs();
    const cap = nextCap({
      levels: hls.levels,
      cap: hls.autoLevelCapping,
      playing: hls.currentLevel,
      estimateBps: hls.bandwidthEstimate,
      bufferedAheadMs: bufferedAheadMs(media),
      sinceChangeMs: nowMs - changedAtMs,
    });
    if (cap !== hls.autoLevelCapping) {
      // eslint-disable-next-line no-param-reassign -- see above
      hls.autoLevelCapping = cap;
      changedAtMs = nowMs;
    }
  }, lookForStepUpEveryMs);
  return () => {
    clearInterval(look);
  };
}

/** A chunked playback: an HLS playback that says which quality plays and how fast its link is. */
export interface ChunkedPlayback extends HlsPlayback {
  /** The quality playing, or undefined before one plays or where the browser chooses itself. */
  readonly playing: () => PlayedLevel | undefined;
  /** Whether it plays its lowest quality: a stall there is one no quality answers (D247 §8). */
  readonly isAtLowest: () => boolean;
}

/**
 * Plays the chunked master playlist at url (D247 §10): with hls.js where the page has MSE, adapting
 * its quality from startAtBps; else with the element's own HLS, which adapts by itself.
 */
export function playChunked(
  video: HTMLVideoElement,
  url: string,
  options: {
    readonly onFailed: (failure: PlaybackFailure) => void;
    readonly startAtBps: number;
    /** Where in the film it starts: where the player was when it stalled. */
    readonly startPositionS: number;
    readonly startAudioRenditionIndex?: number;
    readonly monotonicMs: () => number;
  },
): ChunkedPlayback {
  const method = hlsMethod(hlsSupport(video));
  const unknowing = {
    playing: () => undefined,
    estimateKbps: () => undefined,
    isAtLowest: () => false,
  };
  if (method === "native") {
    return { ...playNative(video, url), ...unknowing };
  }
  if (method === "none") {
    options.onFailed("hls_unsupported");
    return { setAudioRendition: () => undefined, destroy: () => undefined, ...unknowing };
  }
  // No probe of the lowest quality first: its chunks are made on request, and only for a player
  // that needs them.
  const hls = new Hls({
    testBandwidth: false,
    capLevelToPlayerSize: false,
    startPosition: options.startPositionS,
    fragLoadPolicy: chunkLoadPolicy,
  });
  hls.on(Hls.Events.ERROR, (_event, data) => {
    if (data.fatal) {
      options.onFailed(hlsErrorFailure(data));
    }
  });
  startWithAudio(hls, options.startAudioRenditionIndex ?? 0);
  const stopAdapting = adaptQuality(hls, video, {
    startAtBps: options.startAtBps,
    monotonicMs: options.monotonicMs,
  });
  hls.loadSource(url);
  hls.attachMedia(video);
  return {
    setAudioRendition: (index) => {
      hls.audioTrack = index;
    },
    destroy: () => {
      stopAdapting();
      hls.destroy();
    },
    playing: () => hls.levels[hls.currentLevel],
    estimateKbps: () => (hls.bandwidthEstimate > 0 ? hls.bandwidthEstimate / 1000 : undefined),
    isAtLowest: () => hls.currentLevel === 0,
  };
}
