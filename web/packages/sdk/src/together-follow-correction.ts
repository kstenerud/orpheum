// SPDX-License-Identifier: MPL-2.0

import type { TogetherConnection } from "./together-connection.ts";
import {
  driftCorrection,
  learnedLossMs,
  nudgeForMs,
  seekOnlyCorrection,
  type LearnedLoss,
} from "./together-drift.ts";
import { catchUpSeekEvent, fellBehindEvent } from "./together-player-events.ts";
import { catchUpTargetMs, waitAheadAtMostMs, type LoadSpeed } from "./together-load-speed.ts";
import type { CatchUpAimNote } from "./together-notes.ts";
import type { Refill } from "./together-refill.ts";
import { playOrPlayMuted, type ShownAgain } from "./together-sound.ts";
import { returnsBeforeSeeksOnly } from "./together-follow-options.ts";
import type { OwnMoves } from "./together-own-moves.ts";
import {
  nextPhase,
  startingPhase,
  type CorrectionPhase,
  type PhaseEvent,
} from "./together-phase.ts";
import { haveFutureData, holdSeekMs, positionMsOf, type FollowedMedia } from "./together-media.ts";
import type { GroupNow } from "./together-lead.ts";
import type { SyncTuning } from "./together.ts";

/** What correcting a follower's drift needs: its player, its session, and its collaborators. */
export interface DriftCorrectorInit {
  readonly media: FollowedMedia;
  readonly connection: TogetherConnection;
  readonly monotonicMs: () => number;
  readonly ownMoves: OwnMoves;
  readonly refill: Refill;
  readonly loadSpeed: LoadSpeed;
  readonly shownAgain: ShownAgain;
}

/**
 * How one player (GroupFollower, together-follow.ts) corrects its drift from a playing group:
 * nudged away or sought across (#correct), by seeks alone once its rate's returns cost more than
 * the drift left alone (#correctBySeeks, D268 §2), catching up when it jumped ahead and waited
 * (#isWaitingAhead), or held paused at the group's position (holdAt). Where it is in correcting is
 * one CorrectionPhase, moved only by #move (together-phase.ts).
 */
export class DriftCorrector {
  readonly #media: FollowedMedia;
  readonly #connection: TogetherConnection;
  readonly #monotonicMs: () => number;
  readonly #ownMoves: OwnMoves;
  readonly #refill: Refill;
  readonly #loadSpeed: LoadSpeed;
  readonly #shownAgain: ShownAgain;
  /** Where it is in correcting its drift. */
  #phase: CorrectionPhase = startingPhase;
  /** Where its own last seek went, in ms; undefined before it made one. */
  #soughtToMs: number | undefined = undefined;
  /** What this player's rate going back to its group's costs it, as learned (driftCorrection). */
  #returnCost: LearnedLoss = { costMs: 0, moves: 0 };
  /**
   * Whether it corrects by seeks alone, its rate's returns costing it more than the drift left
   * alone (D268 §2): from then on, for as long as it follows.
   */
  #isCorrectingBySeeks = false;
  /** Where its seeks land short of where they aim, as learned (seekOnlyCorrection). */
  #seekLoss: LearnedLoss = { costMs: 0, moves: 0 };
  /** When (monotonic) it last changed its rate itself; undefined before it has. */
  #rateChangedAtMs: number | undefined = undefined;
  /** Until when (monotonic) it is not nudged: Safari's position stands still after a change. */
  #settlesAtMs = 0;
  /** When its own last seek started, on the monotonic clock, until it has loaded; else undefined. */
  #seekStartedAtMs: number | undefined = undefined;
  /** How long its own last seek took to load. */
  #lastLoadMs = 0;

  constructor(init: DriftCorrectorInit) {
    this.#media = init.media;
    this.#connection = init.connection;
    this.#monotonicMs = init.monotonicMs;
    this.#ownMoves = init.ownMoves;
    this.#refill = init.refill;
    this.#loadSpeed = init.loadSpeed;
    this.#shownAgain = init.shownAgain;
  }

  /** When it last changed its rate itself; undefined before it has (for a buffering note). */
  rateChangedAtMs(): number | undefined {
    return this.#rateChangedAtMs;
  }

  /** Where its own last seek went, and whether it is still loading (for holdAt's own seek). */
  #ownSeek(): { readonly isLoading: boolean; readonly toMs: number | undefined } {
    return { isLoading: this.#seekStartedAtMs !== undefined, toMs: this.#soughtToMs };
  }

  /**
   * Whether a player that jumped ahead still waits there for the group: until the group is within
   * the tuning's leaveDriftUpToMs of it, or has passed it, or has sought back from it further
   * than it could have jumped. Then it follows as usual, playing on with what it loaded meanwhile.
   */
  isWaitingAhead(where: GroupNow): boolean {
    if (this.#phase.kind !== "waitingAhead") {
      return false;
    }
    const aheadMs = this.#phase.atMs - where.expectedMs;
    const { leaveDriftUpToMs, seekDriftBeyondMs } = where.tuning;
    if (aheadMs > leaveDriftUpToMs && aheadMs <= waitAheadAtMostMs + seekDriftBeyondMs) {
      return true;
    }
    this.#move({ kind: "stoppedWaiting" });
    return false;
  }

  /** Plays along with a playing group, its drift nudged away or sought across. */
  play(where: GroupNow, hasCaughtUp: boolean): void {
    const media = this.#media;
    if (media.paused && !media.ended) {
      this.startPlaying();
    }
    // A player still loading is corrected by nothing: each correction would start its load again.
    if (
      (this.#phase.kind === "nudging" && where.nowMs < this.#phase.untilMs) ||
      media.seeking ||
      media.ended ||
      media.readyState < haveFutureData
    ) {
      return;
    }
    this.#correct(where, hasCaughtUp);
  }

  /** Plays, muted if refused, offered at a tap if shown again and still wanted (playOrPlayMuted). */
  startPlaying(): void {
    const isWanted = (): boolean =>
      this.#connection.snapshot().group?.state === "Playing" && this.#phase.kind !== "waitingAhead";
    playOrPlayMuted(this.#media, this.#connection, this.#shownAgain.play(isWanted));
  }

  /** Holds the player paused at the group's position: Paused, Waiting or Idle. */
  holdAt(where: GroupNow): void {
    const media = this.#media;
    this.#move({ kind: "stoppedWaiting" }); // a group that stops is followed where it stopped
    media.playbackRate = where.group.rate;
    if (!media.paused) {
      // Its own pause, never its person's: a host's play passes through a sync point's Waiting, and
      // this pause's event, arriving once the group played on, was sent to everyone as the host's
      // pause, so each play stopped 0.4 s later (the owner's test, 2026-10-08).
      this.#ownMoves.mark("pause", this.#monotonicMs());
      media.pause();
    }
    const toMs = holdSeekMs(media, where, this.#ownSeek());
    if (toMs !== undefined) {
      this.#seek(toMs, "hold");
    }
  }

  /**
   * Plays at rate, if it plays at another, settling its position; a nudge ending, behindMs behind,
   * has what that return costs learned once the position settles (#learnReturnCost).
   */
  setRate(rate: number, where: GroupNow, behindMs: number): void {
    const media = this.#media;
    if (rate === media.playbackRate) {
      return;
    }
    const isNudgeEnding = media.playbackRate !== where.group.rate && rate === where.group.rate;
    this.#move({ kind: "rateChanged", returnedBehindMs: isNudgeEnding ? behindMs : undefined });
    media.preservesPitch = where.tuning.shouldNudgeKeepPitch;
    media.playbackRate = rate;
    this.#rateChangedAtMs = this.#monotonicMs();
    this.settle();
  }

  /**
   * Starts refilling at a stall or a seek, and ends it once enough is loaded ahead; times how long
   * its own seek took to load. (The rest of noteLoading, following the group's refill, stays with
   * GroupFollower.)
   */
  notedProgress(): void {
    const media = this.#media;
    if (
      this.#seekStartedAtMs !== undefined &&
      media.readyState >= haveFutureData &&
      !media.seeking
    ) {
      this.#lastLoadMs = this.#monotonicMs() - this.#seekStartedAtMs;
      this.#seekStartedAtMs = undefined;
    }
  }

  #seek(toMs: number, why: "catchUp" | "hold" | "skip", aim: CatchUpAimNote = {}): void {
    this.#connection.note({
      kind: "sought",
      positionMs: positionMsOf(this.#media),
      toMs,
      why,
      ...aim,
    });
    this.#loadSpeed.forget();
    this.#move({ kind: "sought" });
    this.#ownMoves.mark("seek", this.#monotonicMs());
    this.#seekStartedAtMs = this.#monotonicMs();
    this.#refill.sought(why);
    this.#soughtToMs = toMs;
    this.#media.currentTime = Math.max(0, toMs) / 1000;
  }

  /** Moves its phase by event. */
  #move(event: PhaseEvent): void {
    this.#phase = nextPhase(this.#phase, event);
  }

  /** Reads no drift from the player's position for the tuning's settleAfterChangeMs from now. */
  settle(): void {
    const { tuning } = this.#connection.snapshot();
    this.#settlesAtMs = this.#monotonicMs() + tuning.settleAfterChangeMs;
  }

  /**
   * Settled after its last return to the group's rate, behindMs behind: what that return cost it.
   * Once its returns cost it more than the drift left alone, it corrects by seeks alone (D268 §2).
   */
  #learnReturnCost(behindMs: number, tuning: SyncTuning): void {
    if (this.#phase.kind !== "returning") {
      return;
    }
    this.#returnCost = learnedLossMs(this.#returnCost, this.#phase.behindMs, behindMs);
    this.#move({ kind: "learned" });
    const { costMs, moves } = this.#returnCost;
    if (moves >= returnsBeforeSeeksOnly && costMs > tuning.leaveDriftUpToMs) {
      this.#isCorrectingBySeeks = true;
      this.#connection.note({ kind: "correctsBySeeks", returnCostMs: Math.round(costMs) });
    }
  }

  /** Corrects a playing player's drift from the group: nudged away, or sought across. */
  #correct(where: GroupNow, hasCaughtUp: boolean): void {
    const media = this.#media;
    const behindMs = where.expectedMs - positionMsOf(media);
    if (this.#isCorrectingBySeeks) {
      this.#correctBySeeks(where, behindMs, hasCaughtUp);
      return;
    }
    const isNudging = media.playbackRate !== where.group.rate;
    const correction = driftCorrection(
      {
        expectedMs: where.expectedMs,
        actualMs: positionMsOf(media),
        groupRate: where.group.rate,
      },
      where.tuning,
      isNudging ? this.#returnCost.costMs : 0,
    );
    // Safari's position stands still up to 250 ms after a change (D250 §7): no nudge on that, but a
    // drift to seek across is far beyond what a still position makes, and is sought across at once.
    if (where.nowMs < this.#settlesAtMs && correction.kind !== "seek") {
      return;
    }
    this.#learnReturnCost(behindMs, where.tuning);
    // A player stalled and refilling is not nudged: its position does not move, and a faster rate
    // only needs a faster link once it plays (the owner's airplane test, 2026-10-06, where a frozen
    // iPhone was nudged to 2x second after second).
    const isNudged = correction.kind === "nudge" && !this.#refill.isRefilling;
    this.setRate(isNudged ? correction.rate : where.group.rate, where, behindMs);
    if (isNudged) {
      this.#move({ kind: "nudged", untilMs: where.nowMs + nudgeForMs });
      this.#connection.note({
        kind: "nudged",
        positionMs: positionMsOf(media),
        driftMs: where.expectedMs - positionMsOf(media),
        rate: correction.rate,
        returnCostMs: Math.round(this.#returnCost.costMs),
      });
    } else if (correction.kind === "seek") {
      this.#catchUp(correction.toMs, where, hasCaughtUp);
    }
  }

  /**
   * Corrects a playing player's drift, behindMs, by seeks alone: skipped across past the group by
   * what its skips have learned to lose, or sought across to catch up (D268 §2). Its rate is the
   * group's, a nudge under way when it began correcting so ending.
   */
  #correctBySeeks(where: GroupNow, behindMs: number, hasCaughtUp: boolean): void {
    const correction = seekOnlyCorrection(
      {
        expectedMs: where.expectedMs,
        actualMs: positionMsOf(this.#media),
        groupRate: where.group.rate,
      },
      where.tuning,
      this.#seekLoss.costMs,
    );
    if (where.nowMs < this.#settlesAtMs && correction.kind !== "seek") {
      return;
    }
    if (this.#phase.kind === "skipping") {
      this.#seekLoss = learnedLossMs(this.#seekLoss, this.#phase.behindMs, behindMs);
      this.#move({ kind: "learned" });
    }
    this.setRate(where.group.rate, where, behindMs);
    if (correction.kind === "skip" && !this.#refill.isRefilling) {
      this.#seek(correction.toMs, "skip");
      this.#move({ kind: "skipped", behindMs: where.expectedMs - correction.toMs });
    } else if (correction.kind === "seek") {
      this.#catchUp(correction.toMs, where, hasCaughtUp);
    }
  }

  /**
   * Seeks to catch up with the group at toMs, ahead as catchUpTargetMs says, and says it fell behind
   * if it had been in step. It waits there for the group, the host's player too: a host on a slow
   * link stalls as a viewer does (the owner, 2026-10-06), and its pause to wait is no command.
   */
  #catchUp(toMs: number, where: GroupNow, hasCaughtUp: boolean): void {
    if (hasCaughtUp) {
      this.#media.dispatchEvent(new Event(fellBehindEvent));
    }
    const aim = catchUpTargetMs(this.#media, { toMs, rate: where.group.rate }, () =>
      this.#loadSpeed.catchUpLeadMs(this.#lastLoadMs, where.group.rate, where.tuning),
    );
    const { targetMs } = aim;
    const ratio = this.#loadSpeed.ratio();
    this.#media.dispatchEvent(new Event(catchUpSeekEvent));
    this.#seek(targetMs, "catchUp", {
      groupAtMs: Math.round(toMs),
      aheadForLoadMs: Math.round(aim.aheadForLoadMs),
      aimedBy: aim.aimedBy,
      lastLoadMs: Math.round(this.#lastLoadMs),
      ...(ratio === undefined ? {} : { loadsFaster: Math.round(ratio * 10) / 10 }),
    });
    this.#move({ kind: "jumpedAhead", atMs: targetMs });
  }
}
