// SPDX-License-Identifier: MPL-2.0

import JASSUB from "jassub";

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
 */
export function drawSubtitles(
  video: HTMLVideoElement,
  assText: string,
  offsetMs: number,
): SubtitleLayer {
  const renderer = new JASSUB({
    video,
    subContent: assText,
    timeOffset: jassubTimeOffset(offsetMs),
    queryFonts: false,
  });
  return {
    setOffsetMs: (newOffsetMs) => {
      renderer.timeOffset = jassubTimeOffset(newOffsetMs);
      // A paused video shows no new frame, so the change is drawn now.
      void renderer.resize(true);
    },
    destroy: () => {
      void renderer.destroy();
    },
  };
}
