// SPDX-License-Identifier: MPL-2.0

/**
 * A chunk the core cannot make, stepped over: the film plays up to it and on from the chunk after,
 * the page told what was skipped (the owner, 2026-11-03: "Can it just skip automatically … If it's
 * able to skip forward, and will skip more than 15 seconds (or simply can't play any further), show
 * a pop-up dialog as well"; toolkit.md item 12).
 */

import Hls, { type ErrorData } from "hls.js";

import { recoverFrom } from "./abr-controller.ts";
import { cannotMakeStatus } from "./failure.ts";

/** The event a video is sent each time its player steps over chunks the core cannot make. */
export const playerSkippedEvent = "orpheumplayerskipped";

/**
 * A stretch of the film stepped over, from where it starts to where the film plays on, in ms of the
 * film: chunks in a row the core could not make are one stretch, sent again as it grows, its fromMs
 * the same.
 */
export interface SkippedStretch {
  readonly fromMs: number;
  readonly toMs: number;
  /**
   * Whether the film cannot go on past it: stepping over would pass skipsAtMostMs or the film's
   * end, and the playback ends (cannot_make).
   */
  readonly cannotGoOn?: boolean;
}

/**
 * The longest stretch stepped over: the owner's cap (2026-11-03, agreeing to the lead's proposal),
 * past which the playback ends as one that cannot go on.
 */
export const skipsAtMostMs = 120_000;

/**
 * How near the video's position comes to a chunk not made before it is stepped over, in seconds: a
 * video standing at the end of what is loaded stands a picture or two short of the next chunk.
 */
const arrivedWithinS = 0.5;

/** What stepping over a chunk needs of a video: where it is, the film's length, and its events. */
export type SkippingVideo = Pick<
  HTMLVideoElement,
  "currentTime" | "duration" | "addEventListener" | "removeEventListener" | "dispatchEvent"
>;

/** A SkippedStretch an event carries, or undefined where it carries none. */
export function skippedStretchOf(event: Event): SkippedStretch | undefined {
  if (!(event instanceof CustomEvent)) {
    return undefined;
  }
  const detail: unknown = event.detail;
  if (typeof detail !== "object" || detail === null) {
    return undefined;
  }
  const { fromMs, toMs, cannotGoOn } = Object.fromEntries(Object.entries(detail));
  if (typeof fromMs !== "number" || typeof toMs !== "number") {
    return undefined;
  }
  return cannotGoOn === true ? { fromMs, toMs, cannotGoOn } : { fromMs, toMs };
}

/**
 * Steps over each chunk the core answers with 422, cannot_convert (cannotMakeStatus): the error is
 * marked recovered, so failOnFatal, told after, passes it by; the film plays what it has up to the
 * chunk; there it is moved to the chunk's end and loaded on from there, and the video is sent a
 * playerSkippedEvent. A chunk after it that fails too lengthens the same stretch. A stretch that
 * would pass skipsAtMostMs, or the film's end, is left to fail (cannot_make), the video sent a
 * playerSkippedEvent that says so (cannotGoOn). A seek of the person's before the move is theirs:
 * loading goes on from where they went. Returns the stopping.
 */
export function skipChunksNotMade(
  hls: Pick<Hls, "on" | "off" | "startLoad">,
  video: SkippingVideo,
  note: (note: string) => void,
): () => void {
  let stretchFromS: number | undefined;
  let stretchToS: number | undefined;
  let waiting: { readonly atS: number; readonly toS: number } | undefined;
  let isOwnMove = false;
  const tell = (fromS: number, toS: number, cannotGoOn: boolean): void => {
    const what = cannotGoOn ? "cannot step over" : "stepped over";
    note(`${what} chunks the core cannot make, ${fromS.toFixed(1)} s to ${toS.toFixed(1)} s`);
    const stretch: SkippedStretch = {
      fromMs: Math.round(fromS * 1000),
      toMs: Math.round(toS * 1000),
      ...(cannotGoOn ? { cannotGoOn } : {}),
    };
    video.dispatchEvent(new CustomEvent(playerSkippedEvent, { detail: stretch }));
  };
  const moveIfArrived = (): void => {
    if (waiting === undefined || video.currentTime < waiting.atS - arrivedWithinS) {
      return;
    }
    const { toS } = waiting;
    waiting = undefined;
    stretchToS = toS;
    isOwnMove = true;
    hls.startLoad(toS);
    video.currentTime = toS;
    tell(stretchFromS ?? toS, toS, false);
  };
  const onError = (_event: unknown, data: ErrorData): void => {
    const { frag } = data;
    if (!data.fatal || data.response?.code !== cannotMakeStatus || frag === undefined) {
      return;
    }
    const isSameStretch =
      stretchToS !== undefined && Math.abs(frag.start - stretchToS) < arrivedWithinS;
    const fromS = isSameStretch && stretchFromS !== undefined ? stretchFromS : frag.start;
    const toS = frag.start + frag.duration;
    const isPastTheEnd = Number.isFinite(video.duration) && toS >= video.duration - arrivedWithinS;
    if (isPastTheEnd || (toS - fromS) * 1000 > skipsAtMostMs) {
      tell(fromS, toS, true); // failOnFatal ends the playback, saying why
      return;
    }
    recoverFrom(data);
    stretchFromS = fromS;
    waiting = { atS: frag.start, toS };
    moveIfArrived();
  };
  const onSeeking = (): void => {
    if (isOwnMove) {
      isOwnMove = false;
      return;
    }
    stretchFromS = undefined;
    stretchToS = undefined;
    if (waiting !== undefined) {
      waiting = undefined;
      hls.startLoad(video.currentTime); // loading stopped at the chunk: the person went elsewhere
    }
  };
  hls.on(Hls.Events.ERROR, onError);
  video.addEventListener("timeupdate", moveIfArrived);
  video.addEventListener("waiting", moveIfArrived);
  video.addEventListener("seeking", onSeeking);
  return () => {
    hls.off(Hls.Events.ERROR, onError);
    video.removeEventListener("timeupdate", moveIfArrived);
    video.removeEventListener("waiting", moveIfArrived);
    video.removeEventListener("seeking", onSeeking);
  };
}
