// SPDX-License-Identifier: MPL-2.0

import Hls, { type HlsConfig, type LoaderResponse, type LoadPolicy } from "hls.js";

import { shiftingFragmentLoader, SoundShift } from "./audio-shift.ts";

import {
  groupJumpedEvent,
  isFollowingAGroup,
  isVoiceLive,
  mediaCarrierOf,
  type CannotKeepUp,
} from "@orpheum/sdk";

import { fromCacheBy } from "./cache-hit.ts";
import {
  noRoomToMakeStatus,
  tooEarlyStatus,
  unreachableStatus,
  type PlaybackFailed,
} from "./failure.ts";
import { orpheumAbr, type OrpheumAbr } from "./abr-controller.ts";
import type { ChoiceWhy, ChoosingOptions } from "./abr-options.ts";
import { diagnose, diagnosisOf, samplesNoter, stepNoter } from "./diagnosis.ts";
import { loadOnAfterFailures } from "./load-again.ts";
import {
  failOnFatal,
  hlsMethod,
  hlsSupport,
  markSeeks,
  obeyGroupLoading,
  soughtHeader,
  playNative,
  startAndKeepAudio,
  tellLoading,
  tellOwnMoves,
  type HlsPlayback,
  type LoadingHls,
} from "./hls.ts";
import {
  deviceFactsOf,
  keepForwardBuffer,
  loadsAheadBelowTopS,
  type ForwardBuffer,
} from "./forward-buffer.ts";
import {
  longTasksOf,
  watchMainThread,
  type LongTasks,
  type WatchedFilm,
  type WatchedHls,
} from "./main-thread.ts";
import { tellNextChunkLoad } from "./next-chunk.ts";
import { watchStalls, type WatchedMedia } from "./stalls.ts";
import { steadinessNoter } from "./steadiness.ts";
import { linkNoter, linkSpreadNoter, type RememberedLink } from "./link-estimate.ts";
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
 * How much of what was played a chunked playback keeps in the browser's buffer behind where it plays,
 * in seconds: three of the player's 10 s steps back (seekStepMs) and a Watch Together follower's
 * jump back of a second or less, its chunk on the ten-second grid kept with it, so a step back plays
 * at once. hls.js keeps everything played by default (Infinity), so a device would hold a film's
 * played minutes up to the browser's memory for a page's picture, and then evict at every append
 * (the efficiency audit's finding 5, web-client-efficiency-audit-2026-10-28.md). That memory is
 * Chromium's 150 MiB on a desktop, which a film's own picture at 20 Mb/s reaches in a minute; on an
 * Android device of 1 GB, as a Fire TV Stick is, Chromium's own source gives 30 MiB, which 30 s
 * behind at 9 Mb/s alone fills, though whether Silk takes that limit is not shown: a staging log had
 * the stick holding more (forward-buffer-2026-10-29.md §3.3; VQ121, its test T4). Lowering it where
 * the limit is 30 MiB is the owner's call (the report's P2). A justified constant.
 */
export const keptBehindS = 30;

/**
 * The event a chunked playback's video is sent when it gives up a quality as one this device
 * cannot play, its detail an UnplayableQuality, for the play report to tell the core (D266).
 */
export const playerFoundUnplayableEvent = "orpheumplayerfoundunplayable";

/** A quality given up as unplayable here: its BANDWIDTH, in kbit/s, and where the video stood. */
export interface UnplayableQuality {
  readonly qualityKbps: number;
  readonly positionMs: number;
  /** Whether it was the film's own picture, offered then to none of the device's later playbacks of the film. */
  readonly isFilmsOwnPicture?: boolean;
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
  const { qualityKbps, positionMs, readyState, isPaused, isFilmsOwnPicture } = fields;
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
    ...(typeof isFilmsOwnPicture === "boolean" ? { isFilmsOwnPicture } : {}),
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
): (unplayable: {
  readonly qualityKbps: number;
  readonly positionMs: number;
  readonly isFilmsOwnPicture: boolean;
}) => void {
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

/** The header the server's answer says the highest rate it lets the player load in: the core's CeilingHeader (D286 §5). */
export const ceilingHeader = "Orpheum-Ceiling-Kbps";

const httpOk = 200;

/** The ceiling a header says, in kbit/s, or undefined where it says none or nonsense. */
export function ceilingOf(header: string | null): number | undefined {
  const kbps = Number(header ?? "");
  return header !== null && Number.isInteger(kbps) && kbps > 0 ? kbps : undefined;
}

/** The header a download asks the server to pace it in: the core's PaceHeader (D262 §2). */
export const paceHeader = "Orpheum-Pace-Kbps";

/**
 * Whether a chunk's load is tried again: as hls.js would, and after the core's 425 too; never after
 * its 429 or 503, which asking again does not change.
 */
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
  if (response?.code === unreachableStatus) {
    // Its film cannot be read now: the page says why and plays again once it can, where ninety asks
    // again spun the player for minutes, saying nothing (the owner's test, 2026-10-21).
    return false;
  }
  return isRetry || response?.code === tooEarlyStatus;
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
 * hls.js's settings for a chunked playback from startPositionS, loading ahead no further than
 * loadsAheadBelowTopS, or loadsAheadAtMostS where given, until its ForwardBuffer sets the target for
 * the quality it loads (forward-buffer-2026-10-29.md P1); hls.js's bytes never stretch it, as its
 * maxBufferSize of 60 MB did to ten minutes of 360p (staging, 2026-10-29). No probe of the lowest
 * quality first: its chunks are made on request, and only for a player that needs them.
 */
export function chunkedHlsConfig(options: {
  readonly startPositionS: number;
  readonly loadsAheadAtMostS?: number;
  /** The pace each download asks of the server, in kbit/s, or undefined for none (D262 §2). */
  readonly paceKbps?: () => number | undefined;
  /** Whether the video loads where a seek took it, which each request then says (markSeeks). */
  readonly isSought?: () => boolean;
  /** Told the server's ceiling, kbps, with each answer, undefined where it sets none (D286 §5). */
  readonly onCeiling?: (kbps: number | undefined) => void;
  /**
   * The Authorization each request carries, for a page with no media cookie on the core's origin:
   * the lab's cast receiver, handed its sender's sign-in (plans/tv-device-tests.md); none elsewhere.
   */
  readonly authorization?: string | undefined;
}): Partial<HlsConfig> {
  const aheadAtStartS = Math.min(
    loadsAheadBelowTopS,
    options.loadsAheadAtMostS ?? Number.POSITIVE_INFINITY,
  );
  return {
    xhrSetup: (xhr) => {
      xhr.addEventListener("load", () => {
        if (xhr.status === httpOk) {
          options.onCeiling?.(ceilingOf(xhr.getResponseHeader(ceilingHeader)));
        }
      });
      const kbps = options.paceKbps?.();
      if (kbps !== undefined) {
        xhr.setRequestHeader(paceHeader, String(kbps));
      }
      if (options.isSought?.() === true) {
        xhr.setRequestHeader(soughtHeader, "true");
      }
      if (options.authorization !== undefined) {
        xhr.setRequestHeader("Authorization", options.authorization);
      }
    },
    testBandwidth: false,
    capLevelToPlayerSize: false,
    startPosition: options.startPositionS,
    backBufferLength: keptBehindS,
    fragLoadPolicy: chunkLoadPolicy,
    maxBufferSize: 0,
    maxBufferLength: aheadAtStartS,
    maxMaxBufferLength: aheadAtStartS,
  };
}

/**
 * Does as the follower of its group says: loads as it says (obeyGroupLoading), and starts a jump of
 * its group on the common quality (groupJumpedEvent, D286 §3). Returns the stopping.
 */
function obeyGroup(
  hls: Hls,
  video: HTMLVideoElement,
  by: {
    readonly abr: { readonly choosing: () => { readonly groupJumped: () => void } | undefined };
    readonly ahead: ForwardBuffer;
  },
): () => void {
  const { abr } = by;
  const stopObeying = obeyGroupLoading(hls, video, by.ahead);
  const onGroupJumped = (): void => {
    abr.choosing()?.groupJumped();
  };
  video.addEventListener(groupJumpedEvent, onGroupJumped);
  return () => {
    stopObeying();
    video.removeEventListener(groupJumpedEvent, onGroupJumped);
  };
}

/**
 * Tells the bitrate controller of each of video's stalls (watchStalls, D247 §2): a drop, which it
 * weighs as a flaky link's sign and, while starting, lowers its ceiling for (D352 §1, §4); a slow
 * load is none while the chunk on its way waits on the core, which is making it. Returns the
 * stopping.
 */
export function tellStalls(
  video: WatchedMedia,
  choosing: () =>
    { readonly stalled: () => void; readonly isAwaitingTheCore: () => boolean } | undefined,
): () => void {
  return watchStalls(
    video,
    () => {
      choosing()?.stalled();
    },
    { isAwaitingTheCore: () => choosing()?.isAwaitingTheCore() ?? false },
  );
}

/**
 * What a chunked playback follows of its video once hls.js plays it: how far ahead it loads
 * (keepForwardBuffer, at most atMostS where given, its top its controller's, its notes for the play
 * report), its group (obeyGroup), its sound's shift, its stalls (tellStalls), and its page's main
 * thread's stalls (watchMainThread, for the same notes). Returns each stopping.
 */
function followAheadGroupSoundAndStalls(
  hls: Hls,
  video: HTMLVideoElement,
  by: {
    readonly abr: OrpheumAbr;
    readonly soundShift: SoundShift;
    readonly atMostS: number | undefined;
  },
): readonly (() => void)[] {
  const ahead = keepForwardBuffer(hls, {
    device: deviceFactsOf(globalThis.navigator),
    isTop: (level) => level >= (by.abr.choosing()?.topLevel() ?? Number.POSITIVE_INFINITY),
    note: linkNoter(video),
    atMostS: by.atMostS,
  });
  return [
    ahead.stop,
    obeyGroup(hls, video, { abr: by.abr, ahead: ahead.buffer }),
    by.soundShift.attach(hls, video),
    tellStalls(video, by.abr.choosing),
    tellMainThreadStalls(hls, video, longTasksOf(globalThis)),
  ];
}

/**
 * Notes each stall of the page's main thread while video plays (watchMainThread), from the browser's
 * longTasks, or a timer where it reports none, with the link's notes, which the play report carries
 * and the core logs. Returns the stopping.
 */
export function tellMainThreadStalls(
  hls: WatchedHls,
  video: WatchedFilm,
  longTasks: LongTasks | undefined,
): () => void {
  return watchMainThread(hls, video, { note: linkNoter(video), longTasks });
}

/**
 * Tells video what a chunked playback's hls.js loads (tellLoading), each quality it begins with the
 * link as its own bitrate controller measured it, its confident estimate, and none before it has
 * (D352 §1): hls.js's estimate, which our controller samples only as it does, read as hls.js's
 * starting value, 5000 kb/s, on a Fire TV whose controller had measured nothing (the owner's cast,
 * staging, 2026-10-28); and why the controller chose it (whyChose), which a switch's line in the
 * core's log never said (the same Fire TV's Nosferatu, 15:33). Returns that estimate, in kbit/s.
 */
export function tellChunkedLoading(
  hls: LoadingHls,
  video: Pick<EventTarget, "dispatchEvent">,
  choosing: () =>
    | {
        readonly linkKbps: () => number | undefined;
        readonly whyChose: (level: number) => ChoiceWhy | undefined;
      }
    | undefined,
): () => number | undefined {
  const linkKbps = (): number | undefined => choosing()?.linkKbps();
  tellLoading(hls, video, { linkKbps, whyChose: (level) => choosing()?.whyChose(level) });
  return linkKbps;
}

/**
 * Plays url with the element's own HLS where the page has no MSE, knowing nothing of its qualities,
 * or fails as unsupported where it has neither; undefined where hls.js plays it.
 */
function playWithoutHlsJs(
  video: HTMLVideoElement,
  url: string,
  onFailed: PlaybackFailed,
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
 * The share of a measured link a quality may take, as an offer's percent says it; undefined, the
 * controller's starting share, where the offer says none.
 */
export function shareForQualityOf(linkSharePercent: number | undefined): number | undefined {
  return linkSharePercent !== undefined && linkSharePercent > 0
    ? linkSharePercent / 100
    : undefined;
}

/** The link a playback's offer says its device last measured on its network, as the offer says it. */
interface RememberedOffer {
  /**
   * Its speed, kbit/s (D257 §1), the spread of the natural logs of the samples it rested on, and how
   * many of the playback's own samples it counts as (bandwidth-sampling-2026-10-29.md P2); each
   * absent for none.
   */
  readonly rememberedLinkKbps?: number | undefined;
  readonly rememberedLinkSpread?: number | undefined;
  readonly rememberedLinkSamples?: number | undefined;
}

/** The prior an offer's remembered link gives the controller's estimate; undefined for none. */
export function rememberedLinkOf(offer: RememberedOffer): RememberedLink | undefined {
  const { rememberedLinkKbps: kbps, rememberedLinkSamples: countsAsSamples } = offer;
  return kbps === undefined || countsAsSamples === undefined
    ? undefined
    : { kbps, logSpread: offer.rememberedLinkSpread, countsAsSamples };
}

/**
 * What the bitrate controller of video's chunked playback chooses by (D265): starting within
 * startAtBps at the steadiness level its offer says (D352 Q2), giving way to live voice and to its
 * group, telling the page of a quality unplayable here (D266), and keeping each of its choices, with
 * why, among the video's steps, and its steadiness level and its link's spread for its play reports;
 * its estimate of the link starting from what its offer says its device remembers (P2).
 */
export function choosingOptionsOf(
  video: HTMLVideoElement,
  options: {
    readonly startAtBps: number;
    readonly monotonicMs: () => number;
    readonly linkSharePercent?: number | undefined;
    readonly startSteadiness?: number | undefined;
  } & RememberedOffer,
): ChoosingOptions {
  return {
    startAtBps: options.startAtBps,
    shareForQuality: shareForQualityOf(options.linkSharePercent),
    startSteadiness: options.startSteadiness,
    rememberedLink: rememberedLinkOf(options),
    monotonicMs: options.monotonicMs,
    isVoiceLive,
    isInAGroup: isFollowingAGroup,
    fromCache: typeof performance === "undefined" ? () => undefined : fromCacheBy(performance),
    onUnplayable: sendUnplayable(video),
    noteStep: stepNoter(video),
    noteSamples: samplesNoter(video),
    noteSteadiness: steadinessNoter(video),
    noteLinkSpread: linkSpreadNoter(video),
    noteLink: linkNoter(video),
    carrierOf: mediaCarrierOf,
  };
}

/**
 * Tells onFailed when the playback cannot go on (failOnFatal), what failed with it, and asks a load
 * that failed again instead (loadOnAfterFailures), heard first, so failOnFatal passes it by. Returns
 * the stopping.
 */
function failOrLoadOn(
  hls: Hls,
  video: HTMLVideoElement,
  by: { readonly abr: OrpheumAbr; readonly onFailed: PlaybackFailed },
): () => void {
  const { abr, onFailed } = by;
  const stop = loadOnAfterFailures(hls, {
    askingAgain: () => abr.choosing()?.askingAgain(),
    note: linkNoter(video),
    carrierOf: mediaCarrierOf,
    page: typeof window === "undefined" ? undefined : window,
  });
  failOnFatal(hls, onFailed, mediaCarrierOf);
  return stop;
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
    readonly onFailed: PlaybackFailed;
    readonly startAtBps: number;
    /**
     * The share of a measured link, in percent, a quality may take, as the playback's offer says
     * (link_share_for_quality_percent); the starting 70% when absent or 0.
     */
    readonly linkSharePercent?: number | undefined;
    /**
     * The steadiness level it starts at, as the playback's offer says: the one its device last
     * ended at on its network (D352 Q2); 0 when absent.
     */
    readonly startSteadiness?: number | undefined;
    /** Where in the film it starts: where the player was when it stalled. */
    readonly startPositionS: number;
    readonly startAudioRenditionIndex?: number;
    readonly monotonicMs: () => number;
    /**
     * How many seconds ahead it loads at most; its ForwardBuffer's alone when absent. A film page's
     * hidden player fills only its first buffer, so it does not hold a phone's link as one browses.
     */
    readonly loadsAheadAtMostS?: number;
    /** The Authorization each request carries, where the page has no media cookie (chunkedHlsConfig). */
    readonly authorization?: string | undefined;
  } & RememberedOffer,
): ChunkedPlayback {
  const withoutHlsJs = playWithoutHlsJs(video, url, options.onFailed);
  if (withoutHlsJs !== undefined) {
    return withoutHlsJs;
  }
  const abr = orpheumAbr(choosingOptionsOf(video, options));
  const seeks = markSeeks(video);
  const soundShift = new SoundShift();
  const hls = new Hls({
    fLoader: shiftingFragmentLoader(Hls.DefaultConfig.loader, () => soundShift.earlierMs),
    ...chunkedHlsConfig({
      ...options,
      paceKbps: () => abr.choosing()?.paceKbps(),
      isSought: seeks.isSought,
      onCeiling: (kbps) => {
        abr.choosing()?.obeyServer(kbps);
      },
    }),
    abrController: abr.controller,
  });
  const stopLoadingOn = failOrLoadOn(hls, video, { abr, onFailed: options.onFailed });
  diagnose(hls, video);
  const chooseAudio = startAndKeepAudio(hls, options.startAudioRenditionIndex, linkNoter(video));
  tellOwnMoves(hls, video);
  const linkKbps = tellChunkedLoading(hls, video, abr.choosing);
  const follow = { abr, soundShift, atMostS: options.loadsAheadAtMostS };
  const stops = [...followAheadGroupSoundAndStalls(hls, video, follow), seeks.stop, stopLoadingOn];
  stops.push(tellNextChunkLoad(hls, video, () => abr.choosing()?.nextChunkLink()));
  hls.loadSource(url);
  hls.attachMedia(video);
  return {
    setAudioRendition: chooseAudio,
    destroy: () => {
      for (const stop of stops) {
        stop();
      }
      hls.destroy();
    },
    playing: () => hls.levels[hls.currentLevel],
    settledAtBps: () => settledAtBps(hls),
    estimateKbps: linkKbps,
    beyondTheLink: () => beyondTheLink(hls, linkKbps()),
  };
}
