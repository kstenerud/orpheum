// SPDX-License-Identifier: MPL-2.0

/**
 * The frames a video showed and dropped, as its browser counts them (getVideoPlaybackQuality),
 * where it does: a stutter on a Fire TV showed in no log (the owner, 2026-10-28).
 */

/** The part of a video its browser counts its frames on, where it has the count. */
export interface FrameCountingMedia {
  readonly getVideoPlaybackQuality?: () => Pick<
    VideoPlaybackQuality,
    "totalVideoFrames" | "droppedVideoFrames"
  >;
}

/** Frames shown or dropped, and those dropped. */
export interface FramesShown {
  readonly total: number;
  readonly dropped: number;
}

/** The frames media showed and dropped since it began, or undefined where its browser counts none. */
export function framesShownOf(media: FrameCountingMedia): FramesShown | undefined {
  if (typeof media.getVideoPlaybackQuality !== "function") {
    return undefined;
  }
  const quality = media.getVideoPlaybackQuality();
  return { total: quality.totalVideoFrames, dropped: quality.droppedVideoFrames };
}

/**
 * The frames shown between since and now, both counted from the video's start; a count that went
 * back, as the browser starts it again for a new source, is counted from zero.
 */
export function framesBetween(since: FramesShown | undefined, now: FramesShown): FramesShown {
  if (since === undefined || now.total < since.total || now.dropped < since.dropped) {
    return now;
  }
  return { total: now.total - since.total, dropped: now.dropped - since.dropped };
}
