// SPDX-License-Identifier: MPL-2.0

import Hls from "hls.js";

/** A prepared rendition playing in a video element. */
export interface Playback {
  /** Plays the audio rendition at index, in the master playlist's order. */
  readonly setAudioRendition: (index: number) => void;
  /** Stops playing and lets go of the element. */
  readonly destroy: () => void;
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
export type HlsWay = "hls.js" | "native" | "none";

/** The way to play HLS: hls.js wherever MSE is, since it switches audio renditions itself. */
export function hlsWay(support: HlsSupport): HlsWay {
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
export function playNative(target: NativeTarget, url: string): Playback {
  target.setAttribute("src", url);
  return {
    // Safari switches its own tracks from its controls; choosing one is left to them.
    setAudioRendition: () => undefined,
    destroy: () => {
      target.removeAttribute("src");
      target.load();
    },
  };
}

/**
 * Plays an HLS master playlist in video: with hls.js where the page has MSE, else with the
 * element's own HLS, as on an iPhone. The URL was produced by the SDK; the requests carry the
 * media cookie, never a credential in the address (D154).
 */
export function playHls(
  video: HTMLVideoElement,
  url: string,
  failed: (why: string) => void,
): Playback {
  const way = hlsWay(hlsSupport(video));
  if (way === "native") {
    return playNative(video, url);
  }
  if (way === "none") {
    failed("this browser cannot play HLS");
    return { setAudioRendition: () => undefined, destroy: () => undefined };
  }
  const hls = new Hls();
  hls.on(Hls.Events.ERROR, (_event, data) => {
    if (data.fatal) {
      failed(data.details);
    }
  });
  hls.loadSource(url);
  hls.attachMedia(video);
  return {
    setAudioRendition: (index) => {
      hls.audioTrack = index;
    },
    destroy: () => {
      hls.destroy();
    },
  };
}
