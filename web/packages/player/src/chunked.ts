// SPDX-License-Identifier: MPL-2.0

import Hls, { type HlsConfig, type LoaderResponse, type LoadPolicy } from "hls.js";

import { isVoiceLive, type CannotKeepUp } from "@orpheum/sdk";

import { noRoomToMakeStatus, type PlaybackFailure } from "./failure.ts";
import { stepDownWhileFilling } from "./fill.ts";
import {
  failOnFatal,
  hlsMethod,
  hlsSupport,
  measuredKbps,
  playNative,
  startWithAudio,
  tellLoading,
  tellOwnMoves,
  type HlsPlayback,
} from "./hls.ts";
import { nextCap, stepUpHeadroom } from "./step-up.ts";

/**
 * How long a chunk may take to begin arriving: longer than the core holds a request for one a run
 * is still making, at most 30 s (chunk_hold_ms; by default it does not), so a chunk on its way is
 * waited for, not given up on (D247 §10).
 */
export const chunkFirstByteWaitMs = 30_000;

/**
 * How many times a chunk the core says is being made (425) is asked for again, a second or two
 * apart: over a minute and a half, longer than a player catching up waits for the run already
 * making its quality to reach where it jumped to, at most a minute of film ahead (D250 §4, §5).
 */
export const chunkAskAgainCount = 90;

/** The header a download asks the server to pace it in: the core's PaceHeader (D262 §2). */
export const paceHeader = "Orpheum-Pace-Kbps";

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
  if (response?.code === noRoomToMakeStatus) {
    return false; // the core has no room to make it: asking again finds none (D250 §6)
  }
  return isRetry || response?.code === tooEarly;
}

/**
 * How a chunked playback loads its chunks: waiting as long as the core may hold one being made,
 * and asking again after the core's 425, "being made", every second or two for chunkAskAgainCount
 * times; hls.js's own defaults give up after ten seconds, and never ask again after a 425.
 */
export const chunkLoadPolicy: LoadPolicy = {
  default: {
    maxTimeToFirstByteMs: chunkFirstByteWaitMs,
    maxLoadTimeMs: 120_000,
    timeoutRetry: { maxNumRetry: 2, retryDelayMs: 0, maxRetryDelayMs: 0 },
    errorRetry: {
      maxNumRetry: chunkAskAgainCount,
      retryDelayMs: 1000,
      maxRetryDelayMs: 2000,
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

/**
 * Whether a stall now is beyond the link, with both numbers: one at the lowest quality, on a link
 * measured below stepUpHeadroom times what it needs, or not measured at all (D247 §8). A stall on
 * a link measured well above the lowest is something else's doing, and no quality answers it: a
 * player on a 98 Mbit/s link, held behind its Watch Together group at the 892 kbit/s quality, was
 * told its link could not keep up (staging, 2026-10-06).
 */
export function beyondTheLink(
  hls: Pick<AdaptingHls, "levels" | "currentLevel" | "bandwidthEstimate">,
): CannotKeepUp | undefined {
  const lowest = hls.levels[0];
  if (hls.currentLevel !== 0 || lowest === undefined) {
    return undefined;
  }
  const linkKbps = hls.bandwidthEstimate > 0 ? hls.bandwidthEstimate / 1000 : undefined;
  const lowestKbps = lowest.averageBitrate / 1000;
  if (linkKbps !== undefined && linkKbps >= stepUpHeadroom * lowestKbps) {
    return undefined;
  }
  return { linkKbps, lowestKbps };
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

/** The share of the link the film may take while voice is live (D262 §2). */
const voiceLinkShare = 0.5;

/**
 * The highest quality, by its place among levels, the film may play while voice is live: within
 * half the link's estimate, or the lowest when none is; any, while the link is not estimated.
 */
export function voiceCap(levels: readonly PlayedLevel[], estimateBps: number): number {
  if (!(estimateBps > 0)) {
    return levels.length - 1;
  }
  return Math.max(
    levels.findLastIndex((level) => level.averageBitrate <= voiceLinkShare * estimateBps),
    0,
  );
}

/**
 * How far above its quality's rate, its BANDWIDTH, a chunk's download is paced while voice is
 * live: enough that a busy chunk still arrives well before it plays (D262 §2).
 */
export const paceAboveQuality = 1.25;

/**
 * The pace a download asks while this tab's voice is live, in kbit/s: a little above the quality
 * hls.js loads, so the film never fills the link's queue in bursts ahead of the voice (D262 §2);
 * undefined while voice is not live, or before a quality is loaded.
 */
function voicePaceKbps(hls: Pick<Hls, "levels" | "loadLevel">): number | undefined {
  const bitrate = hls.levels[hls.loadLevel]?.bitrate ?? 0;
  return isVoiceLive() && bitrate > 0 ? Math.ceil((bitrate * paceAboveQuality) / 1000) : undefined;
}

/**
 * Starts a chunked playback at the highest quality within startAtBps, the one the server chose from
 * the measured link (D247 §6), and steps it back up one quality at a time as nextCap allows (D247
 * §9); hls.js steps down by itself. Returns the stopping.
 */
export function adaptQuality(
  hls: AdaptingHls,
  media: AdaptedMedia,
  options: {
    readonly startAtBps: number;
    readonly monotonicMs: () => number;
    /** Whether this tab's voice is live, when the film is capped at half the link (voiceCap). */
    readonly isVoiceLive?: () => boolean;
  },
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
    const stepped = nextCap({
      levels: hls.levels,
      cap: hls.autoLevelCapping,
      playing: hls.currentLevel,
      estimateBps: hls.bandwidthEstimate,
      bufferedAheadMs: bufferedAheadMs(media),
      sinceChangeMs: nowMs - changedAtMs,
    });
    const cap =
      options.isVoiceLive?.() === true
        ? Math.min(stepped, voiceCap(hls.levels, hls.bandwidthEstimate))
        : stepped;
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

/**
 * The BANDWIDTH of the quality a playback last loaded a chunk of, where its stepping has settled so
 * far: a player made after it starts there (D260 §3). Undefined before it loads one.
 */
export function settledAtBps(hls: {
  readonly levels: readonly PlayedLevel[];
  readonly loadLevel: number;
}): number | undefined {
  return hls.levels[hls.loadLevel]?.bitrate;
}

/** A chunked playback: an HLS playback that says which quality plays and how fast its link is. */
export interface ChunkedPlayback extends HlsPlayback {
  /** The quality playing, or undefined before one plays or where the browser chooses itself. */
  readonly playing: () => PlayedLevel | undefined;
  /** settledAtBps, or undefined where the browser chooses itself. */
  readonly settledAtBps: () => number | undefined;
  /** Whether a stall now is beyond the link, with both numbers (beyondTheLink, D247 §8). */
  readonly beyondTheLink: () => CannotKeepUp | undefined;
}

/**
 * hls.js's settings for a chunked playback from startPositionS, loading at most loadsAheadAtMostS
 * ahead where given. No probe of the lowest quality first: its chunks are made on request, and only
 * for a player that needs them.
 */
export function chunkedHlsConfig(options: {
  readonly startPositionS: number;
  readonly loadsAheadAtMostS?: number;
  /** The pace each download asks of the server, in kbit/s, or undefined for none (D262 §2). */
  readonly paceKbps?: () => number | undefined;
}): Partial<HlsConfig> {
  return {
    xhrSetup: (xhr) => {
      const kbps = options.paceKbps?.();
      if (kbps !== undefined) {
        xhr.setRequestHeader(paceHeader, String(kbps));
      }
    },
    testBandwidth: false,
    capLevelToPlayerSize: false,
    startPosition: options.startPositionS,
    fragLoadPolicy: chunkLoadPolicy,
    ...(options.loadsAheadAtMostS === undefined
      ? {}
      : {
          maxBufferLength: options.loadsAheadAtMostS,
          maxMaxBufferLength: options.loadsAheadAtMostS,
        }),
  };
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
    /**
     * How many seconds ahead it loads at most; hls.js's own when absent. A film page's hidden player
     * fills only its first buffer, so it does not hold a phone's link while its person browses.
     */
    readonly loadsAheadAtMostS?: number;
  },
): ChunkedPlayback {
  const method = hlsMethod(hlsSupport(video));
  const unknowing = {
    playing: () => undefined,
    settledAtBps: () => undefined,
    estimateKbps: () => undefined,
    beyondTheLink: () => undefined,
  };
  if (method === "native") {
    return { ...playNative(video, url), ...unknowing };
  }
  if (method === "none") {
    options.onFailed("hls_unsupported");
    return { setAudioRendition: () => undefined, destroy: () => undefined, ...unknowing };
  }
  const hls: Hls = new Hls(chunkedHlsConfig({ ...options, paceKbps: () => voicePaceKbps(hls) }));
  failOnFatal(hls, options.onFailed);
  startWithAudio(hls, options.startAudioRenditionIndex ?? 0);
  tellOwnMoves(hls, video);
  tellLoading(hls, video);
  const stopAdapting = adaptQuality(hls, video, {
    startAtBps: options.startAtBps,
    monotonicMs: options.monotonicMs,
    isVoiceLive,
  });
  const stopFilling = stepDownWhileFilling(hls, options.monotonicMs);
  const estimateKbps = measuredKbps(hls);
  hls.loadSource(url);
  hls.attachMedia(video);
  return {
    setAudioRendition: (index) => {
      hls.audioTrack = index;
    },
    destroy: () => {
      stopAdapting();
      stopFilling();
      hls.destroy();
    },
    playing: () => hls.levels[hls.currentLevel],
    settledAtBps: () => settledAtBps(hls),
    estimateKbps,
    beyondTheLink: () => beyondTheLink(hls),
  };
}
