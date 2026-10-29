// SPDX-License-Identifier: MPL-2.0

/**
 * The chunked player's bitrate controller inside hls.js (D265 §1): a subclass of hls.js's own,
 * which hands every choice to a Choosing (abr.ts) and feeds hls.js's estimator for its other
 * controllers to read.
 */

import {
  AbrController,
  ErrorDetails,
  Events,
  PlaylistLevelType,
  type Fragment,
  type FragBufferedData,
  type FragLoadedData,
  type FragLoadingData,
  type ErrorData,
  type LevelLoadedData,
} from "hls.js";
import type Hls from "hls.js";

import { Choosing } from "./abr.ts";
import { arrivalMs } from "./abr-choice.ts";
import type { ChoosingOptions } from "./abr-options.ts";
import { tooEarlyStatus } from "./failure.ts";

/** Fatal errors the player recovers from by itself, which are no failure (failOnFatal). */
const recovered = new WeakSet<object>();

/** Whether a fatal error is one the player recovers from by itself, giving a quality up (D266). */
export function isRecovered(data: object): boolean {
  return recovered.has(data);
}

/** Whether hls.js's error is the core's 425 for a chunk, being made, which hls.js asks for again. */
export function isBeingMade(data: Pick<ErrorData, "details" | "response">): boolean {
  return data.details === ErrorDetails.FRAG_LOAD_ERROR && data.response?.code === tooEarlyStatus;
}

/** Whether hls.js's error is a chunk's load failing or timing out, the core's 425 aside. */
export function isLoadFailure(data: Pick<ErrorData, "details" | "response">): boolean {
  return (
    (data.details === ErrorDetails.FRAG_LOAD_ERROR ||
      data.details === ErrorDetails.FRAG_LOAD_TIMEOUT) &&
    !isBeingMade(data)
  );
}

/**
 * Builds hls's video's decoding anew (recoverMediaError), and starts its loading again from positionS
 * once the new media source is attached. hls.js restarts only a loader still stopped then, and a
 * chunk the detach aborted marks the picture's loader idle while detached (resetFragmentLoading, with
 * no chunk current): at 0:00 its loader then never loaded again, from where it had loaded to, the
 * video holding nothing (a Mac's Firefox giving up A Woman of Paris's own picture, the owner's test
 * on staging, 2026-10-18).
 */
export function loadAgainFrom(
  hls: Pick<Hls, "once" | "recoverMediaError" | "startLoad">,
  positionS: number,
): void {
  hls.once(Events.MEDIA_ATTACHED, () => {
    hls.startLoad(positionS);
  });
  hls.recoverMediaError();
}

/** Whether a chunk is one of picture, which alone measures the link and is chosen among. */
function isPicture(frag: Fragment): boolean {
  return frag.type === PlaylistLevelType.MAIN && frag.sn !== "initSegment";
}

/**
 * Each controller's listeners on hls.js, kept outside it: hls.js registers them from its own
 * constructor, before the controller's fields exist.
 */
const listenersOf = new WeakMap<
  OrpheumAbrController,
  ReturnType<OrpheumAbrController["listeners"]>
>();

/**
 * hls.js's own bitrate controller, its estimator still fed for its other controllers to read, with
 * every choice handed to a Choosing (D265), which chooses by its own estimate (D352 §2). A chunk
 * given up is aborted as hls.js's own controller does, and hls.js loads again from where it was.
 */
class OrpheumAbrController extends AbrController {
  readonly choosing: Choosing<Fragment>;

  constructor(hls: Hls, options: ChoosingOptions) {
    super(hls);
    // hls.js's own giving up of a chunk on its way is off: choosing's is the one (D265 §3, D323). It
    // timed a chunk from its first ask, so one the core answered 425 while it made it looked stalled,
    // and with little loaded ahead it switched down to the quality it reckoned would arrive in time,
    // every quality waiting as long: from the twin to 360p on a 120 Mb/s link, mid-film (the owner's
    // Firefox on nas.example, 2026-10-20). Its check runs from its own timer, read when each chunk loads.
    Reflect.set(this, "_abandonRulesCheck", () => undefined);
    this.choosing = new Choosing<Fragment>(hls, options, {
      measureGivenUp: (durationMs, bytes) => {
        this.bwEstimator.sample(durationMs, bytes);
      },
      giveUp: (frag) => {
        frag.abortRequests();
        hls.trigger(Events.FRAG_LOAD_EMERGENCY_ABORTED, { frag, part: null, stats: frag.stats });
      },
      loadAgain: (positionS) => {
        // A new MediaSource, so the browser's decoding is built anew: dropping the buffer alone
        // left Firefox on macOS stuck after Flow's HEVC, every lower quality frozen too, for two
        // minutes (the owner's test, 2026-10-08).
        loadAgainFrom(hls, positionS);
      },
    });
  }

  /** What it listens to: a picture chunk's loading, loading and buffering, and the quality switching. */
  listeners(): {
    readonly loading: (event: Events.FRAG_LOADING, data: FragLoadingData) => void;
    readonly loaded: (event: Events.FRAG_LOADED, data: FragLoadedData) => void;
    readonly buffered: (event: Events.FRAG_BUFFERED, data: FragBufferedData) => void;
    readonly levelLoaded: (event: Events.LEVEL_LOADED, data: LevelLoadedData) => void;
    readonly switched: () => void;
    readonly error: (event: Events.ERROR, data: ErrorData) => void;
  } {
    return {
      loading: (event, data) => {
        this.onFragLoading(event, data);
      },
      loaded: (event, data) => {
        this.onFragLoaded(event, data);
      },
      buffered: (event, data) => {
        this.onFragBuffered(event, data);
      },
      // hls.js's own: the estimate's half-lives, and a timed-out chunk's sample.
      levelLoaded: (event, data) => {
        this.onLevelLoaded(event, data);
      },
      switched: () => {
        this.choosing.switched();
      },
      error: (event, data) => {
        this.onError(event, data);
        this.passAHole(data);
        const { frag } = data;
        if (frag !== undefined && isPicture(frag) && isBeingMade(data)) {
          this.choosing.beingMade(frag);
        } else if (frag !== undefined && isPicture(frag) && isLoadFailure(data)) {
          this.choosing.failed(frag, data.details);
        }
        if (
          data.details === ErrorDetails.BUFFER_APPEND_NO_PROGRESS &&
          frag !== undefined &&
          isPicture(frag)
        ) {
          this.choosing.notHeld(frag.level);
        }
      },
    };
  }

  /**
   * At a hole hls.js could not step over, which it says is fatal, gives the quality up and loads
   * again below it, rather than letting the playback end (D266): the error is marked recovered, so
   * failOnFatal passes it by, and the giving up waits for hls.js to have finished with the error.
   */
  passAHole(data: ErrorData): void {
    const choosing = this.choosing;
    const media = this.hls.media;
    if (
      data.details !== ErrorDetails.BUFFER_SEEK_OVER_HOLE ||
      !data.fatal ||
      media === null ||
      !choosing.canGiveUpAtAHole()
    ) {
      return;
    }
    recovered.add(data);
    const positionS = media.currentTime;
    setTimeout(() => {
      choosing.giveUpAtAHole(positionS);
      this.hls.startLoad(positionS);
    }, 0);
  }

  protected override registerListeners(): void {
    const { hls } = this;
    const listeners = this.listeners();
    listenersOf.set(this, listeners);
    hls.on(Events.FRAG_LOADING, listeners.loading);
    hls.on(Events.FRAG_LOADED, listeners.loaded);
    hls.on(Events.FRAG_BUFFERED, listeners.buffered);
    hls.on(Events.LEVEL_LOADED, listeners.levelLoaded);
    hls.on(Events.LEVEL_SWITCHED, listeners.switched);
    hls.on(Events.ERROR, listeners.error);
  }

  protected override unregisterListeners(): void {
    const { hls } = this;
    const listeners = listenersOf.get(this);
    if (listeners === undefined) {
      return;
    }
    hls.off(Events.FRAG_LOADING, listeners.loading);
    hls.off(Events.FRAG_LOADED, listeners.loaded);
    hls.off(Events.FRAG_BUFFERED, listeners.buffered);
    hls.off(Events.LEVEL_LOADED, listeners.levelLoaded);
    hls.off(Events.LEVEL_SWITCHED, listeners.switched);
    hls.off(Events.ERROR, listeners.error);
  }

  protected override onFragLoading(_event: Events.FRAG_LOADING, data: FragLoadingData): void {
    if (isPicture(data.frag)) {
      this.choosing.loading(data.frag);
    }
  }

  protected override onFragLoaded(event: Events.FRAG_LOADED, data: FragLoadedData): void {
    super.onFragLoaded(event, data); // its time to first byte
    if (isPicture(data.frag)) {
      this.choosing.loaded(data.frag);
    }
  }

  /**
   * A chunk of picture that measures the link is sampled over the time its bytes took to arrive
   * (arrivalMs), not hls.js's from the ask to the chunk parsed, which counts the wait for the first
   * byte as the link's.
   */
  protected override onFragBuffered(_event: Events.FRAG_BUFFERED, data: FragBufferedData): void {
    const { frag } = data;
    if (isPicture(frag) && !frag.stats.aborted && this.choosing.measures(frag)) {
      this.bwEstimator.sample(arrivalMs(frag.stats.loading), frag.stats.loaded);
    }
  }

  override get firstAutoLevel(): number {
    return this.choosing.firstLevel();
  }

  override get forcedAutoLevel(): number {
    return this.choosing.forcedLevel();
  }

  override get nextAutoLevel(): number {
    return this.choosing.nextLevel();
  }

  override set nextAutoLevel(level: number) {
    this.choosing.setLevel(level);
  }

  override destroy(): void {
    this.choosing.stop();
    super.destroy();
  }
}

/** A playback's bitrate controller, made for hls.js's config, and its choosing once hls.js made it. */
export interface OrpheumAbr {
  readonly controller: typeof AbrController;
  readonly choosing: () => Choosing<Fragment> | undefined;
}

/** The bitrate controller of one chunked playback, choosing by options (D265). */
export function orpheumAbr(options: ChoosingOptions): OrpheumAbr {
  let made: Choosing<Fragment> | undefined;
  class PlaybackAbrController extends OrpheumAbrController {
    constructor(hls: Hls) {
      super(hls, options);
      made = this.choosing;
    }
  }
  return { controller: PlaybackAbrController, choosing: () => made };
}
