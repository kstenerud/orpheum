// SPDX-License-Identifier: MPL-2.0

import {
  arrivalMs,
  measuresTheLink,
  paceAboveQuality,
  reportsLinkFromSamples,
} from "./abr-choice.ts";
import type {
  ChoiceWhy,
  ChoosingHls,
  ChoosingLink,
  ChoosingOptions,
  FollowedChunk,
} from "./abr-options.ts";
import { LinkEstimate, priorNoteText } from "./link-estimate.ts";
import { ChunkOnItsWay, type AskingAgain } from "./abr-on-its-way.ts";
import type { NextChunkLink } from "./next-chunk.ts";
import { PlayedTime, StandingStill } from "./abr-video.ts";
import { Steadiness } from "./steadiness.ts";
import { StepUpShown } from "./step-up.ts";
import { bufferAheadS, QualityCeiling } from "./abr-ceiling.ts";

/** How often the controller looks at a chunk on its way (D265 §3). */
export const lookAtChunkEveryMs = 500;

/** How often it looks at whether it may step up a quality (D247 §9). */
export const lookForStepUpEveryMs = 10_000;

export {
  paceAboveQuality,
  reportsLinkFromSamples,
  startingPlayedMs,
  startingSamples,
} from "./abr-choice.ts";

export { holdsQualityAheadS } from "./abr-ceiling.ts";

export type { ChoosingHls, ChoosingLink, ChoosingOptions, FollowedChunk } from "./abr-options.ts";

/**
 * Every choice of a chunked playback's quality (D265), by abr-choice.ts's rules: where it starts,
 * when a chunk on its way is given up, what the next chunk is of, the step-up, what voice leaves the
 * film, and which chunks measure the link; each by its own estimate of the link, trusted as far as
 * the evidence goes (D352): its first seconds latch nothing, it climbs as far as consistent samples
 * carry, and it grows steadier when it swings or its link flakes. hls.js asks it through OrpheumAbr
 * (abr-controller.ts).
 */
export class Choosing<Chunk extends FollowedChunk> {
  readonly #hls: ChoosingHls;
  readonly #options: ChoosingOptions;
  /** The chunk of picture on its way, followed to judge whether to give it up. */
  readonly #onItsWay = new ChunkOnItsWay<Chunk>();
  readonly isAwaitingTheCore = (): boolean => this.#onItsWay.isAwaitingTheCore();
  /** Asks again after a load hls.js stopped, counted with the chunks stuck (loadOnAfterFailures). */
  readonly askingAgain = (): AskingAgain => {
    this.#still.linkFailed("its loading stopped");
    return this.#onItsWay.askingAgain();
  };
  /** The pace each paced chunk was asked to arrive at, bits a second. */
  readonly #paces = new WeakMap<Chunk, number>();
  /**
   * The link as its last chunks measured it, which every choice reads (D352 §2): hls.js's estimate,
   * weighted by each chunk's seconds, held one slow first sample of 1008 kb/s for minutes on a
   * 12 Mb/s path (the owner, on staging, 2026-10-26).
   */
  readonly #estimate: LinkEstimate;
  /** How steady the link has been, which sets the step-up's hold and headroom (D352 §3–§4). */
  readonly #steadiness: Steadiness;
  /** How long the video has played. */
  readonly #played = new PlayedTime();
  /** Where the video, asked to play, has stood still, and what came meanwhile. */
  readonly #still = new StandingStill();
  /** The ceiling and choice state this playback's quality is decided from (abr-ceiling.ts). */
  readonly #ceiling: QualityCeiling<Chunk>;
  readonly #timers: ReturnType<typeof setInterval>[];

  constructor(hls: ChoosingHls, options: ChoosingOptions, link: ChoosingLink<Chunk>) {
    this.#hls = hls;
    this.#options = options;
    this.#estimate = new LinkEstimate(options.rememberedLink);
    const prior = this.#estimate.prior();
    if (prior !== undefined) {
      options.noteLink?.(priorNoteText(prior));
    }
    this.#steadiness = new Steadiness(options.monotonicMs(), options.startSteadiness);
    const stepUpShown = new StepUpShown((note) => options.noteLink?.(note));
    options.noteSteadiness?.(this.#steadiness.level());
    this.#ceiling = new QualityCeiling({
      hls,
      options,
      link,
      estimate: this.#estimate,
      steadiness: this.#steadiness,
      stepUpShown,
      onItsWay: this.#onItsWay,
      still: this.#still,
    });
    this.#timers = [
      setInterval(() => {
        this.#ceiling.lookAtChunk();
        this.#ceiling.lookAtPlaying();
        this.#played.look(this.#hls.media, this.#options.monotonicMs());
        this.#ceiling.endStartingWhenDone(this.#played.playedMs());
      }, lookAtChunkEveryMs),
      setInterval(() => {
        this.#ceiling.lookForStepUp();
      }, lookForStepUpEveryMs),
    ];
  }

  /** The quality the first chunk is of. */
  firstLevel(): number {
    return this.#ceiling.firstLevel();
  }

  /**
   * Why level was last chosen, and the link's samples then, for the switch to it (ChoiceWhy);
   * undefined for a level never chosen.
   */
  whyChose(level: number): ChoiceWhy | undefined {
    return this.#ceiling.whyChose(level);
  }

  /** The quality the next chunk is of: a forced one, else the steady choice, under voice's ceiling. */
  nextLevel(): number {
    return this.#ceiling.nextLevel();
  }

  /**
   * Its group jumped: unless it keeps the film's own picture, it loads the common quality for
   * commonStartForMs, then settles higher or lower as it would (D286 §3).
   */
  groupJumped(): void {
    this.#ceiling.groupJumped();
  }

  /** The quality hls.js forced, or -1. */
  forcedLevel(): number {
    return this.#ceiling.forcedLevel();
  }

  /**
   * hls.js setting the next quality: its echo of what was just chosen, which changes nothing, or a
   * force, a switch down after a load error, obeyed until a chunk of it loads (D265 §8).
   */
  setLevel(level: number): void {
    this.#ceiling.setLevel(level);
  }

  /** A chunk of picture began loading, at the pace asked while voice is live. */
  loading(chunk: Chunk): void {
    this.#onItsWay.loading(chunk, this.#options.monotonicMs());
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
    // Its 425's own body is counted loaded, its first byte with it: 81 bytes in two seconds gave
    // the twin up for 360p and measured the link at 667 kb/s (the owner's Firefox, 2026-10-20).
    this.#onItsWay.beingMade(chunk, this.#options.monotonicMs());
  }

  /** A chunk of picture's load failed or timed out, the core's 425 aside (ChunkOnItsWay.failed). */
  failed(chunk: Chunk, details: string): void {
    this.#still.linkFailed(`a chunk failed (${details})`);
    this.#options.noteLink?.(
      this.#onItsWay.failed(
        chunk,
        details,
        `carried ${this.#options.carrierOf?.(chunk.url) ?? "unknown"}`,
      ),
    );
  }

  /** A chunk of picture loaded. */
  loaded(chunk: Chunk): void {
    this.#still.chunkLoaded();
    this.#onItsWay.loaded(chunk);
    this.#ceiling.forcedLoaded(chunk.level);
  }

  /**
   * The browser took a chunk of picture of level and held nothing of it (hls.js's
   * bufferAppendNoProgress): counted while the video stands still at that quality (D266).
   */
  notHeld(level: number): void {
    if (level === this.#hls.loadLevel) {
      this.#still.chunkNotHeld();
    }
  }

  /** Whether a loaded chunk of picture measures the link, to be counted in the estimate (D265 §7). */
  measures(chunk: Chunk): boolean {
    const { loading, loaded } = chunk.stats;
    const isCounted = measuresTheLink({
      loadMs: loading.end - loading.start,
      arrivingMs: arrivalMs(loading),
      bytes: loaded,
      paceBps: this.#paces.get(chunk),
      isFromCache: this.#options.fromCache(chunk.url),
    });
    if (isCounted) {
      this.#ceiling.sample({
        level: chunk.level,
        bytes: loaded,
        firstByteMs: Math.max(0, loading.first - loading.start),
        arrivedMs: arrivalMs(loading),
      });
      this.#ceiling.endStartingWhenDone(this.#played.playedMs());
    }
    return isCounted;
  }

  /**
   * The video stalled (D247 §2, watchStalls): a drop, which may make the playback steadier (D352
   * §4), and, while starting, the one thing besides a chunk given up that lowers the ceiling, to
   * what the link's estimate now chooses (§1).
   */
  stalled(): void {
    this.#ceiling.stalled();
  }

  /** The quality playing changed: the step-up holds from now, and a step up shown is noted (StepUpShown). */
  switched(): void {
    this.#ceiling.switched();
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

  /**
   * The link as reported, and remembered for the device's next start (D257 §1): its estimate, the
   * centre the next start's prior is (bandwidth-sampling P2), never a bound of it, which would
   * remember each playback lower than the last; once reportsLinkFromSamples chunks measured it.
   * estimate, in kbit/s (D352 §1); undefined before reportsLinkFromSamples chunks measured it.
   */
  linkKbps(): number | undefined {
    const estimateBps = this.#estimate.estimateBps();
    return estimateBps === undefined || this.#estimate.count() < reportsLinkFromSamples
      ? undefined
      : estimateBps / 1000;
  }

  /**
   * The link its next chunk's load is reckoned over, for the start gate (next-chunk.ts): the
   * estimate's next-chunk bound at what is loaded ahead, from the remembered link alone before any
   * sample, and the wait its first byte is expected to take; undefined with neither (P5).
   */
  nextChunkLink(): NextChunkLink | undefined {
    const bps = this.#estimate.nextChunkBps(bufferAheadS(this.#hls));
    return bps === undefined
      ? undefined
      : {
          kbps: bps / 1000,
          firstByteMs: this.#estimate.firstByteMs(),
          from: this.#estimate.kept() > 0 ? "confident" : "remembered",
        };
  }

  /** The playback's steadiness level (D352 §3). */
  steadiness(): number {
    return this.#steadiness.level();
  }

  /** Whether the playback is still starting (D352 §1). */
  isStarting(): boolean {
    return this.#ceiling.isStarting();
  }

  /** The highest quality it may ever load: the top, below any unplayable here (D266); no ceiling of the moment. */
  topLevel(): number {
    return this.#ceiling.topLevel();
  }

  stop(): void {
    for (const timer of this.#timers) {
      clearInterval(timer);
    }
  }

  /**
   * Whether the quality loading, a hole in which hls.js could not step over, can be given up as
   * unplayable here, a lower one there to load (D266): iPhone Safari kept 0.26 s of a well-formed
   * first chunk of Copper Kettle's own picture, 242 frames over 10 s, and hls.js, stepping over the hole
   * and finding the playhead still, gave the playback up (the owner's group test, 2026-10-13).
   */
  canGiveUpAtAHole(): boolean {
    return this.#ceiling.canGiveUpAtAHole();
  }

  /** Gives the quality loading up as unplayable here, at a hole it could not pass at positionS. */
  giveUpAtAHole(positionS: number): void {
    this.#ceiling.giveUpAtAHole(positionS);
  }

  /**
   * The server's ceiling, kbps, with a chunk's answer: the highest rate it lets this player load,
   * its quality's average with its sound's, so its upload carries every player (D286 §5); undefined
   * lifts it. The next chunk is of a quality under it, and the step-up stays under it.
   */
  obeyServer(kbps: number | undefined): void {
    this.#ceiling.obeyServer(kbps);
  }
}
