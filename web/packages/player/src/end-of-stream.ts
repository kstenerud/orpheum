// SPDX-License-Identifier: MPL-2.0

import Hls, { type BufferEOSData } from "hls.js";

/** How often the position is looked at. */
const lookEveryMs = 500;

/**
 * How near the end of what is loaded a still video must stand, and how near the playlist's end
 * what is loaded must reach, for it to be at its end: a few frames, more than the last frame or two
 * a browser holds back until it is told the stream has ended, less than any chunk.
 */
const atTheEndS = 0.5;

/** What ending the stream at its last frame needs of hls.js: the playlist, and its events. */
export interface EndingHls {
  readonly levels: readonly {
    readonly details?: { readonly live: boolean; readonly edge: number };
  }[];
  readonly currentLevel: number;
  trigger: (event: typeof Hls.Events.BUFFER_EOS, data: BufferEOSData) => unknown;
}

/** The part of a video element the end is watched by. */
export type EndingMedia = Pick<
  HTMLMediaElement,
  "currentTime" | "paused" | "seeking" | "ended" | "buffered"
>;

/**
 * Ends the media source when the video, asked to play, has stood still for a look at the end of a
 * whole film's playlist with all of it loaded, which hls.js does not do when a hole lies behind the
 * playhead: its own check measures the buffered range from its merged start with no hole allowed,
 * finds a "second range" past the hole and never calls endOfStream, so the video stands a frame or
 * two short of its end for good, never "ended", and no next episode comes (the Fire TV, 2026-11-02;
 * idiosyncrasies.md, "hls.js never ends a stream with a hole behind the playhead"). hls.js's own
 * BUFFER_EOS, of no one track, ends every track and then the source. Returns the stopping.
 */
export function endAtTheLastFrame(hls: EndingHls, media: EndingMedia): () => void {
  let stoodAtS: number | undefined;
  let isEnded = false;
  const timer = setInterval(() => {
    if (media.seeking) {
      isEnded = false; // a seek back reopens the source, and its end is ended again
    }
    const position = media.currentTime;
    if (media.paused || media.seeking || media.ended || stoodAtS !== position) {
      stoodAtS = media.paused || media.seeking || media.ended ? undefined : position;
      return;
    }
    if (!isEnded && isAtLoadedEnd(hls, media, position)) {
      isEnded = true;
      hls.trigger(Hls.Events.BUFFER_EOS, { type: null });
    }
  }, lookEveryMs);
  return () => {
    clearInterval(timer);
  };
}

/** Whether position is at the end of what is loaded, and that reaches a whole playlist's end. */
function isAtLoadedEnd(hls: EndingHls, media: EndingMedia, position: number): boolean {
  const details = hls.levels[hls.currentLevel]?.details;
  const { buffered } = media;
  if (details === undefined || details.live || buffered.length === 0) {
    return false;
  }
  const loadedEnd = buffered.end(buffered.length - 1);
  return loadedEnd >= details.edge - atTheEndS && position >= loadedEnd - atTheEndS;
}
