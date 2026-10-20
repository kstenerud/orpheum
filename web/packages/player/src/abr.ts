// SPDX-License-Identifier: MPL-2.0

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
  type HlsProgressivePerformanceTiming,
  type LevelLoadedData,
  type LoaderStats,
} from "hls.js";
import type Hls from "hls.js";

import {
  giveUpFor,
  isLoadedWhereItStands,
  isUnplayable,
  keptOwnPicture,
  ownPictureLevel,
  commonStartForMs,
  commonStartLevel,
  isWaitingToPlay,
  measuresTheLink,
  startingLevel,
  steadyLevel,
  underCeiling,
  voiceCeiling,
  type ChoiceLevel,
} from "./abr-choice.ts";
import { tooEarlyStatus } from "./failure.ts";
import { nextCap } from "./step-up.ts";

/** Fatal errors the player recovers from by itself, which are no failure (failOnFatal). */
const recovered = new WeakSet<object>();

/** Whether a fatal error is one the player recovers from by itself, giving a quality up (D266). */
export function isRecovered(data: object): boolean {
  return recovered.has(data);
}

/** How far, in seconds, a video's position must move to be playing, not standing still. */
const movedAtLeastS = 0.1;

/** How often the controller looks at a chunk on its way (D265 §3). */
export const lookAtChunkEveryMs = 500;

/** How often it looks at whether it may step up a quality (D247 §9). */
export const lookForStepUpEveryMs = 10_000;

/**
 * How far above its quality's rate, its BANDWIDTH, a chunk's download is paced while voice is
 * live: enough that a busy chunk still arrives well before it plays (D262 §2).
 */
export const paceAboveQuality = 1.25;

/** Whether hls.js's error is the core's 425 for a chunk, being made, which hls.js asks for again. */
export function isBeingMade(data: Pick<ErrorData, "details" | "response">): boolean {
  return data.details === ErrorDetails.FRAG_LOAD_ERROR && data.response?.code === tooEarlyStatus;
}

/** A chunk as choosing follows it: hls.js's Fragment, or a test's. */
export interface FollowedChunk {
  readonly url: string;
  readonly level: number;
  readonly duration: number;
  readonly stats: Pick<LoaderStats, "loaded" | "total" | "aborted"> & {
    readonly loading: Pick<HlsProgressivePerformanceTiming, "start" | "first" | "end">;
  };
}

/** What choosing reads of hls.js. */
export interface ChoosingHls {
  readonly levels: readonly ChoiceLevel[];
  readonly currentLevel: number;
  readonly loadLevel: number;
  readonly minAutoLevel: number;
  readonly maxAutoLevel: number;
  readonly mainForwardBufferInfo: { readonly len: number } | null;
  /** The video it plays into, whose position shows whether it plays. */
  readonly media: Pick<
    HTMLMediaElement,
    "buffered" | "currentTime" | "paused" | "readyState"
  > | null;
}

/** What a playback tells its choosing. */
export interface ChoosingOptions {
  /** The highest BANDWIDTH it starts within (D247 §6, D260 §3). */
  readonly startAtBps: number;
  readonly monotonicMs: () => number;
  /** Whether this tab's voice is live, when the film gives way to it (D262 §2). */
  readonly isVoiceLive: () => boolean;
  /** Whether it plays in a Watch Together group, which keeps the film's own picture (D286 §2). */
  readonly isInAGroup?: () => boolean;
  /** Whether the browser took the chunk at url from its cache; undefined where it does not say. */
  readonly fromCache: (url: string) => boolean | undefined;
  /**
   * Told each change of the ceiling it chooses under, and why, for the diagnosis's steps: a player
   * that dropped from the twin to 360p after a jump on a 77 Mb/s link said nothing of why (the owner's
   * Firefox on nas.example, 2026-10-20).
   */
  readonly noteStep?: (step: string) => void;
  /** Told of a quality given up as one this device cannot play, and where (D266). */
  readonly onUnplayable?: (unplayable: {
    readonly qualityKbps: number;
    readonly positionMs: number;
    readonly isFilmsOwnPicture: boolean;
  }) => void;
}

/** What choosing is given of the link: hls.js's estimate of it, and how to give a chunk up. */
export interface ChoosingLink<Chunk extends FollowedChunk> {
  readonly estimateBps: () => number;
  readonly giveUp: (chunk: Chunk) => void;
  /**
   * Takes what a chunk given up showed of the link, bytes over durationMs, as a measure of it: the
   * chunk loads no more, and nothing else would measure a link that slowed while the old measure kept
   * a group's player on the film's own picture, given up again every few seconds (the owner's group
   * test, 2026-10-14). hls.js's own abandon resets its measures instead, which ours does not (D265).
   */
  readonly measureGivenUp: (durationMs: number, bytes: number) => void;
  /**
   * Builds the video's decoding anew and loads again from positionS, where the video stood, at the
   * quality chosen next.
   */
  readonly loadAgain: (positionS: number) => void;
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

/**
 * Where a video, asked to play, has stood still: since when with what it plays there loaded
 * (undefined while not), and how many chunks came meanwhile.
 */
interface StillAt {
  readonly positionS: number;
  loadedSinceMs: number | undefined;
  chunksLoaded: number;
  chunksNotHeld: number;
}

/**
 * Every choice of a chunked playback's quality (D265), by abr-choice.ts's rules: where it starts,
 * when a chunk on its way is given up, what the next chunk is of, the step-up, what voice leaves the
 * film, and which chunks measure the link. hls.js asks it through OrpheumAbr.
 */
export class Choosing<Chunk extends FollowedChunk> {
  readonly #hls: ChoosingHls;
  readonly #options: ChoosingOptions;
  readonly #link: ChoosingLink<Chunk>;
  /** The highest quality it may choose: the start, lowered by each step down, raised by the step-up. */
  #ceiling: number | undefined;
  #changedAtMs: number;
  #onItsWay: { readonly chunk: Chunk; readonly sinceMs: number } | undefined;
  /** The pace each paced chunk was asked to arrive at, bits a second. */
  readonly #paces = new WeakMap<Chunk, number>();
  /** A quality hls.js forced, after a load error, until a chunk of it loads (D265 §8). */
  #forced: number | undefined;
  #lastChosen = -1;
  #hasMeasured = false;
  /** Where the video, asked to play, has stood still, and what came meanwhile. */
  #still: StillAt | undefined;
  /** Until when, after its group jumped, it keeps the common quality (D286 §3); 0 for not. */
  #commonUntilMs = 0;
  /** The lowest quality found unplayable here: it and every one above it are never chosen again (D266). */
  #unplayableFrom: number | undefined;
  /** The highest rate the server lets it load, in kbit/s, with each chunk's answer (D286 §5); undefined for none. */
  #serverCeilingKbps: number | undefined;
  readonly #timers: ReturnType<typeof setInterval>[];

  constructor(hls: ChoosingHls, options: ChoosingOptions, link: ChoosingLink<Chunk>) {
    this.#hls = hls;
    this.#options = options;
    this.#link = link;
    this.#changedAtMs = options.monotonicMs();
    this.#timers = [
      setInterval(() => {
        this.#lookAtChunk();
        this.#lookAtPlaying();
      }, lookAtChunkEveryMs),
      setInterval(() => {
        this.#lookForStepUp();
      }, lookForStepUpEveryMs),
    ];
  }

  /** The quality the first chunk is of. */
  firstLevel(): number {
    this.#lastChosen = this.#clamped(this.#ceilingNow());
    return this.#lastChosen;
  }

  /** The quality the next chunk is of: a forced one, else the steady choice, under voice's ceiling. */
  nextLevel(): number {
    if (this.#forced !== undefined) {
      this.#lastChosen = this.#forced;
      return this.#forced;
    }
    const linkBps = this.#linkBps();
    const inTheGroup = this.#groupsChoice(linkBps);
    if (inTheGroup !== undefined) {
      this.#lastChosen = this.#clamped(inTheGroup);
      return this.#lastChosen;
    }
    const ceiling = this.#ceilingNow();
    const steady = steadyLevel({
      levels: this.#hls.levels,
      ceiling,
      linkBps,
      bufferAheadS: this.#bufferAheadS(),
    });
    this.#lower(steady, () => {
      const link =
        linkBps === undefined ? "unmeasured" : `${String(Math.round(linkBps / 1000))} kb/s`;
      return `link ${link}, ${this.#bufferAheadS().toFixed(1)} s ahead`;
    });
    const level = this.#options.isVoiceLive()
      ? Math.min(steady, voiceCeiling(this.#hls.levels, linkBps))
      : steady;
    this.#lastChosen = this.#clamped(level);
    return this.#lastChosen;
  }

  /**
   * Its group jumped: unless it keeps the film's own picture, it loads the common quality for
   * commonStartForMs, then settles higher or lower as it would (D286 §3).
   */
  groupJumped(): void {
    this.#commonUntilMs = this.#options.monotonicMs() + commonStartForMs;
  }

  /**
   * The quality its group has it play, if any: the film's own picture while its link carries it
   * (D286 §2), or, for a while after a jump, the common quality (§3); never one found unplayable.
   */
  #groupsChoice(linkBps: number | undefined): number | undefined {
    const unplayableFrom = this.#unplayableFrom ?? Number.POSITIVE_INFINITY;
    const kept =
      this.#options.isInAGroup?.() === true ? keptOwnPicture(this.#hls.levels, linkBps) : undefined;
    if (kept !== undefined && kept < unplayableFrom) {
      return kept;
    }
    const common = commonStartLevel(this.#hls.levels);
    const isCommon = this.#options.monotonicMs() < this.#commonUntilMs && common >= 0;
    return isCommon && common < unplayableFrom ? common : undefined;
  }

  /** The quality hls.js forced, or -1. */
  forcedLevel(): number {
    return this.#forced ?? -1;
  }

  /**
   * hls.js setting the next quality: its echo of what was just chosen, which changes nothing, or a
   * force, a switch down after a load error, obeyed until a chunk of it loads (D265 §8).
   */
  setLevel(level: number): void {
    const clamped = this.#clamped(level);
    if (
      level < 0 ||
      clamped === this.#lastChosen ||
      clamped >= (this.#unplayableFrom ?? Number.POSITIVE_INFINITY)
    ) {
      return;
    }
    this.#forced = clamped;
    this.#lower(clamped, () => "forced by hls.js");
  }

  /** A chunk of picture began loading, at the pace asked while voice is live. */
  loading(chunk: Chunk): void {
    this.#onItsWay = { chunk, sinceMs: this.#options.monotonicMs() };
    const paceKbps = this.paceKbps();
    if (paceKbps !== undefined) {
      this.#paces.set(chunk, paceKbps * 1000);
    }
  }

  /**
   * The core answered a chunk of picture on its way with 425, being made: it is judged from when it
   * is made, not from when it was first asked for. Judged from then, a film's first play after the
   * core's start gave its own picture up for the lowest quality within two seconds, nothing having
   * arrived yet, and measured the link at a byte in two seconds, so it stayed there (A Man Called
   * Otto on nas.example, the owner's Firefox, 2026-10-20). A lower quality is made from the film too, and
   * slower than the copy of its own picture, so giving one up for it never brings a chunk sooner.
   */
  beingMade(chunk: Chunk): void {
    if (this.#onItsWay?.chunk === chunk) {
      this.#onItsWay = { chunk, sinceMs: this.#options.monotonicMs() };
    }
  }

  /** A chunk of picture loaded. */
  loaded(chunk: Chunk): void {
    if (this.#still !== undefined) {
      this.#still.chunksLoaded += 1;
    }
    if (this.#onItsWay?.chunk === chunk) {
      this.#onItsWay = undefined;
    }
    if (this.#forced === chunk.level) {
      this.#forced = undefined;
    }
  }

  /**
   * The browser took a chunk of picture of level and held nothing of it (hls.js's
   * bufferAppendNoProgress): counted while the video stands still at that quality (D266).
   */
  notHeld(level: number): void {
    if (this.#still !== undefined && level === this.#hls.loadLevel) {
      this.#still.chunksNotHeld += 1;
    }
  }

  /** Whether a loaded chunk of picture measures the link, to be counted in the estimate (D265 §7). */
  measures(chunk: Chunk): boolean {
    const { loading, loaded } = chunk.stats;
    const isCounted = measuresTheLink({
      loadMs: loading.end - loading.start,
      arrivingMs: loading.end - loading.first,
      bytes: loaded,
      paceBps: this.#paces.get(chunk),
      isFromCache: this.#options.fromCache(chunk.url),
    });
    this.#hasMeasured ||= isCounted;
    return isCounted;
  }

  /** The quality playing changed: the step-up holds from now. */
  switched(): void {
    this.#changedAtMs = this.#options.monotonicMs();
  }

  /**
   * The pace a download asks while this tab's voice is live, in kbit/s: paceAboveQuality times the
   * quality it loads, so the film never fills the link's queue in bursts ahead of the voice (D262
   * §2); undefined while voice is not live, or before a quality is loaded.
   */
  paceKbps(): number | undefined {
    const bitrate = this.#hls.levels[this.#hls.loadLevel]?.bitrate ?? 0;
    return this.#options.isVoiceLive() && bitrate > 0
      ? Math.ceil((bitrate * paceAboveQuality) / 1000)
      : undefined;
  }

  /** The link as measured, in kbit/s; undefined before a chunk measured it. */
  linkKbps(): number | undefined {
    const linkBps = this.#linkBps();
    return linkBps === undefined ? undefined : linkBps / 1000;
  }

  stop(): void {
    for (const timer of this.#timers) {
      clearInterval(timer);
    }
  }

  #lookAtChunk(): void {
    const onItsWay = this.#onItsWay;
    if (onItsWay === undefined || onItsWay.chunk.stats.aborted) {
      return;
    }
    const { chunk, sinceMs } = onItsWay;
    const { loading, loaded, total } = chunk.stats;
    const chosen = giveUpFor({
      levels: this.#hls.levels,
      loadingLevel: chunk.level,
      lowestLevel: this.#hls.minAutoLevel,
      durationS: chunk.duration,
      loadingMs: this.#options.monotonicMs() - sinceMs,
      firstByteMs: loading.first > 0 ? loading.first - loading.start : undefined,
      loadedBytes: loaded,
      totalBytes: total,
      linkBps: this.#linkBps(),
      bufferAheadS: this.#bufferAheadS(),
    });
    if (chosen === undefined) {
      return;
    }
    this.#onItsWay = undefined;
    this.#link.measureGivenUp(
      Math.max(1, this.#options.monotonicMs() - sinceMs),
      Math.max(1, loaded),
    );
    this.#hasMeasured = true;
    this.#lower(chosen, () => {
      const loadingMs = this.#options.monotonicMs() - sinceMs;
      return `gave up a chunk of ${String(chunk.level)}: ${String(loaded)} B in ${String(loadingMs)} ms`;
    });
    this.#lastChosen = chosen;
    this.#link.giveUp(chunk);
  }

  /**
   * Gives up the quality loading as unplayable here when the video, waiting to play, stands still
   * while its chunks arrive (D266): every quality from it up is dropped, what is loaded with it, and
   * the video loads again from where it is at the quality chosen next. Standing still counts its
   * time only while what it plays is loaded where it stands, a seek's sound coming late being no
   * sign; the browser holding nothing of the chunks it took counts whatever is loaded, since a
   * picture it cannot decode is never loaded anywhere (Flow's on Firefox on macOS, the owner's
   * group test, 2026-10-12).
   */
  #lookAtPlaying(): void {
    const nowMs = this.#options.monotonicMs();
    const still = this.#standingStill(nowMs);
    const level = this.#hls.loadLevel;
    if (
      still !== undefined &&
      level > this.#hls.minAutoLevel &&
      isUnplayable({
        forMs: still.loadedSinceMs === undefined ? 0 : nowMs - still.loadedSinceMs,
        chunksLoaded: still.chunksLoaded,
        chunksNotHeld: still.chunksNotHeld,
        loadedAheadS: this.#bufferAheadS(),
      })
    ) {
      this.#giveUpUnplayable(level, still.positionS);
    }
  }

  /**
   * Where the video, waiting to play, has stood still since the last look, kept up to date; undefined
   * when it is not waiting, or has just moved and starts standing still afresh.
   */
  #standingStill(nowMs: number): StillAt | undefined {
    const media = this.#hls.media;
    if (media === null || !isWaitingToPlay(media)) {
      this.#still = undefined;
      return undefined;
    }
    const isLoaded = isLoadedWhereItStands(media);
    const still = this.#still;
    if (still === undefined || Math.abs(media.currentTime - still.positionS) > movedAtLeastS) {
      this.#still = {
        positionS: media.currentTime,
        loadedSinceMs: isLoaded ? nowMs : undefined,
        chunksLoaded: 0,
        chunksNotHeld: 0,
      };
      return undefined;
    }
    still.loadedSinceMs = isLoaded ? (still.loadedSinceMs ?? nowMs) : undefined;
    return still;
  }

  /**
   * Whether the quality loading, a hole in which hls.js could not step over, can be given up as
   * unplayable here, a lower one there to load (D266): iPhone Safari kept 0.26 s of a well-formed
   * first chunk of Copper Kettle's own picture, 242 frames over 10 s, and hls.js, stepping over the hole
   * and finding the playhead still, gave the playback up (the owner's group test, 2026-10-13).
   */
  canGiveUpAtAHole(): boolean {
    return this.#hls.loadLevel > this.#hls.minAutoLevel;
  }

  /** Gives the quality loading up as unplayable here, at a hole it could not pass at positionS. */
  giveUpAtAHole(positionS: number): void {
    const level = this.#hls.loadLevel;
    if (level > this.#hls.minAutoLevel) {
      this.#giveUpUnplayable(level, positionS);
    }
  }

  /** Gives level up, with every quality above it, and loads again from positionS below it (D266). */
  #giveUpUnplayable(level: number, positionS: number): void {
    this.#unplayableFrom = Math.min(level, this.#unplayableFrom ?? level);
    if (this.#forced !== undefined && this.#forced >= this.#unplayableFrom) {
      this.#forced = undefined; // obeyed, it would load the quality given up
    }
    this.#still = undefined;
    this.#onItsWay = undefined;
    this.#lower(level - 1, () => "unplayable here");
    this.#lastChosen = -1;
    this.#options.onUnplayable?.({
      qualityKbps: Math.round((this.#hls.levels[level]?.bitrate ?? 0) / 1000),
      positionMs: Math.round(positionS * 1000),
      isFilmsOwnPicture: level === ownPictureLevel(this.#hls.levels),
    });
    this.#link.loadAgain(positionS);
  }

  #lookForStepUp(): void {
    const ceiling = this.#ceiling;
    if (ceiling === undefined) {
      return;
    }
    const raised = nextCap({
      levels: this.#hls.levels,
      cap: ceiling,
      playing: this.#hls.currentLevel,
      estimateBps: this.#linkBps() ?? 0,
      bufferedAheadMs: this.#bufferAheadS() * 1000,
      sinceChangeMs: this.#options.monotonicMs() - this.#changedAtMs,
    });
    if (raised !== ceiling && raised < (this.#unplayableFrom ?? Number.POSITIVE_INFINITY)) {
      this.#options.noteStep?.(`ceiling ${String(ceiling)} to ${String(raised)}: stepped up`);
      this.#ceiling = raised;
      this.#changedAtMs = this.#options.monotonicMs();
    }
  }

  /** The ceiling, the start until the levels are known and it is set from them. */
  #ceilingNow(): number {
    if (this.#ceiling === undefined && this.#hls.levels.length > 0) {
      this.#ceiling = startingLevel(this.#hls.levels, this.#options.startAtBps);
    }
    return this.#ceiling ?? 0;
  }

  /** Lowers the ceiling to level, a step down; one at or above it changes nothing. */
  #lower(level: number, why: () => string): void {
    if (level < this.#ceilingNow()) {
      this.#options.noteStep?.(
        `ceiling ${String(this.#ceilingNow())} to ${String(level)}: ${why()}`,
      );
      this.#ceiling = level;
      this.#changedAtMs = this.#options.monotonicMs();
    }
  }

  #clamped(level: number): number {
    const { minAutoLevel, maxAutoLevel, levels } = this.#hls;
    if (levels.length === 0) {
      return level;
    }
    const highest = Math.min(maxAutoLevel, underCeiling(levels, this.#serverCeilingKbps));
    return Math.max(Math.min(level, highest), minAutoLevel);
  }

  /**
   * The server's ceiling, kbps, with a chunk's answer: the highest rate it lets this player load,
   * its quality's average with its sound's, so its upload carries every player (D286 §5); undefined
   * lifts it. The next chunk is of a quality under it, and the step-up stays under it.
   */
  obeyServer(kbps: number | undefined): void {
    this.#serverCeilingKbps = kbps;
  }

  #linkBps(): number | undefined {
    return this.#hasMeasured ? this.#link.estimateBps() : undefined;
  }

  #bufferAheadS(): number {
    return this.#hls.mainForwardBufferInfo?.len ?? 0;
  }
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
 * hls.js's own bitrate controller, keeping its estimator, which its other controllers read, with
 * every choice handed to a Choosing (D265). A chunk given up is aborted as hls.js's own controller
 * does, and hls.js loads again from where it was.
 */
class OrpheumAbrController extends AbrController {
  readonly choosing: Choosing<Fragment>;

  constructor(hls: Hls, options: ChoosingOptions) {
    super(hls);
    this.choosing = new Choosing<Fragment>(hls, options, {
      estimateBps: () => this.bwEstimator.getEstimate(),
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

  protected override onFragBuffered(event: Events.FRAG_BUFFERED, data: FragBufferedData): void {
    if (isPicture(data.frag) && this.choosing.measures(data.frag)) {
      super.onFragBuffered(event, data); // its sample of the link
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
