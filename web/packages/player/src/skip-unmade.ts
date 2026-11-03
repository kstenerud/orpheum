// SPDX-License-Identifier: MPL-2.0

/**
 * A chunk the core cannot make, stepped over: the film plays up to it and on from the chunk after,
 * the page told what was skipped (the owner, 2026-11-03: "Can it just skip automatically … If it's
 * able to skip forward, and will skip more than 15 seconds (or simply can't play any further), show
 * a pop-up dialog as well"; toolkit.md item 12).
 */

import Hls, { type ErrorData, type Fragment } from "hls.js";

import { isNotMade, recoverFrom } from "./abr-controller.ts";
import { seekTo } from "./report.ts";

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
  readonly isWhereItStops?: boolean;
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
  const fields = Object.fromEntries(Object.entries(detail));
  const { fromMs, toMs } = fields;
  if (typeof fromMs !== "number" || typeof toMs !== "number") {
    return undefined;
  }
  return fields["isWhereItStops"] === true
    ? { fromMs, toMs, isWhereItStops: true }
    : { fromMs, toMs };
}

/**
 * Steps over each picture chunk the core answers with 422, cannot_convert (cannotMakeStatus), at
 * once: the core cannot make it in any quality, and hls.js is kept from trying another
 * (abr-controller.ts's keepTheQualityAtNotMade), a fault of the file being no reason to step down (the owner,
 * 2026-11-03). Its loading is stopped and the error marked recovered, so failOnFatal, told after,
 * passes it by; the film plays what it has up to the chunk; there it is moved into the next chunk and
 * loaded on from there, and the video is sent a playerSkippedEvent. A chunk after it that fails too
 * lengthens the same stretch. A stretch that would pass skipsAtMostMs, or the film's end, fails
 * (cannot_make), the video sent a playerSkippedEvent that says so (isWhereItStops). A seek of the
 * person's before the move is theirs: loading goes on from where they went. Returns the stopping.
 */
export function skipChunksNotMade(
  hls: Pick<Hls, "on" | "off" | "startLoad" | "stopLoad">,
  video: SkippingVideo,
  note: (note: string) => void,
): () => void {
  const skipping = new Skipping(hls, video, note);
  const onError = (_event: unknown, data: ErrorData): void => {
    const { frag } = data;
    if (frag !== undefined && isNotMade(data)) {
      skipping.cannotMake(data, frag);
    }
  };
  const moveIfArrived = (): void => {
    skipping.moveIfArrived();
  };
  const onSeeking = (): void => {
    skipping.seeking();
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

/** A chunk as hls.js gives it with its error: where in the film it starts, and how long it is. */
type ChunkSpan = Pick<Fragment, "start" | "duration">;

/** The stretch a playback is stepping over, and the move it waits to make (skipChunksNotMade). */
class Skipping {
  #fromS: number | undefined;
  #toS: number | undefined;
  #waiting: { readonly atS: number; readonly toS: number } | undefined;
  #isOwnMove = false;
  readonly #hls: Pick<Hls, "startLoad" | "stopLoad">;
  readonly #video: SkippingVideo;
  readonly #note: (note: string) => void;

  constructor(
    hls: Pick<Hls, "startLoad" | "stopLoad">,
    video: SkippingVideo,
    note: (note: string) => void,
  ) {
    this.#hls = hls;
    this.#video = video;
    this.#note = note;
  }

  /** Whether chunk starts where the stretch being stepped over ends. */
  #continuesTheStretch(chunk: ChunkSpan): boolean {
    return this.#toS !== undefined && Math.abs(chunk.start - this.#toS) < arrivedWithinS;
  }

  /** The core cannot make chunk: it is stepped over once the film reaches it, or the film stops. */
  cannotMake(data: Pick<ErrorData, "fatal">, chunk: ChunkSpan): void {
    this.#hls.stopLoad(); // where hls.js would ask for it again

    const fromS =
      this.#continuesTheStretch(chunk) && this.#fromS !== undefined ? this.#fromS : chunk.start;
    const toS = chunk.start + chunk.duration;
    const { duration } = this.#video;
    const isPastTheEnd = Number.isFinite(duration) && toS >= duration - arrivedWithinS;
    if (isPastTheEnd || (toS - fromS) * 1000 > skipsAtMostMs) {
      this.#tell({ fromS, toS, isWhereItStops: true });
      // eslint-disable-next-line no-param-reassign -- so failOnFatal ends the playback, saying why
      data.fatal = true;
      return;
    }
    recoverFrom(data);
    this.#fromS = fromS;
    this.#waiting = { atS: chunk.start, toS };
    this.moveIfArrived();
  }

  /** Moves the film past the chunk not made, once it has played up to it. */
  moveIfArrived(): void {
    const waiting = this.#waiting;
    if (waiting === undefined || this.#video.currentTime < waiting.atS - arrivedWithinS) {
      return;
    }
    const { toS } = waiting;
    this.#waiting = undefined;
    this.#toS = toS;
    this.#isOwnMove = true;
    // hls.js loads from where the video is, not where startLoad says, and goes back for the chunk
    // before where the next one's picture starts after the video (staging, 2026-11-03): moved first,
    // into the next chunk, loading begins past the chunk not made.
    const onS = toS + arrivedWithinS;
    seekTo(this.#video, onS * 1000);
    this.#hls.startLoad(onS);
    this.#tell({ fromS: this.#fromS ?? toS, toS, isWhereItStops: false });
  }

  /** A seek of the person's ends the stretch, and loading goes on from where they went. */
  seeking(): void {
    if (this.#isOwnMove) {
      this.#isOwnMove = false;
      return;
    }
    this.#fromS = undefined;
    this.#toS = undefined;
    if (this.#waiting !== undefined) {
      this.#waiting = undefined;
      this.#hls.startLoad(this.#video.currentTime); // loading stopped at the chunk
    }
  }

  #tell(stretchS: {
    readonly fromS: number;
    readonly toS: number;
    readonly isWhereItStops: boolean;
  }): void {
    const { fromS, toS, isWhereItStops } = stretchS;
    const what = isWhereItStops ? "cannot step over" : "stepped over";
    this.#note(`${what} chunks the core cannot make, ${fromS.toFixed(1)} s to ${toS.toFixed(1)} s`);
    const stretch: SkippedStretch = {
      fromMs: Math.round(fromS * 1000),
      toMs: Math.round(toS * 1000),
      ...(isWhereItStops ? { isWhereItStops } : {}),
    };
    this.#video.dispatchEvent(new CustomEvent(playerSkippedEvent, { detail: stretch }));
  }
}
