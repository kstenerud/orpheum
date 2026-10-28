// SPDX-License-Identifier: MPL-2.0

import { diagnosisOf, rangesText } from "./diagnosis.ts";
import { framesShownOf, type FrameCountingMedia } from "./frames.ts";

/**
 * How long a video asked to play may keep its position without saying it waits before it is told
 * of: a phone's Brave said "playing" at 0:00 and stood there, sending no waiting, so no stall was
 * told and nothing said why (the owner's friend, 2026-10-12).
 */
export const standsStillReportAfterMs = 5000;

/** How often the position is looked at. */
const lookEveryMs = 1000;

/** The longest of the texts a stand-still's report carries, as the server takes them. */
const rangesTextMost = 400;

/** What the video showed as it stood still, for the server's log: every field says why it may. */
export interface Standstill {
  readonly positionMs: number;
  readonly forMs: number;
  /** HTMLMediaElement's readyState and networkState. */
  readonly readyState: number;
  readonly networkState: number;
  /** What the element holds, as "from-to" ranges in seconds, and a chunked playback's picture and sound. */
  readonly buffered: string;
  readonly videoBuffered?: string;
  readonly audioBuffered?: string;
  readonly lastError: string;
  /** The picture's size, 0 until a frame is decoded, and the frames decoded and dropped. */
  readonly videoWidth: number;
  readonly videoHeight: number;
  readonly decodedFrames?: number;
  readonly droppedFrames?: number;
  readonly isMuted: boolean;
  readonly volume: number;
  readonly isPageHidden: boolean;
}

/** The part of a video element a stand-still is watched and described by. */
export type StandingMedia = Pick<
  HTMLVideoElement,
  | "currentTime"
  | "paused"
  | "seeking"
  | "ended"
  | "readyState"
  | "networkState"
  | "buffered"
  | "error"
  | "videoWidth"
  | "videoHeight"
  | "muted"
  | "volume"
> &
  FrameCountingMedia;

/** What the video shows as it has stood still forMs, for the server's log. */
export function standstillOf(
  media: StandingMedia,
  forMs: number,
  isPageHidden: boolean,
): Standstill {
  const diagnosis = diagnosisOf(media);
  const frames = framesShownOf(media);
  const elementError = media.error;
  return {
    positionMs: Math.max(0, Math.round(media.currentTime * 1000)),
    forMs: Math.round(forMs),
    readyState: media.readyState,
    networkState: media.networkState,
    buffered: rangesText(media.buffered).slice(-rangesTextMost),
    ...(diagnosis === undefined
      ? {}
      : {
          videoBuffered: diagnosis.videoBuffered.slice(-rangesTextMost),
          audioBuffered: diagnosis.audioBuffered.slice(-rangesTextMost),
        }),
    lastError:
      diagnosis?.lastError ??
      (elementError === null
        ? ""
        : `media error ${String(elementError.code)}: ${elementError.message}`.slice(0, 200)),
    videoWidth: media.videoWidth,
    videoHeight: media.videoHeight,
    ...(frames === undefined ? {} : { decodedFrames: frames.total, droppedFrames: frames.dropped }),
    isMuted: media.muted,
    volume: media.volume,
    isPageHidden,
  };
}

/**
 * Tells onStill, once for each place, when the video, asked to play and not seeking, has kept its
 * position for standsStillReportAfterMs: what it showed then, which says whether it had the film,
 * decoded a frame or met an error. It is a report only; the stall watcher and the quality controller
 * act. Returns the stopping.
 */
export function watchStandingStill(
  media: StandingMedia,
  onStill: (still: Standstill) => void,
  isPageHidden: () => boolean = () => document.visibilityState === "hidden",
): () => void {
  let still: { positionS: number; forMs: number; isTold: boolean } | undefined;
  const timer = setInterval(() => {
    if (media.paused || media.seeking || media.ended) {
      still = undefined;
      return;
    }
    if (still?.positionS !== media.currentTime) {
      still = { positionS: media.currentTime, forMs: 0, isTold: false };
      return;
    }
    still.forMs += lookEveryMs;
    if (!still.isTold && still.forMs >= standsStillReportAfterMs) {
      still.isTold = true;
      onStill(standstillOf(media, still.forMs, isPageHidden()));
    }
  }, lookEveryMs);
  return () => {
    clearInterval(timer);
  };
}
