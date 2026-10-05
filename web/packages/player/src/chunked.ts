// SPDX-License-Identifier: MPL-2.0

import Hls from "hls.js";

import { hlsErrorFailure, type PlaybackFailure } from "./failure.ts";
import { hlsMethod, hlsSupport, playNative, startWithAudio, type HlsPlayback } from "./hls.ts";
import { nextCap } from "./step-up.ts";

/** How often a chunked playback looks at whether it may step up a quality. */
export const lookForStepUpEveryMs = 10_000;

/** A quality hls.js plays: its peak bit rate and its picture's height, as the playlist says. */
export interface PlayedLevel {
  readonly bitrate: number;
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
  /** The link's speed in kb/s as hls.js measures its chunks, or undefined where it cannot say. */
  readonly estimateKbps: () => number | undefined;
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
  const hls = new Hls({ testBandwidth: false, capLevelToPlayerSize: false });
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
