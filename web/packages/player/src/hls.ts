// SPDX-License-Identifier: MPL-2.0

import { playerMovedItselfEvent } from "@orpheum/sdk";
import Hls, { type ErrorData } from "hls.js";

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
    destroy: () => {
      target.removeAttribute("src");
      target.load();
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
 * Tells video, by playerMovedItselfEvent, each time hls.js moves it itself: stepping over a gap in
 * what it loaded, or nudging a stalled video on. hls.js sets the position and then says so, before
 * the browser fires seeking, so a Watch Together host's page knows that seek for the player's own.
 */
export function tellOwnMoves(hls: OwnMoves, video: Pick<EventTarget, "dispatchEvent">): void {
  hls.on(Hls.Events.ERROR, (_event, data) => {
    if (
      data.details === Hls.ErrorDetails.BUFFER_SEEK_OVER_HOLE ||
      data.details === Hls.ErrorDetails.BUFFER_NUDGE_ON_STALL
    ) {
      video.dispatchEvent(new Event(playerMovedItselfEvent));
    }
  });
}

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
  const hls = new Hls();
  hls.on(Hls.Events.ERROR, (_event, data) => {
    if (data.fatal) {
      onFailed(hlsErrorFailure(data));
    }
  });
  startWithAudio(hls, options.startAudioRenditionIndex ?? 0);
  tellOwnMoves(hls, video);
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
    estimateKbps: () => (hls.bandwidthEstimate > 0 ? hls.bandwidthEstimate / 1000 : undefined),
  };
}
