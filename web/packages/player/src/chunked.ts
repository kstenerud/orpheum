// SPDX-License-Identifier: MPL-2.0

import Hls, { type HlsConfig, type LoaderResponse, type LoadPolicy } from "hls.js";

import { isFollowingAGroup, isVoiceLive, type CannotKeepUp } from "@orpheum/sdk";

import { fromCacheBy } from "./cache-hit.ts";
import { noRoomToMakeStatus, type PlaybackFailure } from "./failure.ts";
import { orpheumAbr } from "./abr.ts";
import { diagnose, diagnosisOf } from "./diagnosis.ts";
import {
  failOnFatal,
  hlsMethod,
  hlsSupport,
  markSeeks,
  obeyGroupLoading,
  soughtHeader,
  playNative,
  startWithAudio,
  tellLoading,
  tellOwnMoves,
  type HlsPlayback,
} from "./hls.ts";
import { stepUpHeadroom } from "./step-up.ts";

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

/**
 * The event a chunked playback's video is sent when it gives up a quality as one this device
 * cannot play, its detail an UnplayableQuality, for the play report to tell the core (D266).
 */
export const playerFoundUnplayableEvent = "orpheumplayerfoundunplayable";

/** A quality given up as unplayable here: its BANDWIDTH, in kbit/s, and where the video stood. */
export interface UnplayableQuality {
  readonly qualityKbps: number;
  readonly positionMs: number;
  /**
   * The video as it was when the quality was given up, before its decoding was built anew: its
   * readyState, whether it was paused, what its picture's and sound's buffers held, and its last
   * error. A stall reported after showed only the rebuilt pipeline refilling (D266).
   */
  readonly readyState?: number;
  readonly isPaused?: boolean;
  readonly videoBuffered?: string;
  readonly audioBuffered?: string;
  readonly lastError?: string;
}

/** An event's UnplayableQuality, or undefined when it carries none. */
export function unplayableOf(event: Event): UnplayableQuality | undefined {
  if (!(event instanceof CustomEvent)) {
    return undefined;
  }
  const detail: unknown = event.detail;
  if (typeof detail !== "object" || detail === null) {
    return undefined;
  }
  const fields = Object.fromEntries(Object.entries(detail));
  const { qualityKbps, positionMs, readyState, isPaused } = fields;
  if (typeof qualityKbps !== "number" || typeof positionMs !== "number") {
    return undefined;
  }
  const text = (key: string): Readonly<Record<string, string>> =>
    typeof fields[key] === "string" ? { [key]: fields[key] } : {};
  return {
    qualityKbps,
    positionMs,
    ...(typeof readyState === "number" ? { readyState } : {}),
    ...(typeof isPaused === "boolean" ? { isPaused } : {}),
    ...text("videoBuffered"),
    ...text("audioBuffered"),
    ...text("lastError"),
  };
}

/**
 * Sends video a playerFoundUnplayableEvent for each quality given up, with the video as it was,
 * read before its decoding is built anew, which empties every buffer (D266).
 */
export function sendUnplayable(
  video: Pick<HTMLVideoElement, "readyState" | "paused" | "error" | "dispatchEvent">,
): (unplayable: { readonly qualityKbps: number; readonly positionMs: number }) => void {
  return (unplayable) => {
    const found: UnplayableQuality = {
      ...unplayable,
      readyState: video.readyState,
      isPaused: video.paused,
      ...diagnosisOf(video),
    };
    video.dispatchEvent(new CustomEvent(playerFoundUnplayableEvent, { detail: found }));
  };
}

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

/**
 * A quality hls.js plays, as the playlist says: its peak bit rate, by which the start is chosen and
 * a voice's pace set, its average, the sound's included, by which the bitrate controller weighs
 * the link (D265), and its picture's height.
 */
export interface PlayedLevel {
  readonly bitrate: number;
  readonly averageBitrate: number;
  readonly height: number;
}

/**
 * Whether a stall now is beyond the link, with both numbers: one at the lowest quality, on a link
 * measured below stepUpHeadroom times what it needs, or not measured at all (D247 §8). A stall on
 * a link measured well above the lowest is something else's doing, and no quality answers it: a
 * player on a 98 Mbit/s link, held behind its Watch Together group at the 892 kbit/s quality, was
 * told its link could not keep up (staging, 2026-10-06).
 */
export function beyondTheLink(
  hls: { readonly levels: readonly PlayedLevel[]; readonly currentLevel: number },
  linkKbps: number | undefined,
): CannotKeepUp | undefined {
  const lowest = hls.levels[0];
  if (hls.currentLevel !== 0 || lowest === undefined) {
    return undefined;
  }
  const lowestKbps = lowest.averageBitrate / 1000;
  if (linkKbps !== undefined && linkKbps >= stepUpHeadroom * lowestKbps) {
    return undefined;
  }
  return { linkKbps, lowestKbps };
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
  /** Whether the video loads where a seek took it, which each request then says (markSeeks). */
  readonly isSought?: () => boolean;
}): Partial<HlsConfig> {
  return {
    xhrSetup: (xhr) => {
      const kbps = options.paceKbps?.();
      if (kbps !== undefined) {
        xhr.setRequestHeader(paceHeader, String(kbps));
      }
      if (options.isSought?.() === true) {
        xhr.setRequestHeader(soughtHeader, "true");
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
 * Plays url with the element's own HLS where the page has no MSE, knowing nothing of its qualities,
 * or fails as unsupported where it has neither; undefined where hls.js plays it.
 */
function playWithoutHlsJs(
  video: HTMLVideoElement,
  url: string,
  onFailed: (failure: PlaybackFailure) => void,
): ChunkedPlayback | undefined {
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
    onFailed("hls_unsupported");
    return { setAudioRendition: () => undefined, destroy: () => undefined, ...unknowing };
  }
  return undefined;
}

/**
 * Plays the chunked master playlist at url (D247 §10): with hls.js where the page has MSE, its
 * quality chosen by a bitrate controller of our own from startAtBps (D265); else with the element's
 * own HLS, which adapts by itself.
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
  const withoutHlsJs = playWithoutHlsJs(video, url, options.onFailed);
  if (withoutHlsJs !== undefined) {
    return withoutHlsJs;
  }
  const abr = orpheumAbr({
    startAtBps: options.startAtBps,
    monotonicMs: options.monotonicMs,
    isVoiceLive,
    isInAGroup: isFollowingAGroup,
    fromCache: typeof performance === "undefined" ? () => undefined : fromCacheBy(performance),
    onUnplayable: sendUnplayable(video),
  });
  const seeks = markSeeks(video);
  const hls = new Hls({
    ...chunkedHlsConfig({
      ...options,
      paceKbps: () => abr.choosing()?.paceKbps(),
      isSought: seeks.isSought,
    }),
    abrController: abr.controller,
  });
  failOnFatal(hls, options.onFailed);
  diagnose(hls, video);
  startWithAudio(hls, options.startAudioRenditionIndex ?? 0);
  tellOwnMoves(hls, video);
  tellLoading(hls, video);
  const stopObeying = obeyGroupLoading(hls, video);
  const linkKbps = (): number | undefined => abr.choosing()?.linkKbps();
  hls.loadSource(url);
  hls.attachMedia(video);
  return {
    setAudioRendition: (index) => {
      hls.audioTrack = index;
    },
    destroy: () => {
      stopObeying();
      seeks.stop();
      hls.destroy();
    },
    playing: () => hls.levels[hls.currentLevel],
    settledAtBps: () => settledAtBps(hls),
    estimateKbps: linkKbps,
    beyondTheLink: () => beyondTheLink(hls, linkKbps()),
  };
}
