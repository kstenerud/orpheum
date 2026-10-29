// SPDX-License-Identifier: MPL-2.0

import {
  arrivalMs,
  giveUpFor,
  givenUpSample,
  givenUpText,
  highestWithin,
  linkShare,
  isUnplayable,
  keptOwnPicture,
  ownPictureLevel,
  commonStartForMs,
  commonStartLevel,
  measuresTheLink,
  startingLevel,
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
import { estimateText, LinkEstimate, samplesText, type LinkSample } from "./link-estimate.ts";
import { ChunkOnItsWay, type JudgedChunk } from "./abr-on-its-way.ts";
import { PlayedTime, StandingStill } from "./abr-video.ts";
import { Steadiness } from "./steadiness.ts";
import { nextCap, stepUpBufferMs } from "./step-up.ts";

/** How often the controller looks at a chunk on its way (D265 §3). */
export const lookAtChunkEveryMs = 500;

/** How often it looks at whether it may step up a quality (D247 §9). */
export const lookForStepUpEveryMs = 10_000;

/**
 * How many chunks must have measured the link, and how long the playback must have played, before
 * it stops starting (D352 §1): four are the fewest whose spread says anything, and by 20 s played
 * the player has filled its first buffer, a dozen chunks or more, so a bad first sample has left the
 * estimate's five.
 */
export const startingSamples = 4;
export const startingPlayedMs = 20_000;

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

/**
 * How many chunks must have measured the link before it is reported, and so remembered for the
 * device's next start (D257 §1): never a lone first sample (D352 §1).
 */
export const reportsLinkFromSamples = 2;

/**
 * How far above its quality's rate, its BANDWIDTH, a chunk's download is paced while voice is
 * live: enough that a busy chunk still arrives well before it plays (D262 §2).
 */
export const paceAboveQuality = 1.25;

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
  readonly #link: ChoosingLink<Chunk>;
  /** The highest quality it may choose: the start, lowered by each step down, raised by the step-up. */
  #ceiling: number | undefined;
  #changedAtMs: number;
  /** The chunk of picture on its way, followed to judge whether to give it up. */
  readonly #onItsWay = new ChunkOnItsWay<Chunk>();
  /** The pace each paced chunk was asked to arrive at, bits a second. */
  readonly #paces = new WeakMap<Chunk, number>();
  /** A quality hls.js forced, after a load error, until a chunk of it loads (D265 §8). */
  #forced: number | undefined;
  #lastChosen = -1;
  /** Why each quality was last chosen, by level, for the switch to it (whyChose). */
  readonly #whyChosen = new Map<number, ChoiceWhy>();
  /** Why the ceiling stands where it does: "first" until it changes, then its last change's reason. */
  #ceilingWhy = "first";
  /**
   * The link as its last chunks measured it, which every choice reads (D352 §2): hls.js's estimate,
   * weighted by each chunk's seconds, held one slow first sample of 1008 kb/s for minutes on a
   * 12 Mb/s path (the owner, on staging, 2026-10-26).
   */
  readonly #estimate: LinkEstimate;
  /** How steady the link has been, which sets the step-up's hold and headroom (D352 §3–§4). */
  readonly #steadiness: Steadiness;
  /**
   * Whether the playback is starting: until startingSamples chunks measured the link and it played
   * startingPlayedMs, a choice below the ceiling does not lower it (D352 §1).
   */
  #isStarting = true;
  /** Whether the hold of the quality loading (holdsQualityAheadS) was noted since it began. */
  #isHoldNoted = false;
  /** How long the video has played. */
  readonly #played = new PlayedTime();
  /** Where the video, asked to play, has stood still, and what came meanwhile. */
  readonly #still = new StandingStill();
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
    this.#estimate = new LinkEstimate(options.rememberedLink);
    this.#steadiness = new Steadiness(options.monotonicMs(), options.startSteadiness);
    options.noteSteadiness?.(this.#steadiness.level());
    this.#timers = [
      setInterval(() => {
        this.#lookAtChunk();
        this.#lookAtPlaying();
        this.#played.look(this.#hls.media, this.#options.monotonicMs());
        this.#endStartingWhenDone();
      }, lookAtChunkEveryMs),
      setInterval(() => {
        this.#lookForStepUp();
      }, lookForStepUpEveryMs),
    ];
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
      bufferAheadS: this.#bufferAheadS(),
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

  /** A chunk of picture loaded. */
  loaded(chunk: Chunk): void {
    this.#still.chunkLoaded();
    this.#onItsWay.ended(chunk);
    if (this.#forced === chunk.level) {
      this.#forced = undefined;
    }
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
      this.#sample({
        level: chunk.level,
        bytes: loaded,
        firstByteMs: Math.max(0, loading.first - loading.start),
        arrivedMs: arrivalMs(loading),
      });
      this.#endStartingWhenDone();
    }
    return isCounted;
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

  /** The playback's steadiness level (D352 §3). */
  steadiness(): number {
    return this.#steadiness.level();
  }

  /** Whether the playback is still starting (D352 §1). */
  isStarting(): boolean {
    return this.#isStarting;
  }

  stop(): void {
    for (const timer of this.#timers) {
      clearInterval(timer);
    }
  }

  #lookAtChunk(): void {
    const judged = this.#onItsWay.look(this.#options.monotonicMs());
    if (judged === undefined) {
      return;
    }
    const { chunk } = judged;
    const chosen = giveUpFor({
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
      bufferAheadS: this.#bufferAheadS(),
    });
    if (chosen !== undefined) {
      this.#giveUp(judged, chosen);
    }
  }

  /**
   * Gives up a chunk on its way for chosen (D265 §3), a drop (D352 §4). One that was arriving is a
   * sample of the link, hls.js's estimator fed it too, and lowers the ceiling to chosen; one stuck is
   * no sample, and lowers the ceiling while starting only as far as the estimate then carries, as a
   * stall does (bandwidth-sampling-2026-10-29.md P3): a stuck transport is not a slow link, and a
   * lower quality's chunk travels it no better.
   */
  #giveUp(judged: JudgedChunk<Chunk>, chosen: number): void {
    const { chunk } = judged;
    this.#onItsWay.ended(chunk);
    const nowMs = this.#options.monotonicMs();
    const sample = givenUpSample({ ...judged, level: chunk.level });
    if (sample !== undefined) {
      this.#link.measureGivenUp(sample.firstByteMs + sample.arrivedMs, sample.bytes);
      this.#sample(sample);
    }
    const why = `gave up a chunk of ${String(chunk.level)}, ${givenUpText({
      ...judged,
      isSample: sample !== undefined,
    })}; ${this.#stateText()}`;
    const isStuckStarting = sample === undefined && this.#isStarting;
    if (!this.#lower(isStuckStarting ? Math.max(chosen, this.#carriedNow()) : chosen, () => why)) {
      this.#options.noteStep?.(why); // a give-up is always told, the ceiling lowered or not
    }
    this.#noteSteadiness(this.#steadiness.dropped(nowMs));
    this.#choose(chosen, () => why);
    this.#link.giveUp(chunk);
  }

  /** The quality the link's estimate carries now, under the ceiling, by D265 §4's share; the ceiling with no estimate. */
  #carriedNow(): number {
    return steadyLevel({
      levels: this.#hls.levels,
      ceiling: this.#ceilingNow(),
      linkBps: this.#linkBps(),
      bufferAheadS: this.#bufferAheadS(),
      shareForQuality: this.#options.shareForQuality,
    });
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
    const still = this.#still.look(this.#hls.media, nowMs);
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
  #lookForStepUp(): void {
    const nowMs = this.#options.monotonicMs();
    this.#noteSteadiness(this.#steadiness.relax(nowMs));
    const ceiling = this.#ceiling;
    const confidentBps = this.#estimate.confidentBps(this.#bufferAheadS());
    if (ceiling === undefined || this.#isStarting || confidentBps === undefined) {
      return;
    }
    const raised = nextCap({
      levels: this.#hls.levels,
      cap: ceiling,
      playing: this.#hls.currentLevel,
      estimateBps: confidentBps,
      bufferedAheadMs: this.#bufferAheadS() * 1000,
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
    }
  }

  /**
   * Takes a chunk's measure of the link into the estimate, and tells the page its samples, and, once
   * the link is reported, its spread.
   */
  #sample(sample: LinkSample): void {
    this.#estimate.add(sample);
    this.#options.noteSamples?.(samplesText(this.#estimate.samples()));
    if (this.linkKbps() !== undefined) {
      this.#options.noteLinkSpread?.(this.#estimate.spread());
    }
  }

  /**
   * Ends starting once startingSamples chunks measured the link and the video played
   * startingPlayedMs: the ceiling becomes the highest quality the confident estimate carries, by
   * D265 §4's share, no higher than it was (D352 §1), and the end is noted with its samples.
   */
  #endStartingWhenDone(): void {
    if (
      !this.#isStarting ||
      this.#estimate.count() < startingSamples ||
      this.#played.playedMs() < startingPlayedMs
    ) {
      return;
    }
    this.#isStarting = false;
    const ceiling = this.#ceilingNow();
    const carried = highestWithin(
      this.#hls.levels,
      ceiling,
      linkShare(this.#bufferAheadS(), this.#options.shareForQuality) *
        (this.#estimate.confidentBps(this.#bufferAheadS()) ?? 0),
    );
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

  /**
   * The quality held where the link carries only carried, under ceiling: the quality loading, no
   * higher than ceiling, while holdsQualityAheadS is loaded ahead and carried is below it; undefined
   * when nothing holds and the link decides.
   */
  #heldAbove(carried: number, ceiling: number): number | undefined {
    const loading = Math.min(this.#lastChosen, ceiling);
    if (loading <= carried || this.#bufferAheadS() < holdsQualityAheadS) {
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
      `${this.#bufferAheadS().toFixed(1)} s loaded ahead` +
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
      `${estimateText(this.#estimate, this.#bufferAheadS())}, ${this.#bufferAheadS().toFixed(1)} s ahead,` +
      ` steadiness ${String(this.#steadiness.level())},` +
      ` hold ${String(this.#steadiness.holdMs() / 1000)} s`
    );
  }

  /** The ceiling, the start until the levels are known and it is set from them. */
  #ceilingNow(): number {
    if (this.#ceiling === undefined && this.#hls.levels.length > 0) {
      this.#ceiling = startingLevel(this.#hls.levels, this.#options.startAtBps);
    }
    return this.#ceiling ?? 0;
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

  /**
   * The server's ceiling, kbps, with a chunk's answer: the highest rate it lets this player load,
   * its quality's average with its sound's, so its upload carries every player (D286 §5); undefined
   * lifts it. The next chunk is of a quality under it, and the step-up stays under it.
   */
  obeyServer(kbps: number | undefined): void {
    this.#serverCeilingKbps = kbps;
  }

  /** The link's estimate, every choice's (D352 §2); undefined before a chunk measured it. */
  #linkBps(): number | undefined {
    return this.#estimate.estimateBps();
  }

  #bufferAheadS(): number {
    return this.#hls.mainForwardBufferInfo?.len ?? 0;
  }
}
