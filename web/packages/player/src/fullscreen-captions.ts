// SPDX-License-Identifier: MPL-2.0

/** A caption's times, in seconds of the video, as a TextTrackCue holds them. */
export interface CaptionTimes {
  startTime: number;
  endTime: number;
}

/** The text track the browser shows the captions with: a TextTrack, its cues those it loaded. */
export interface CaptionTrack {
  mode: TextTrackMode;
  readonly cues: Iterable<CaptionTimes> | null;
}

/**
 * The captions a video shows while it is in the browser's own fullscreen, where the drawn layer
 * over it is not seen (D254): the chosen track as WebVTT, which the browser draws itself. Hidden out
 * of fullscreen, where the drawn layer shows the same track as its author styled it. Each caption is
 * moved by the subtitle offset, later when it is positive, as the drawn layer is.
 */
export class FullscreenCaptions {
  readonly #track: CaptionTrack;
  #offsetMs: number;
  /** The offset the loaded captions are moved by now. */
  #appliedMs = 0;

  constructor(track: CaptionTrack, offsetMs: number) {
    this.#track = track;
    this.#offsetMs = offsetMs;
    // Hidden, not disabled: a disabled track's captions are never loaded.
    this.#track.mode = "hidden";
  }

  /** The browser loaded the captions, as the file has them: they are moved by the offset. */
  loaded(): void {
    this.#appliedMs = 0;
    this.#apply();
  }

  /** The video entered or left the browser's own fullscreen. */
  fullscreenChanged(isFullscreen: boolean): void {
    this.#track.mode = isFullscreen ? "showing" : "hidden";
  }

  /** Shows the captions this many milliseconds later, or earlier when negative. */
  setOffsetMs(offsetMs: number): void {
    this.#offsetMs = offsetMs;
    this.#apply();
  }

  #apply(): void {
    const cues = this.#track.cues;
    if (cues === null) {
      return;
    }
    const shiftSeconds = (this.#offsetMs - this.#appliedMs) / 1000;
    for (const cue of cues) {
      cue.startTime += shiftSeconds;
      cue.endTime += shiftSeconds;
    }
    this.#appliedMs = this.#offsetMs;
  }
}

/** What the browser's own fullscreen captions are made from: the track as WebVTT, and its name. */
export interface CaptionSource {
  readonly vttUrl: string;
  /** Its language, as BCP 47, or empty when not known. */
  readonly language: string;
  readonly label: string;
}

/** Whether the video itself is in the browser's own fullscreen, not the page around it. */
function isVideoFullscreen(video: HTMLVideoElement): boolean {
  return video.ownerDocument.fullscreenElement === video;
}

/**
 * Adds source to video as a text track the browser shows only while the video is in its own
 * fullscreen (D254): the fullscreen of its controls on Android and a computer, and the iPhone's
 * player, which is always fullscreen. Returns its captions and how to take them away.
 */
export function addFullscreenCaptions(
  video: HTMLVideoElement,
  source: CaptionSource,
  offsetMs: number,
): { readonly captions: FullscreenCaptions; readonly remove: () => void } {
  const element = video.ownerDocument.createElement("track");
  element.kind = "subtitles";
  element.label = source.label;
  if (source.language !== "") {
    element.srclang = source.language;
  }
  element.src = source.vttUrl;
  video.append(element);
  const captions = new FullscreenCaptions(element.track, offsetMs);
  const onLoad = (): void => {
    captions.loaded();
  };
  const onFullscreenChange = (): void => {
    captions.fullscreenChanged(isVideoFullscreen(video));
  };
  // The iPhone's player is not the page's fullscreen: it says so on the video, prefixed.
  const onIPhoneBegin = (): void => {
    captions.fullscreenChanged(true);
  };
  const onIPhoneEnd = (): void => {
    captions.fullscreenChanged(false);
  };
  element.addEventListener("load", onLoad);
  video.ownerDocument.addEventListener("fullscreenchange", onFullscreenChange);
  video.addEventListener("webkitbeginfullscreen", onIPhoneBegin);
  video.addEventListener("webkitendfullscreen", onIPhoneEnd);
  captions.fullscreenChanged(isVideoFullscreen(video));
  return {
    captions,
    remove: () => {
      element.removeEventListener("load", onLoad);
      video.ownerDocument.removeEventListener("fullscreenchange", onFullscreenChange);
      video.removeEventListener("webkitbeginfullscreen", onIPhoneBegin);
      video.removeEventListener("webkitendfullscreen", onIPhoneEnd);
      element.track.mode = "disabled";
      element.remove();
    },
  };
}
