// SPDX-License-Identifier: MPL-2.0

import JASSUB from "jassub";

import { addFullscreenCaptions, type CaptionSource } from "./fullscreen-captions.ts";

/** A subtitle track drawn over a video, as its author styled it. */
export interface SubtitleLayer {
  /** Shows the subtitles this many milliseconds later, or earlier when negative. */
  readonly setOffsetMs: (offsetMs: number) => void;
  /** Stops drawing and removes the layer. */
  readonly destroy: () => void;
}

/**
 * The time JASSUB adds to the video's before drawing, in seconds, for a subtitle offset in
 * milliseconds. Showing the subtitles later means drawing, at each moment, the one due earlier.
 */
export function jassubTimeOffset(offsetMs: number): number {
  return -offsetMs / 1000;
}

/**
 * Draws an ASS track, assText, over video with JASSUB (D217 §4), offset by offsetMs. The text
 * was fetched through the SDK. Fonts come from the track's own styles and JASSUB's bundled default; the page
 * never asks the browser for its local fonts, which would prompt the viewer, nor any font service.
 * The drawn layer is not seen in the browser's own fullscreen, so the same track, captions, is
 * given to the browser to show there (D254).
 */
export function drawSubtitles(
  video: HTMLVideoElement,
  track: { readonly assText: string; readonly captions: CaptionSource },
  offsetMs: number,
): SubtitleLayer {
  const renderer = new JASSUB({
    video,
    subContent: track.assText,
    timeOffset: jassubTimeOffset(offsetMs),
    queryFonts: false,
  });
  const fullscreen = addFullscreenCaptions(video, track.captions, offsetMs);
  return {
    setOffsetMs: (newOffsetMs) => {
      renderer.timeOffset = jassubTimeOffset(newOffsetMs);
      fullscreen.captions.setOffsetMs(newOffsetMs);
      // A paused video shows no new frame, so the change is drawn now.
      void renderer.resize(true);
    },
    destroy: () => {
      void renderer.destroy();
      fullscreen.remove();
    },
  };
}
