// SPDX-License-Identifier: MPL-2.0

import {
  catchUpSeekEvent,
  holdLoadingEvent,
  loadingRunwayEvent,
  loadingRunwayOf,
  loadingHoldOf,
  playerLoadedChunkEvent,
  playerMovedItselfEvent,
  playerSwitchedQualityEvent,
  type ChunkLoaded,
  type QualitySwitched,
} from "@orpheum/sdk";
import Hls, { PlaylistLevelType, type ErrorData } from "hls.js";

import { isRecovered } from "./abr.ts";
import { hlsErrorFailure, type PlaybackFailure } from "./failure.ts";

/** A prepared rendition playing in a video element. */
export interface HlsPlayback {
  /** Plays the audio rendition at index, in the master playlist's order. */
  readonly setAudioRendition: (index: number) => void;
  /** Stops playing and lets go of the element. */
  readonly destroy: () => void;
  /** The link's speed in kb/s as hls.js measures its segments, or undefined where it cannot say. */
  readonly estimateKbps: () => number | undefined;
}

/** What the page offers for HLS: hls.js over MSE, or the element's own HLS, as Safari's. */
export interface HlsSupport {
  readonly hasMse: boolean;
  readonly canPlayNatively: boolean;
}

/** Whether this page can play HLS, and how. */
export function hlsSupport(video: HTMLVideoElement): HlsSupport {
  return {
    hasMse: Hls.isSupported(),
    canPlayNatively: video.canPlayType("application/vnd.apple.mpegurl") !== "",
  };
}

/**
 * Whether this page can play in step with a Watch Together group: only hls.js can be nudged and
 * told what to load, so it needs MSE, or on an iPhone ManagedMediaSource, iOS 17.1 and later
 * (D250 §7). Below that an iPhone has only its own HLS, which watches alone.
 */
export function canFollowAGroup(): boolean {
  return Hls.isSupported();
}

/** How a page plays HLS: hls.js, the element's own HLS, or not at all. */
export type HlsMethod = "hls.js" | "native" | "none";

/** The method to play HLS by: hls.js wherever MSE is, since it switches audio renditions itself. */
export function hlsMethod(support: HlsSupport): HlsMethod {
  if (support.hasMse) {
    return "hls.js";
  }
  return support.canPlayNatively ? "native" : "none";
}

/** The part of a media element the element's own HLS needs. */
export interface NativeTarget {
  readonly getAttribute: (name: string) => string | null;
  readonly setAttribute: (name: string, value: string) => void;
  readonly removeAttribute: (name: string) => void;
  readonly load: () => void;
}

/** Plays a playlist with the element's own HLS, as Safari's. */
export function playNative(target: NativeTarget, url: string): HlsPlayback {
  target.setAttribute("src", url);
  return {
    // Safari switches its own tracks from its controls; choosing one is left to them.
    setAudioRendition: () => undefined,
    // Safari measures its link for itself, and says nothing of it.
    estimateKbps: () => undefined,
    // A page's video kept for its next film may hold that film's source already: it is left be.
    destroy: () => {
      if (target.getAttribute("src") === url) {
        target.removeAttribute("src");
        target.load();
      }
    },
  };
}

/** What choosing the starting audio rendition needs of hls.js. */
export interface AudioStart {
  on: (event: typeof Hls.Events.MANIFEST_PARSED, listener: () => void) => void;
  audioTrack: number;
}

/**
 * Starts with the audio rendition at audioIndex once the master playlist is read: hls.js picks the
 * playlist's default until then, and a choice made earlier is not kept.
 */
export function startWithAudio(hls: AudioStart, audioIndex: number): void {
  if (audioIndex === 0) {
    return;
  }
  hls.on(Hls.Events.MANIFEST_PARSED, () => {
    // eslint-disable-next-line no-param-reassign -- hls.js switches the mix when audioTrack is set
    hls.audioTrack = audioIndex;
  });
}

/** What telling the player's own moves needs of hls.js. */
export interface OwnMoves {
  on: (
    event: typeof Hls.Events.ERROR,
    listener: (event: unknown, data: Pick<ErrorData, "details">) => void,
  ) => void;
}

/**
 * Loads no more than the runway the follower of its group asks for, while one in the group is still
 * loading, and as far as fullS otherwise (loadingRunwayEvent, D285 §2). Returns the stopping.
 */
export function keepRunwayWhenTold(
  hls: { readonly config: Pick<Hls["config"], "maxBufferLength" | "maxMaxBufferLength"> },
  video: Pick<EventTarget, "addEventListener" | "removeEventListener">,
  fullS: { readonly maxBufferLength: number; readonly maxMaxBufferLength: number },
): () => void {
  const listener = (event: Event): void => {
    const runway = loadingRunwayOf(event);
    if (runway === undefined) {
      return;
    }
    const { aheadS } = runway;
    // eslint-disable-next-line no-param-reassign -- hls.js reads its config as it loads
    hls.config.maxBufferLength =
      aheadS === undefined ? fullS.maxBufferLength : Math.min(aheadS, fullS.maxBufferLength);
    // eslint-disable-next-line no-param-reassign -- as above
    hls.config.maxMaxBufferLength =
      aheadS === undefined ? fullS.maxMaxBufferLength : Math.min(aheadS, fullS.maxMaxBufferLength);
  };
  video.addEventListener(loadingRunwayEvent, listener);
  return () => {
    video.removeEventListener(loadingRunwayEvent, listener);
  };
}

/**
 * Loads as the follower of its group says: held while its group waits (holdLoadingWhenTold), and no
 * more than a runway while one in the group is behind (keepRunwayWhenTold), as far as hls.js was
 * set to otherwise. Returns the stopping.
 */
export function obeyGroupLoading(
  hls: Pick<Hls, "stopLoad" | "startLoad" | "config">,
  video: Pick<EventTarget, "addEventListener" | "removeEventListener">,
): () => void {
  const stops = [
    holdLoadingWhenTold(hls, video),
    keepRunwayWhenTold(hls, video, {
      maxBufferLength: hls.config.maxBufferLength,
      maxMaxBufferLength: hls.config.maxMaxBufferLength,
    }),
  ];
  return () => {
    for (const stop of stops) {
      stop();
    }
  };
}

/** The header in which a player says it loads where a seek took it (D284 §2). */
export const soughtHeader = "Orpheum-Sought";

/**
 * Whether the video loads where a seek took it, its person's or its group's, from its seeking until
 * it can play there: a request then says so (soughtHeader), and the server starts a run there sooner
 * than it does for another (D284 §2). The seek of a follower catching up (catchUpSeekEvent), and
 * hls.js's own steps over a gap (playerMovedItselfEvent), are no jumps. The owner: "We should be able
 * to tell from the controlling client whether it's a jump or not" (2026-10-12).
 */
export function markSeeks(video: Pick<EventTarget, "addEventListener" | "removeEventListener">): {
  readonly isSought: () => boolean;
  readonly stop: () => void;
} {
  let isSought = false;
  let isNextSeekNoJump = false;
  const listeners: Record<string, () => void> = {
    [catchUpSeekEvent]: () => {
      isNextSeekNoJump = true;
    },
    [playerMovedItselfEvent]: () => {
      isNextSeekNoJump = true;
    },
    seeking: () => {
      isSought = !isNextSeekNoJump;
      isNextSeekNoJump = false;
    },
    canplay: () => {
      isSought = false;
    },
    playing: () => {
      isSought = false;
    },
  };
  for (const [event, listener] of Object.entries(listeners)) {
    video.addEventListener(event, listener);
  }
  return {
    isSought: () => isSought,
    stop: () => {
      for (const [event, listener] of Object.entries(listeners)) {
        video.removeEventListener(event, listener);
      }
    },
  };
}

/**
 * Holds hls.js's loading while the follower of its group says to, and lets it go on from where the
 * video is when told (holdLoadingEvent): a ready player then leaves the server's upload to those its
 * group waits for. Returns the stopping.
 */
export function holdLoadingWhenTold(
  hls: Pick<Hls, "stopLoad" | "startLoad">,
  video: Pick<EventTarget, "addEventListener" | "removeEventListener">,
): () => void {
  const listener = (event: Event): void => {
    const hold = loadingHoldOf(event);
    if (hold?.isHeld === true) {
      hls.stopLoad();
    } else if (hold?.isHeld === false) {
      hls.startLoad(-1); // from where the video is
    }
  };
  video.addEventListener(holdLoadingEvent, listener);
  return () => {
    video.removeEventListener(holdLoadingEvent, listener);
  };
}

/**
 * Tells video, by playerMovedItselfEvent, each time hls.js moves it itself: stepping over a gap in
 * what it loaded, or nudging a stalled video on. hls.js sets the position and then says so, before
 * the browser fires seeking, so a Watch Together host's page knows that seek for the player's own.
 */
export function tellOwnMoves(hls: OwnMoves, video: Pick<EventTarget, "dispatchEvent">): void {
  hls.on(Hls.Events.ERROR, (_event, data) => {
    // What it moved over goes with the event, for the pause's or the seek's context (event-context.ts).
    if (data.details === Hls.ErrorDetails.BUFFER_SEEK_OVER_HOLE) {
      video.dispatchEvent(new CustomEvent(playerMovedItselfEvent, { detail: "hole" }));
    }
    if (data.details === Hls.ErrorDetails.BUFFER_NUDGE_ON_STALL) {
      video.dispatchEvent(new CustomEvent(playerMovedItselfEvent, { detail: "nudge" }));
    }
  });
}

/** What telling what hls.js loads needs of it: its qualities, its link estimate, its events. */
export interface LoadingHls {
  readonly levels: readonly { readonly bitrate: number }[];
  readonly bandwidthEstimate: number;
  on: Hls["on"];
}

/**
 * Tells onFailed why, when hls.js says the playback cannot go on, unless the player recovers from
 * it by itself (isRecovered).
 */
export function failOnFatal(
  hls: Pick<Hls, "on">,
  onFailed: (failure: PlaybackFailure) => void,
): void {
  hls.on(Hls.Events.ERROR, (_event, data) => {
    if (data.fatal && !isRecovered(data)) {
      onFailed(hlsErrorFailure(data));
    }
  });
}

/**
 * The link's speed hls.js measured, in kb/s, once it has loaded a chunk; undefined before. Until
 * then its estimate is its own starting value, which a phone's player reported as its link, 500
 * then 5000 kb/s, and the core remembered for its next playback (staging, 2026-10-07).
 */
export function measuredKbps(
  hls: Pick<LoadingHls, "bandwidthEstimate" | "on">,
): () => number | undefined {
  let hasMeasured = false;
  hls.on(Hls.Events.FRAG_LOADED, () => {
    hasMeasured = true;
  });
  return () =>
    hasMeasured && hls.bandwidthEstimate > 0 ? hls.bandwidthEstimate / 1000 : undefined;
}

/**
 * Tells video what hls.js loads, for a Watch Together player's log: each chunk, by
 * playerLoadedChunkEvent, with how long it took and how fast, and each quality it begins playing,
 * by playerSwitchedQualityEvent, with its link estimate then, once a chunk measured it. A viewer's iPhone froze every few
 * seconds and nothing said what it loaded, or how fast (staging, 2026-10-06).
 */
export function tellLoading(hls: LoadingHls, video: Pick<EventTarget, "dispatchEvent">): void {
  const linkEstimateKbps = measuredKbps(hls);
  hls.on(Hls.Events.FRAG_LOADED, (_event, data) => {
    const { frag } = data;
    const isVideo = frag.type === PlaylistLevelType.MAIN;
    if (!isVideo && frag.type !== PlaylistLevelType.AUDIO) {
      return;
    }
    const { loading, total } = frag.stats;
    const loadMs = Math.max(0, loading.end - loading.start);
    const loaded: ChunkLoaded = {
      track: isVideo ? "video" : "audio",
      loadMs,
      loadedKbps: loadMs > 0 ? (total * 8) / loadMs : 0,
      qualityKbps: isVideo ? (hls.levels[frag.level]?.bitrate ?? 0) / 1000 : 0,
      filmMs: frag.duration * 1000,
    };
    video.dispatchEvent(new CustomEvent(playerLoadedChunkEvent, { detail: loaded }));
  });
  hls.on(Hls.Events.LEVEL_SWITCHED, (_event, data) => {
    const switched: QualitySwitched = {
      qualityKbps: (hls.levels[data.level]?.bitrate ?? 0) / 1000,
      linkEstimateKbps: linkEstimateKbps(),
    };
    video.dispatchEvent(new CustomEvent(playerSwitchedQualityEvent, { detail: switched }));
  });
}

/**
 * How hls.js plays a prepared rendition: from its start. A rendition still being prepared has an
 * open playlist (EVENT, no ENDLIST yet), which hls.js reads as live and, left to itself, starts
 * near its newest segment: a film whose prepare had made 46 s began at 40 s (staging,
 * 2026-10-06). A person's place to resume is a seek after it loads, as for any rendition.
 */
export const preparedHlsConfig = { startPosition: 0 } as const;

/**
 * Plays an HLS master playlist in video: with hls.js where the page has MSE, else with the
 * element's own HLS, as on an iPhone. The URL was produced by the SDK; the requests carry the
 * media cookie, never a credential in the address (D154).
 */
export function playHls(
  video: HTMLVideoElement,
  url: string,
  options: {
    /** Called with why, as a code, when the playback cannot go on. */
    readonly onFailed: (failure: PlaybackFailure) => void;
    /** The audio rendition to start with; the playlist's default when omitted. */
    readonly startAudioRenditionIndex?: number;
  },
): HlsPlayback {
  const { onFailed } = options;
  const method = hlsMethod(hlsSupport(video));
  if (method === "native") {
    return playNative(video, url);
  }
  if (method === "none") {
    onFailed("hls_unsupported");
    return {
      setAudioRendition: () => undefined,
      destroy: () => undefined,
      estimateKbps: () => undefined,
    };
  }
  const hls = new Hls(preparedHlsConfig);
  failOnFatal(hls, onFailed);
  startWithAudio(hls, options.startAudioRenditionIndex ?? 0);
  tellOwnMoves(hls, video);
  tellLoading(hls, video);
  const estimateKbps = measuredKbps(hls);
  hls.loadSource(url);
  hls.attachMedia(video);
  return {
    setAudioRendition: (index) => {
      hls.audioTrack = index;
      // hls.js drops the buffered audio of the old mix, but the browser has already decoded a
      // few seconds of it ahead and would play those out first. Seeking to where it is makes it
      // discard them, so the new mix is heard at once, after a moment's pause.
      if (!video.paused || video.currentTime > 0) {
        video.dispatchEvent(new Event(playerMovedItselfEvent));
        // eslint-disable-next-line no-param-reassign, no-self-assign -- a media element seeks when currentTime is set; the same value drops what it decoded ahead
        video.currentTime = video.currentTime;
      }
    },
    destroy: () => {
      hls.destroy();
    },
    estimateKbps,
  };
}
