// SPDX-License-Identifier: MPL-2.0

import {
  commonStartForMs,
  commonStartLevel,
  giveUpFor,
  givenUpSample,
  givenUpText,
  highestWithin,
  keptOwnPicture,
  linkShare,
  ownPictureLevel,
  reportsLinkFromSamples,
  startingLevel,
  startingPlayedMs,
  startingSamples,
  steadyLevel,
  underCeiling,
  voiceCeiling,
} from "./abr-choice.ts";
import type {
  ChoiceWhy,
  ChoosingHls,
  ChoosingLink,
  ChoosingOptions,
  FollowedChunk,
} from "./abr-options.ts";
import {
  estimateText,
  lastSampleNote,
  samplesText,
  type LinkEstimate,
  type LinkSample,
} from "./link-estimate.ts";
import type { ChunkOnItsWay, JudgedChunk } from "./abr-on-its-way.ts";
import { resumePositionOf } from "./resume-position.ts";
import type { StandingStill } from "./abr-video.ts";
import type { Steadiness } from "./steadiness.ts";
import { nextCap, stepUpBufferMs, type StepUpShown } from "./step-up.ts";
import { busyText, type BusyPage } from "./abr-busy.ts";

/**
 * How much loaded ahead holds the quality loading, in seconds: while at least this much is loaded,
 * no reading of the link lowers it, neither the next chunk's choice nor the ceiling, at starting's
 * end or after; the link decides again once less is loaded, on the evidence then. A step down exists
 * so the player does not run dry, and with this much loaded it is not about to: a Fire TV with
 * 37.3 s loaded fell from its top quality to its second lowest on one slow sample, and the change
 * of sound with it threw its loaded sound away and stalled it (staging, Nosferatu, 2026-10-28). The
 * step-up's own (stepUpBufferMs): what is enough to climb on is enough to stay on, and a climb that
 * does not hold is followed down once the buffer it spent falls under it.
 */
export const holdsQualityAheadS = stepUpBufferMs / 1000;

/** What is loaded ahead of hls's playhead, in seconds. */
export function bufferAheadS(hls: ChoosingHls): number {
  return hls.mainForwardBufferInfo?.len ?? 0;
}

/**
 * What a QualityCeiling needs: the playback and link it chooses for, and the collaborators it
 * shares with the rest of Choosing (abr.ts) - the link's estimate and steadiness, its step-up
 * note, and the chunk and stall trackers a give-up or an unplayable quality is judged from.
 */
export interface QualityCeilingInit<Chunk extends FollowedChunk> {
  readonly hls: ChoosingHls;
  readonly options: ChoosingOptions;
  readonly link: ChoosingLink<Chunk>;
  readonly estimate: LinkEstimate;
  readonly steadiness: Steadiness;
  readonly stepUpShown: StepUpShown;
  readonly onItsWay: ChunkOnItsWay<Chunk>;
  readonly still: StandingStill;
  readonly busy: BusyPage;
}

/**
 * The ceiling and choice state of Choosing's playback (abr.ts): the highest quality it may choose
 * now, why it stands there, the last quality chosen and why (whyChose), a quality hls.js forced,
 * one found unplayable here, whether the playback is still starting, and the group's own picture
 * or common quality (D286). Choosing delegates firstLevel, nextLevel and the rest of its public
 * choice-making to this, keeping only the chunk and stall trackers (ChunkOnItsWay, StandingStill)
 * and the timers that drive lookAtChunk, lookAtPlaying, lookForStepUp and endStartingWhenDone.
 */
export class QualityCeiling<Chunk extends FollowedChunk> {
  readonly #hls: ChoosingHls;
  readonly #options: ChoosingOptions;
  readonly #link: ChoosingLink<Chunk>;
  readonly #estimate: LinkEstimate;
  readonly #steadiness: Steadiness;
  readonly #stepUpShown: StepUpShown;
  readonly #onItsWay: ChunkOnItsWay<Chunk>;
  readonly #still: StandingStill;
  readonly #busy: BusyPage;
  /** The chunk on its way last excused by a long task of the page, noted once (D382 §3). */
  #excused: Chunk | undefined;
  /** The highest quality it may choose: the start, lowered by each step down, raised by the step-up. */
  #ceiling: number | undefined;
  #changedAtMs: number;
  /** A quality hls.js forced, after a load error, until a chunk of it loads (D265 §8). */
  #forced: number | undefined;
  #lastChosen = -1;
  /** Why each quality was last chosen, by level, for the switch to it (whyChose). */
  readonly #whyChosen = new Map<number, ChoiceWhy>();
  /** Why the ceiling stands where it does: "first" until it changes, then its last change's reason. */
  #ceilingWhy = "first";
  /**
   * Whether the playback is starting: until it played startingPlayedMs and its samples settle the
   * ceiling or startingSamples measured the link, a choice below the ceiling does not lower it
   * (D352 §1, P5).
   */
  #isStarting = true;
  /** Whether the hold of the quality loading (holdsQualityAheadS) was noted since it began. */
  #isHoldNoted = false;
  /** Until when, after its group jumped, it keeps the common quality (D286 §3); 0 for not. */
  #commonUntilMs = 0;
  /** The lowest quality found unplayable here: it and every one above it are never chosen again (D266). */
  #unplayableFrom: number | undefined;
  /** The highest rate the server lets it load, in kbit/s, with each chunk's answer (D286 §5); undefined for none. */
  #serverCeilingKbps: number | undefined;

  constructor(init: QualityCeilingInit<Chunk>) {
    this.#hls = init.hls;
    this.#options = init.options;
    this.#link = init.link;
    this.#estimate = init.estimate;
    this.#steadiness = init.steadiness;
    this.#stepUpShown = init.stepUpShown;
    this.#onItsWay = init.onItsWay;
    this.#still = init.still;
    this.#busy = init.busy;
    this.#changedAtMs = init.options.monotonicMs();
  }

  /** The quality the first chunk is of. */
  firstLevel(): number {
    return this.#choose(this.#clamped(this.#ceilingNow()), () => this.#ceilingWhy);
  }

  /**
   * Why level was last chosen, and the link's samples then, for the switch to it (ChoiceWhy);
   * undefined for a level never chosen.
   */
  whyChose(level: number): ChoiceWhy | undefined {
    return this.#whyChosen.get(level);
  }

  /** The quality the next chunk is of: a forced one, else the steady choice, under voice's ceiling. */
  nextLevel(): number {
    if (this.#forced !== undefined) {
      return this.#choose(this.#forced, () => "forced by hls.js");
    }
    const linkBps = this.#linkBps();
    const inTheGroup = this.#groupsChoice(linkBps);
    if (inTheGroup !== undefined) {
      return this.#choose(this.#clamped(inTheGroup.level), () => inTheGroup.why);
    }
    const ceiling = this.#ceilingNow();
    const carried = steadyLevel({
      levels: this.#hls.levels,
      ceiling,
      linkBps,
      bufferAheadS: bufferAheadS(this.#hls),
      shareForQuality: this.#options.shareForQuality,
    });
    const held = this.#heldAbove(carried, ceiling);
    if (held !== undefined) {
      this.#noteHeldOnce(
        () => `ceiling ${String(ceiling)} held at ${String(held)}: link carries ${String(carried)}`,
      );
    } else if (!this.#isStarting && this.#lower(carried, () => `link; ${this.#stateText()}`)) {
      // While starting, a choice below the ceiling is the next chunk's alone (D352 §1).
      this.#noteSteadiness(this.#steadiness.steppedDown(this.#options.monotonicMs()));
    }
    const steady = held ?? carried;
    const level = this.#options.isVoiceLive()
      ? Math.min(steady, voiceCeiling(this.#hls.levels, linkBps))
      : steady;
    const clamped = this.#clamped(level);
    return this.#choose(clamped, () => {
      const ceilingNow = this.#ceilingNow();
      let why = this.#ceilingWhy;
      if (held !== undefined) {
        why = `held at ${String(held)}: link carries ${String(carried)}, ${this.#holdText()}`;
      } else if (carried < ceilingNow) {
        why = `link carries ${String(carried)} under ceiling ${String(ceilingNow)}; ${this.#stateText()}`;
      }
      if (level < steady) {
        why = `voice live, ${String(steady)} to ${String(level)}; ${why}`;
      }
      return clamped === level ? why : `${why}; clamped from ${String(level)}`;
    });
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
  #groupsChoice(
    linkBps: number | undefined,
  ): { readonly level: number; readonly why: string } | undefined {
    const unplayableFrom = this.#unplayableFrom ?? Number.POSITIVE_INFINITY;
    const kept =
      this.#options.isInAGroup?.() === true ? keptOwnPicture(this.#hls.levels, linkBps) : undefined;
    if (kept !== undefined && kept < unplayableFrom) {
      return { level: kept, why: "its group's: the film's own picture, its link carrying it" };
    }
    const common = commonStartLevel(this.#hls.levels);
    const isCommon = this.#options.monotonicMs() < this.#commonUntilMs && common >= 0;
    return isCommon && common < unplayableFrom
      ? { level: common, why: "its group's: the common quality after its group jumped" }
      : undefined;
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

  /** A chunk of picture loaded at level: a quality hls.js forced until then is no longer forced. */
  forcedLoaded(level: number): void {
    if (this.#forced === level) {
      this.#forced = undefined;
    }
  }

  /**
   * The video stalled (D247 §2, watchStalls): a drop, which may make the playback steadier (D352
   * §4), and, while starting, the one thing besides a chunk given up that lowers the ceiling, to
   * what the link's estimate now chooses (§1).
   */
  stalled(): void {
    this.#lower(this.#carriedNow(), () => `a stall; ${this.#stateText()}`);
    this.#noteSteadiness(this.#steadiness.dropped(this.#options.monotonicMs()));
  }

  /** The quality playing changed: the step-up holds from now, and a step up shown is noted (StepUpShown). */
  switched(): void {
    this.#changedAtMs = this.#options.monotonicMs();
    this.#stepUpShown.switched(this.#changedAtMs, this.#hls.currentLevel);
  }

  /** Whether the playback is still starting (D352 §1). */
  isStarting(): boolean {
    return this.#isStarting;
  }

  /** The highest quality it may ever load: the top, below any unplayable here (D266); no ceiling of the moment. */
  topLevel(): number {
    return Math.min(this.#hls.maxAutoLevel, (this.#unplayableFrom ?? Number.POSITIVE_INFINITY) - 1);
  }

  /**
   * The server's ceiling, kbps, with a chunk's answer: the highest rate it lets this player load,
   * its quality's average with its sound's, so its upload carries every player (D286 §5); undefined
   * lifts it. The next chunk is of a quality under it, and the step-up stays under it.
   */
  obeyServer(kbps: number | undefined): void {
    this.#serverCeilingKbps = kbps;
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
    if (this.canGiveUpAtAHole()) {
      this.#giveUpUnplayable(this.#hls.loadLevel, positionS);
    }
  }

  /** Looks at the chunk of picture on its way, judging whether to give it up (#onItsWay.look). */
  lookAtChunk(): void {
    const judged = this.#onItsWay.look(this.#options.monotonicMs(), this.#options.firstPieceOf);
    if (judged === undefined) {
      return;
    }
    const { chunk } = judged;
    if (this.#isToGiveUp(judged)) {
      this.#giveUp(judged);
      return;
    }
    // Not given up, as one of the lowest quality: stuck, noted with what carried it, and asked for
    // again, aborted for hls.js to load it again, no sample and lowering nothing (P3).
    const stuck = this.#onItsWay.stuck(judged, () => this.#carrierText(chunk));
    if (stuck !== undefined) {
      this.#still.linkFailed("a chunk stuck on its way");
      this.#options.noteLink?.(stuck.note);
    }
    if (stuck?.isAskedAgain === true) {
      this.#link.giveUp(chunk);
    }
  }

  /**
   * Whether the chunk on its way is to be given up: by its own rate (giveUpFor), unless the page was
   * too busy to read it (D382 §3); what for is the link's estimate's to say (#givenUpLevel, §1).
   */
  #isToGiveUp(judged: JudgedChunk<Chunk>): boolean {
    const { chunk } = judged;
    return (
      !this.#isExcusedBusy(judged) &&
      giveUpFor({
        shareForQuality: this.#options.shareForQuality,
        levels: this.#hls.levels,
        loadingLevel: chunk.level,
        lowestLevel: this.#hls.minAutoLevel,
        durationS: chunk.duration,
        loadingMs: judged.loadingMs,
        firstByteMs: judged.firstByteMs,
        loadedBytes: judged.firstByteMs === undefined ? 0 : judged.loadedBytes,
        totalBytes: chunk.stats.total,
        linkBps: this.#linkBps(),
        bufferAheadS: bufferAheadS(this.#hls),
      }) !== undefined
    );
  }

  /**
   * Whether the chunk on its way arrived, from its first byte to now, while a long task of the page
   * ran, which excuses it from being given up (D382 §3): stuck, it is still asked for again. Noted
   * once for each chunk, with the longest task's length.
   */
  #isExcusedBusy(judged: JudgedChunk<Chunk>): boolean {
    const { chunk } = judged;
    const longestMs =
      judged.firstByteMs === undefined
        ? undefined
        : this.#busy.longestWithin(chunk.stats.loading.first, this.#options.monotonicMs());
    if (longestMs === undefined) {
      return false;
    }
    if (this.#excused !== chunk) {
      this.#excused = chunk;
      const came = `${String(judged.loadedBytes)} B in ${String(judged.loadingMs)} ms`;
      this.#options.noteLink?.(
        busyText({ level: chunk.level, how: `on its way (${came})` }, longestMs),
      );
    }
    return true;
  }

  /** What carries chunk's load, for a note of it given up or stuck (the owner, 2026-10-29). */
  #carrierText(chunk: Chunk): string {
    return `carried ${this.#options.carrierOf?.(chunk.url) ?? "unknown"}`;
  }

  /**
   * Gives up a chunk on its way (D265 §3), a drop (D352 §4), for the highest quality below it the
   * link's estimate carries, its own sample counted in it (#givenUpLevel, D382 §1). One that was
   * arriving is a sample of the link, hls.js's estimator fed it too, and lowers the ceiling to that
   * quality; one stuck is no sample, and lowers the ceiling while starting only as far as the
   * estimate then carries, as a stall does (bandwidth-sampling-2026-10-29.md P3): a stuck transport
   * is not a slow link, and a lower quality's chunk travels it no better.
   */
  #giveUp(judged: JudgedChunk<Chunk>): void {
    const { chunk } = judged;
    const carried = this.#carrierText(chunk); // before it is aborted
    this.#onItsWay.ended(chunk);
    this.#still.linkFailed("a chunk given up");
    const nowMs = this.#options.monotonicMs();
    const sample = givenUpSample({ ...judged, level: chunk.level });
    if (sample !== undefined) {
      this.#link.measureGivenUp(sample.firstByteMs + sample.arrivedMs, sample.bytes);
      this.sample(sample);
    }
    const chosen = this.#givenUpLevel(chunk.level);
    const why = `gave up a chunk of ${String(chunk.level)}, ${givenUpText({
      ...judged,
      isSample: sample !== undefined,
    })}; ${this.#stateText()}`;
    const isStuckStarting = sample === undefined && this.#isStarting;
    if (!this.#lower(isStuckStarting ? this.#carriedNow() : chosen, () => why)) {
      this.#options.noteStep?.(why); // a give-up is always told, the ceiling lowered or not
    }
    this.#options.noteLink?.(
      `gave up a chunk of ${String(chunk.level)}, ${givenUpText({ ...judged, isSample: sample !== undefined })}; ${carried}`,
    );
    this.#noteSteadiness(this.#steadiness.dropped(nowMs));
    this.#choose(chosen, () => why);
    this.#link.giveUp(chunk);
  }

  /**
   * The quality a chunk of level given up is given up for (D382 §1): the highest below it the link's
   * estimate carries now (#carriedNow), never the chunk's own rate, by which one chunk that brought a
   * single 128 KiB piece took a Fire TV on a 20 Mb/s link to its lowest quality (staging,
   * 2026-10-30); the lowest the player may play at the least.
   */
  #givenUpLevel(level: number): number {
    return Math.max(Math.min(this.#carriedNow(), level - 1), this.#hls.minAutoLevel);
  }

  /** The quality the link's estimate carries now, under the ceiling, by D265 §4's share; the ceiling with no estimate. */
  #carriedNow(): number {
    return steadyLevel({
      levels: this.#hls.levels,
      ceiling: this.#ceilingNow(),
      linkBps: this.#linkBps(),
      bufferAheadS: bufferAheadS(this.#hls),
      shareForQuality: this.#options.shareForQuality,
    });
  }

  /**
   * Gives up the quality loading as unplayable here when the video, waiting to play, stands still
   * while its chunks arrive (D266; StandingStill.judge, which counts no standing still the link's):
   * every quality from it up is dropped, what is loaded with it, and the video loads again from where
   * it resumes at the quality chosen next. Notes why it judged a quality unplayable, or why not.
   */
  lookAtPlaying(): void {
    const nowMs = this.#options.monotonicMs();
    if (this.#options.isOffline?.() === true) {
      this.#still.linkFailed("the browser offline");
    }
    const level = this.#hls.loadLevel;
    if (this.#still.look(this.#hls.media, nowMs) === undefined || level <= this.#hls.minAutoLevel) {
      return;
    }
    const judged = this.#still.judge(nowMs, bufferAheadS(this.#hls));
    if (judged.note !== undefined) {
      this.#options.noteLink?.(`quality ${String(level)} ${judged.note}`);
    }
    if (judged.isUnplayable) {
      this.#giveUpUnplayable(level, resumePositionOf(this.#hls));
    }
  }

  /** Gives level up, with every quality above it, and loads again from positionS below it (D266). */
  #giveUpUnplayable(level: number, positionS: number): void {
    this.#unplayableFrom = Math.min(level, this.#unplayableFrom ?? level);
    if (this.#forced !== undefined && this.#forced >= this.#unplayableFrom) {
      this.#forced = undefined; // obeyed, it would load the quality given up
    }
    this.#still.forget();
    this.#onItsWay.forget();
    this.#lower(level - 1, () => "unplayable here");
    this.#lastChosen = -1;
    this.#options.onUnplayable?.({
      qualityKbps: Math.round((this.#hls.levels[level]?.bitrate ?? 0) / 1000),
      positionMs: Math.round(positionS * 1000),
      isFilmsOwnPicture: level === ownPictureLevel(this.#hls.levels),
    });
    this.#link.loadAgain(positionS);
  }

  /**
   * Relaxes the steadiness, then, once starting is over, steps the ceiling up as far as the
   * confident estimate carries, at the steadiness's hold and headroom (D352 §2–§3); never to a
   * quality found unplayable.
   */
  lookForStepUp(): void {
    const nowMs = this.#options.monotonicMs();
    this.#noteSteadiness(this.#steadiness.relax(nowMs));
    const ceiling = this.#ceiling;
    const confidentBps = this.#estimate.confidentBps(bufferAheadS(this.#hls));
    if (ceiling === undefined || this.#isStarting || confidentBps === undefined) {
      return;
    }
    const raised = nextCap({
      levels: this.#hls.levels,
      cap: ceiling,
      playing: this.#hls.currentLevel,
      estimateBps: confidentBps,
      bufferedAheadMs: bufferAheadS(this.#hls) * 1000,
      sinceChangeMs: nowMs - this.#changedAtMs,
      holdMs: this.#steadiness.holdMs(),
      headroom: this.#steadiness.headroom(),
      isOneAtATime: this.#steadiness.climbsOneAtATime() || !this.#estimate.agrees(),
    });
    const allowed = Math.min(raised, (this.#unplayableFrom ?? Number.POSITIVE_INFINITY) - 1);
    if (allowed > ceiling) {
      this.#ceilingWhy = `stepped up; ${this.#stateText()}`;
      this.#options.noteStep?.(
        `ceiling ${String(ceiling)} to ${String(allowed)}: ${this.#ceilingWhy}`,
      );
      this.#ceiling = allowed;
      this.#changedAtMs = nowMs;
      this.#steadiness.steppedUp(nowMs);
      this.#stepUpShown.steppedUp(nowMs, this.#hls.currentLevel, bufferAheadS(this.#hls));
    }
  }

  /**
   * Takes a chunk's measure of the link into the estimate, and tells the page its samples, and, once
   * the link is reported, its spread.
   */
  sample(sample: LinkSample): void {
    this.#estimate.add(sample, bufferAheadS(this.#hls));
    this.#options.noteSamples?.(samplesText(this.#estimate.samples()));
    const taken = lastSampleNote(this.#estimate);
    if (taken !== undefined) {
      this.#options.noteLink?.(taken);
    }
    if (this.#isReported()) {
      this.#options.noteLinkSpread?.(this.#estimate.spread());
    }
  }

  /** Whether the estimate is reported yet: once reportsLinkFromSamples chunks measured it. */
  #isReported(): boolean {
    return (
      this.#estimate.estimateBps() !== undefined && this.#estimate.count() >= reportsLinkFromSamples
    );
  }

  /**
   * Ends starting once the video played startingPlayedMs and its samples settle the ceiling or
   * startingSamples measured the link: the ceiling becomes the highest quality the confident
   * estimate carries, by D265 §4's share, no higher than it was (D352 §1), and the end is noted with
   * its samples.
   */
  endStartingWhenDone(playedMs: number): void {
    if (!this.#isStarting || !this.#hasStarted(playedMs)) {
      return;
    }
    this.#isStarting = false;
    const ceiling = this.#ceilingNow();
    const carried = this.#confidentlyCarried(this.#estimate) ?? ceiling;
    const held = this.#heldAbove(carried, ceiling);
    const lowered = held ?? carried;
    let change =
      lowered < ceiling
        ? `ceiling ${String(ceiling)} to ${String(lowered)}`
        : `ceiling ${String(ceiling)} kept`;
    if (held !== undefined) {
      change += `, held at ${String(held)}: link carries ${String(carried)}, ${this.#holdText()}`;
      this.#isHoldNoted = true;
    }
    this.#options.noteStep?.(
      `started, ${change}; ${this.#stateText()}; ${samplesText(this.#estimate.samples())}`,
    );
    if (lowered < ceiling) {
      this.#ceiling = lowered;
      this.#ceilingWhy = `started; ${this.#stateText()}`;
      this.#changedAtMs = this.#options.monotonicMs();
    }
  }

  /** Whether the video played startingPlayedMs, and its samples settle the ceiling or startingSamples measured the link. */
  #hasStarted(playedMs: number): boolean {
    return (
      playedMs >= startingPlayedMs &&
      (this.#estimate.count() >= startingSamples || this.#startingSettles())
    );
  }

  /**
   * Whether the samples settle the ceiling starting's end would set: it is the same with any one of
   * them left out (P5). Agreeing samples and a prior settle it in two or three; samples that
   * disagree take more; a lone sample never does, nothing being left without it.
   */
  #startingSettles(): boolean {
    const whole = this.#confidentlyCarried(this.#estimate);
    return (
      whole !== undefined &&
      this.#estimate.withoutEach().every((without) => this.#confidentlyCarried(without) === whole)
    );
  }

  /**
   * The highest quality, no higher than the ceiling, estimate's confident estimate carries by D265
   * §4's share; undefined before it has one.
   */
  #confidentlyCarried(estimate: LinkEstimate): number | undefined {
    const bps = estimate.confidentBps(bufferAheadS(this.#hls));
    return bps === undefined
      ? undefined
      : highestWithin(
          this.#hls.levels,
          this.#ceilingNow(),
          linkShare(bufferAheadS(this.#hls), this.#options.shareForQuality) * bps,
        );
  }

  /**
   * The quality held where the link carries only carried, under ceiling: the quality loading, no
   * higher than ceiling, while holdsQualityAheadS is loaded ahead and carried is below it; undefined
   * when nothing holds and the link decides.
   */
  #heldAbove(carried: number, ceiling: number): number | undefined {
    const loading = Math.min(this.#lastChosen, ceiling);
    if (loading <= carried || bufferAheadS(this.#hls) < holdsQualityAheadS) {
      this.#isHoldNoted = false;
      return undefined;
    }
    return loading;
  }

  /** Notes a hold after starting, once from when it begins until the link decides again (D352 §5). */
  #noteHeldOnce(why: () => string): void {
    if (this.#isStarting || this.#isHoldNoted) {
      return;
    }
    this.#isHoldNoted = true;
    this.#options.noteStep?.(`${why()}, ${this.#holdText()}; ${this.#stateText()}`);
  }

  /** Why a quality is held, for its note: what is loaded ahead, and from how much it holds. */
  #holdText(): string {
    return (
      `${bufferAheadS(this.#hls).toFixed(1)} s loaded ahead` +
      ` (holds from ${String(holdsQualityAheadS)} s)`
    );
  }

  /** Notes a change of the steadiness level, if there was one (D352 §5). */
  #noteSteadiness(change: string | undefined): void {
    if (change !== undefined) {
      this.#options.noteStep?.(change);
      this.#options.noteSteadiness?.(this.#steadiness.level());
    }
  }

  /**
   * What a choice rests on, for its note (D352 §5): the confident estimate and its parts, what is
   * loaded ahead, the steadiness level and the hold.
   */
  #stateText(): string {
    return (
      `${estimateText(this.#estimate, bufferAheadS(this.#hls))}, ${bufferAheadS(this.#hls).toFixed(1)} s ahead,` +
      ` steadiness ${String(this.#steadiness.level())},` +
      ` hold ${String(this.#steadiness.holdMs() / 1000)} s`
    );
  }

  /** The ceiling, the start until the levels are known and it is set from them. */
  #ceilingNow(): number {
    if (this.#ceiling === undefined && this.#hls.levels.length > 0) {
      this.#ceiling = this.#startingCeiling();
    }
    return this.#ceiling ?? 0;
  }

  /**
   * Where it starts: where the link its device remembers carries it, by the rule every later choice
   * plays by, nothing loaded ahead yet, so the first chunk is of the quality it plays and no switch
   * lands as playback begins (D381); else the start it was given (D257 §2, D260 §3).
   */
  #startingCeiling(): number {
    const { levels } = this.#hls;
    const remembered = this.#options.rememberedLink;
    if (this.#options.shouldStartByRememberedLink !== true || remembered === undefined) {
      return startingLevel(levels, this.#options.startAtBps);
    }
    const level = steadyLevel({
      levels,
      ceiling: levels.length - 1,
      linkBps: remembered.kbps * 1000,
      bufferAheadS: 0,
      shareForQuality: this.#options.shareForQuality,
    });
    this.#ceilingWhy = `first by the remembered link, ${String(Math.round(remembered.kbps))} kb/s`;
    return level;
  }

  /** Lowers the ceiling to level, a step down, saying whether it did; one at or above it changes nothing. */
  #lower(level: number, why: () => string): boolean {
    if (level >= this.#ceilingNow()) {
      return false;
    }
    this.#ceilingWhy = why();
    this.#options.noteStep?.(
      `ceiling ${String(this.#ceilingNow())} to ${String(level)}: ${this.#ceilingWhy}`,
    );
    this.#ceiling = level;
    this.#changedAtMs = this.#options.monotonicMs();
    this.#stepUpShown.lowered(level);
    return true;
  }

  /**
   * Chooses level for the next chunk, and keeps why, with the link's samples then, when it is a
   * change, for the switch to it (whyChose): a switch's line in the core's log said nothing of why
   * (staging, Nosferatu, 2026-10-28).
   */
  #choose(level: number, why: () => string): number {
    if (level !== this.#lastChosen) {
      this.#whyChosen.set(level, {
        why: why(),
        linkSamples: samplesText(this.#estimate.samples()),
      });
    }
    this.#lastChosen = level;
    return level;
  }

  #clamped(level: number): number {
    const { minAutoLevel, maxAutoLevel, levels } = this.#hls;
    if (levels.length === 0) {
      return level;
    }
    const highest = Math.min(maxAutoLevel, underCeiling(levels, this.#serverCeilingKbps));
    return Math.max(Math.min(level, highest), minAutoLevel);
  }

  /** The link's estimate, every choice's (D352 §2); undefined before a chunk measured it. */
  #linkBps(): number | undefined {
    return this.#estimate.estimateBps();
  }
}
