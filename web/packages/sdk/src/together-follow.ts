// SPDX-License-Identifier: MPL-2.0

import type { PlaybackRights, SyncTuning } from "./together.ts";
import {
  expectedGroupPositionMs,
  groupStartsAtMs,
  type GroupCommand,
  type PlayerState,
  type TogetherConnection,
} from "./together-connection.ts";
import {
  driftCorrection,
  learnedLossMs,
  nudgeForMs,
  seekOnlyCorrection,
  type LearnedLoss,
} from "./together-drift.ts";
import {
  catchUpSeekEvent,
  chunkLoadedOf,
  fellBehindEvent,
  playerLoadedChunkEvent,
  playerMovedItselfEvent,
  playerSwitchedQualityEvent,
  qualitySwitchedOf,
  type ChunkLoaded,
} from "./together-player-events.ts";
import { commandOfEvent, mayLead, type GroupNow } from "./together-lead.ts";
import { OwnMoves } from "./together-own-moves.ts";
import {
  nextPhase,
  startingPhase,
  type CorrectionPhase,
  type PhaseEvent,
} from "./together-phase.ts";
import {
  haveFutureData,
  haveMetadata,
  isInStep,
  isOnLoaded,
  loadedFromMs,
  playableAt,
  positionMsOf,
  type FollowedMedia,
} from "./together-media.ts";
import { LoadSpeed, waitAheadAtMostMs } from "./together-load-speed.ts";
import { markFollowingAGroup } from "./following-a-group.ts";
import { GroupLoading } from "./together-loading.ts";
import { Refill } from "./together-refill.ts";
import { playOrPlayMuted } from "./together-sound.ts";
import {
  memoryOfTheSession,
  runningPositionMs,
  StartLearner,
  type StartLagMemory,
} from "./together-start.ts";

/**
 * How long a leading player's pause or seek waits to be sent: a browser's own controls pause the
 * video while its person drags the timeline, and seek at each step of the drag, each within this of
 * the last. A pause with a seek after it is that seek, not a pause for everyone, and a drag is one
 * seek, to where it ended (the owner's tests, 2026-10-05).
 */
export const scrubPauseWindowMs = 300;

export {
  aimAheadAtMostMs,
  loadSampleMarginMs,
  loadSpanAtLeastMs,
  loadWindowMs,
  playerLoadsAheadMs,
  waitAheadAtMostMs,
} from "./together-load-speed.ts";

/**
 * How often a following player checks its drift while the group plays, and reports its state while
 * the group waits.
 */
export const checkDriftEveryMs = 1000;

/**
 * How many of its rate's returns to the group's a player learns from before it may correct by
 * seeks alone: one return's loss may be a stall's, and a Chrome taken for an iPhone would hold its
 * group less tightly for nothing (D268 §2).
 */
export const returnsBeforeSeeksOnly = 2;

/** How many of its player's last events a leading player's command note carries. */
export const recentEventsKept = 8;

/** How a player follows its group. */
export interface FollowOptions {
  /** The film this player plays: a group playing another is not followed. */
  readonly fileId: number;
  /**
   * Which of its person's moves are the group's: the host's every one, a guest's those their link
   * gives (D271 §4). A move it may not make is undone, the player put back where the group is.
   */
  readonly leads: PlaybackRights;
  readonly monotonicMs: () => number;
  /** Where the device keeps how late its player starts; for the follower's life when not given (D282). */
  readonly startLag?: StartLagMemory;
}

/** The events a following player tells the group of, and is moved by. */
const followedEvents = [
  "loadedmetadata",
  "canplay",
  "playing",
  "waiting",
  "play",
  "pause",
  "seeking",
  "seeked",
  "ended",
] as const;

/**
 * One player following its group, as followGroup describes it. Where it is in correcting its drift
 * is one CorrectionPhase, moved only by #move (together-phase.ts). The rest of its state is facts
 * that hold beside any phase: whether it has once been in step, is refilling, waits for the group's
 * answer to its command, has a seek loading, or corrects by seeks alone; and until when its
 * position is settling.
 */
class GroupFollower {
  readonly #media: FollowedMedia;
  readonly #connection: TogetherConnection;
  readonly #options: FollowOptions;
  /** The moves it makes itself, marked so their events are not taken for its person's. */
  readonly #ownMoves = new OwnMoves();
  /** Where it is in correcting its drift. */
  #phase: CorrectionPhase = startingPhase;
  /** Where its own last seek went, in ms; undefined before it made one. */
  #soughtToMs: number | undefined = undefined;
  /** What it tells its player to load while its group waits or another is behind (D283 §2, D285 §2). */
  readonly #loading: GroupLoading;
  /** How late it starts, as learned (D282). */
  readonly #start: StartLearner;
  /** Where it sought back to run up from, for the start under way (D282); undefined until it has. */
  #runUpFromMs: number | undefined = undefined;
  /** The player's last events, as "<event>[(own)]@<monotonic ms>", for a command's note. */
  #recentEvents: string[] = [];
  /** What this player's rate going back to its group's costs it, as learned (driftCorrection). */
  #returnCost: LearnedLoss = { costMs: 0, moves: 0 };
  /**
   * Whether it corrects by seeks alone, its rate's returns costing it more than the drift left
   * alone (D268 §2): from then on, for as long as it follows.
   */
  #isCorrectingBySeeks = false;
  /** Where its seeks land short of where they aim, as learned (seekOnlyCorrection). */
  #seekLoss: LearnedLoss = { costMs: 0, moves: 0 };
  /** Until when (monotonic) it is not nudged: Safari's position stands still after a change. */
  #settlesAtMs = 0;
  /** Whether it has once been where the group is: until then it holds no one up (D246 §6). */
  #hasCaughtUp = false;
  /** Whether it has stalled or sought, why, and whether it has loaded enough ahead to play on. */
  readonly #refill = new Refill();
  /** Whether the group was last told it buffers, so loading only tells it of a change. */
  #wasReportedBuffering: boolean | undefined = undefined;
  /**
   * Whether a command this player sent awaits the server's answer: until the group is told again or
   * the command refused, the player is left as its person put it, not pulled back to the group as
   * it was. A pong or a roster is no answer: taken for one, a pong arriving just after a host's play
   * held the player paused again (the M4 closing audit, 2026-10-08).
   */
  #isAwaitingGroup = false;
  /** The connection's answers heard when this player last sent a command (answersHeard). */
  #answersHeardAtCommand = 0;
  /** When its own last seek started, on the monotonic clock, until it has loaded; else undefined. */
  #seekStartedAtMs: number | undefined = undefined;
  /** How long its own last seek took to load. */
  #lastLoadMs = 0;
  /** How fast it loads, measured while it loads (D250 §4). */
  readonly #loadSpeed = new LoadSpeed();
  /** The timer that applies a playing group as it starts moving (#applyAt). */
  #startTimer: ReturnType<typeof setTimeout> | undefined = undefined;
  /** A leading player's pause or seek not yet sent, which a seek that follows it at once replaces. */
  #pendingCommand: ReturnType<typeof setTimeout> | undefined = undefined;
  /** The command pendingCommand will send, while one waits. */
  #pendingKind: GroupCommand["command"] | undefined = undefined;

  constructor(media: FollowedMedia, connection: TogetherConnection, options: FollowOptions) {
    this.#media = media;
    this.#connection = connection;
    this.#options = options;
    this.#start = new StartLearner(options.startLag ?? memoryOfTheSession());
    this.#loading = new GroupLoading(media, connection);
  }

  /** The player's state as the group is told it. */
  readonly playerState = (): PlayerState => {
    const isBuffering = this.#isBuffering();
    this.#wasReportedBuffering = isBuffering;
    const startLagMs = this.#start.lagMs();
    const loadsFaster = this.#refill.loadsFaster();
    return {
      positionMs: positionMsOf(this.#media),
      isPlaying: !this.#media.paused,
      isBuffering,
      readyOnMs: this.#refill.readyOnMs(),
      ...(startLagMs === undefined ? {} : { startLagMs }),
      ...(loadsFaster === undefined ? {} : { loadsFaster }),
    };
  };

  /** The connection changed: a command this player sent is answered once an answer was heard since. */
  readonly connectionChanged = (): void => {
    if (this.#connection.snapshot().answersHeard !== this.#answersHeardAtCommand) {
      this.#isAwaitingGroup = false;
    }
    this.apply();
  };

  /** Awaits the server's answer to a command this player is sending, or will once its drag ends. */
  #awaitAnswer(): void {
    this.#isAwaitingGroup = true;
    this.#answersHeardAtCommand = this.#connection.snapshot().answersHeard;
  }

  /** Puts the player where the group is now, when the group plays its film. */
  readonly apply = (): void => {
    const where = this.#whereNow();
    if (where === undefined || this.#isAwaitingGroup || this.#media.readyState < haveMetadata) {
      return;
    }
    if (where.group.state === "Playing") {
      this.#followPlaying(where);
    } else {
      this.#runUpFromMs = undefined;
      this.#holdAt(where);
    }
    this.#hasCaughtUp ||= isInStep(this.#media, where.expectedMs, where.tuning);
    this.#loading.follow(where.group.state, !this.#isBuffering());
  };

  /** Follows a playing group: waiting ahead of it, held until it starts, or playing along. */
  #followPlaying(where: GroupNow): void {
    if (this.#isWaitingAhead(where)) {
      // A player that jumped ahead to catch up waits there, paused and loading, for the group.
      if (!this.#media.paused) {
        this.#ownMoves.mark("pause", this.#options.monotonicMs());
        this.#media.pause();
      }
    } else if (where.nowMs < where.startsAtMs && this.#media.paused) {
      this.#startEarly(where);
    } else {
      this.#play(where);
    }
  }

  /**
   * Starts at the moment the server named, not as its message arrived (D268 §3), and early by how
   * late it has learned it starts: held where the group starts until then. One that has not learned
   * it runs up, from where the group would be had it already started, and learns it on the way
   * (D282).
   */
  #startEarly(where: GroupNow): void {
    const media = this.#media;
    const lagMs = this.#start.lagMs();
    if (lagMs !== undefined && where.nowMs < where.startsAtMs - lagMs) {
      this.#holdAt(where);
      this.#applyAt(where.startsAtMs - lagMs);
      return;
    }
    const runningMs = runningPositionMs(where.group, where.startsAtMs, where.nowMs);
    if (lagMs === undefined && this.#runUpFromMs === undefined) {
      // No earlier than what it loaded where the group starts: an earlier chunk is another to make.
      this.#runUpFromMs = Math.max(runningMs, loadedFromMs(media, where.group.positionMs));
      this.#seek(this.#runUpFromMs, "runUp");
      return;
    }
    if (media.seeking || media.readyState < haveFutureData) {
      return; // its run-up loads: it plays once it can, its events applying the group again
    }
    if (this.#runUpFromMs !== undefined && runningMs < this.#runUpFromMs) {
      this.#applyAt(
        where.startsAtMs - (where.group.positionMs - this.#runUpFromMs) / where.group.rate,
      );
      return; // a shortened run-up starts once the group's run reaches where it begins
    }
    this.#start.pressedPlay(runningMs - positionMsOf(media));
    this.#startPlaying();
  }

  /** One of the player's own events: a person's command when it leads, else a reason to apply. */
  readonly heard = (event: string): void => {
    this.#keepEvent(event);
    if (event === "playing" || event === "seeked") {
      this.#settle();
    }
    if (event === "playing") {
      this.#start.movedAfterPress();
    }
    this.#noteLoading(event);
    const isOwn = this.#isOwnMove(event);
    const where = this.#whereNow();
    const command =
      where !== undefined && !isOwn ? commandOfEvent(event, this.#media, where) : undefined;
    if (command === undefined || !mayLead(command, this.#options.leads)) {
      this.apply();
    } else {
      this.#lead(command);
    }
    // While its person's command waits to be sent, the player's loading is of a place the group
    // has not been told of, and would be taken for a stall: it is told once the command is sent.
    if (this.#pendingCommand === undefined) {
      this.#connection.reportState();
    }
  };

  /** Whether event is of a seek or pause the follower made itself, not its person: no command. */
  #isOwnMove(event: string): boolean {
    return this.#ownMoves.heard(event, this.#options.monotonicMs());
  }

  /**
   * Sends a leading player's command: a pause or a seek once no seek has followed it within
   * scrubPauseWindowMs, the last of a drag. Until then the player is left where its person put it.
   */
  #lead(command: GroupCommand): void {
    if (command.command !== "pause" && command.command !== "seek") {
      this.#send(command);
      return;
    }
    clearTimeout(this.#pendingCommand);
    this.#awaitAnswer();
    this.#pendingKind = command.command;
    this.#pendingCommand = setTimeout(() => {
      this.#pendingCommand = undefined;
      this.#pendingKind = undefined;
      this.#send(command);
    }, scrubPauseWindowMs);
  }

  /**
   * An iPhone's own full-screen player closed. iOS pauses the video as it closes: that pause is the
   * browser's, not its person's, and was sent as a pause for everyone each time a guest left full
   * screen (the owner's test, 2026-10-11). A pause waiting to be sent is dropped, one still to come
   * is taken for the player's own, and the video is put back where the group is.
   */
  readonly leftFullscreen = (): void => {
    if (this.#pendingKind === "pause") {
      clearTimeout(this.#pendingCommand);
      this.#pendingCommand = undefined;
      this.#pendingKind = undefined;
      this.#isAwaitingGroup = false;
    }
    this.#ownMoves.mark("pause", this.#options.monotonicMs());
    this.apply();
  };

  /**
   * Each checkDriftEveryMs: applies the group, and while it waits tells it this player's state, by
   * which the server ends a wait whose time is up (D247 §4).
   */
  readonly tick = (): void => {
    for (const move of this.#ownMoves.expired(this.#options.monotonicMs())) {
      this.#connection.note({ kind: "ownMarkExpired", why: move });
    }
    this.#loadSpeed.sample(this.#media, this.#options.monotonicMs());
    this.apply();
    if (this.#connection.snapshot().group?.state === "Waiting") {
      this.#connection.reportState();
    }
  };

  /** The browser loaded more: it moves nothing, and tells the group only if it made it ready. */
  readonly loaded = (): void => {
    this.#noteLoading("progress");
    this.#loadSpeed.sample(this.#media, this.#options.monotonicMs());
    if (this.#pendingCommand === undefined && this.#isBuffering() !== this.#wasReportedBuffering) {
      this.#connection.reportState();
    }
  };

  /** A chunk loaded: how fast the link carries the film, for when it is ready after a seek. */
  readonly chunkLoaded = (loaded: ChunkLoaded): void => {
    this.#refill.chunkLoaded(loaded);
  };

  /** The player moves the video itself: the seeking that follows is not its person's. */
  readonly playerMovedItself = (): void => {
    this.#ownMoves.mark("seek", this.#options.monotonicMs());
  };

  /** Stops what is pending, as the follower stops. */
  readonly stop = (): void => {
    clearTimeout(this.#pendingCommand);
    clearTimeout(this.#startTimer);
    this.#loading.stop();
  };

  /** Applies the group again at atMs, on the monotonic clock: when a playing group starts. */
  #applyAt(atMs: number): void {
    clearTimeout(this.#startTimer);
    this.#startTimer = setTimeout(this.apply, atMs - this.#options.monotonicMs());
  }

  /**
   * Starts refilling at a stall or a seek, and ends it once enough is loaded ahead; times how long
   * its own seek took to load.
   */
  #noteLoading(event: string): void {
    const media = this.#media;
    if (
      this.#seekStartedAtMs !== undefined &&
      media.readyState >= haveFutureData &&
      !media.seeking
    ) {
      this.#lastLoadMs = this.#options.monotonicMs() - this.#seekStartedAtMs;
      this.#seekStartedAtMs = undefined;
    }
    this.#refill.heard(event, media);
  }

  /** Whether the player holds the group up: not yet caught up, loading, or refilling. */
  #isBuffering(): boolean {
    const media = this.#media;
    return (
      !this.#hasCaughtUp ||
      media.readyState < haveFutureData ||
      media.seeking ||
      this.#refill.isShort(media)
    );
  }

  #send(command: GroupCommand): void {
    this.#awaitAnswer();
    this.#connection.command(command);
    if (command.command === "pause" || command.command === "seek" || command.command === "play") {
      // Which of its player's events made the command: a host's play stopped 0.4 s later by a
      // pause no one could say the source of (the owner's tests, 2026-10-08).
      this.#connection.note({
        kind: "leaderCommand",
        why: command.command,
        recentEvents: this.#recentEvents.join(" "),
      });
    }
    this.#connection.reportState();
  }

  /** Keeps event, its time and whether the follower made it, among the last recentEventsKept. */
  #keepEvent(event: string): void {
    const nowMs = this.#options.monotonicMs();
    const isOwn = this.#ownMoves.isOwn(event, nowMs);
    const atMs = Math.round(nowMs);
    this.#recentEvents = [
      ...this.#recentEvents,
      `${event}${isOwn ? "(own)" : ""}@${String(atMs)}`,
    ].slice(-recentEventsKept);
  }

  #whereNow(): GroupNow | undefined {
    const snapshot = this.#connection.snapshot();
    const { group } = snapshot;
    if (group?.fileId !== this.#options.fileId) {
      return undefined;
    }
    const nowMs = this.#options.monotonicMs();
    const startsAtMs = groupStartsAtMs(snapshot) ?? nowMs;
    // A player playing before its group starts runs up to it, judged by where it would be (D282).
    const isRunningUp = group.state === "Playing" && !this.#media.paused && nowMs < startsAtMs;
    return {
      group,
      nowMs,
      expectedMs: isRunningUp
        ? runningPositionMs(group, startsAtMs, nowMs)
        : (expectedGroupPositionMs(snapshot, nowMs) ?? group.positionMs),
      startsAtMs,
      tuning: snapshot.tuning,
    };
  }

  #seek(toMs: number, why: "catchUp" | "hold" | "skip" | "runUp"): void {
    this.#connection.note({ kind: "sought", positionMs: positionMsOf(this.#media), toMs, why });
    this.#loadSpeed.forget();
    this.#move({ kind: "sought" });
    this.#ownMoves.mark("seek", this.#options.monotonicMs());
    this.#seekStartedAtMs = this.#options.monotonicMs();
    this.#refill.sought(why);
    this.#soughtToMs = toMs;
    this.#media.currentTime = Math.max(0, toMs) / 1000;
  }

  /** Moves its phase by event. */
  #move(event: PhaseEvent): void {
    this.#phase = nextPhase(this.#phase, event);
  }

  /** Plays along with a playing group, its drift nudged away or sought across. */
  #play(where: GroupNow): void {
    const media = this.#media;
    if (media.paused && !media.ended) {
      this.#startPlaying();
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
    this.#correct(where);
  }

  /** Plays, muted if the browser will not play it with sound (playOrPlayMuted). */
  #startPlaying(): void {
    playOrPlayMuted(this.#media, this.#connection);
  }

  /** Reads no drift from the player's position for the tuning's settleAfterChangeMs from now. */
  #settle(): void {
    const { tuning } = this.#connection.snapshot();
    this.#settlesAtMs = this.#options.monotonicMs() + tuning.settleAfterChangeMs;
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

  /**
   * Plays at rate, if it plays at another, settling its position; a nudge ending, behindMs behind,
   * has what that return costs learned once the position settles (#learnReturnCost).
   */
  #setRate(rate: number, where: GroupNow, behindMs: number): void {
    const media = this.#media;
    if (rate === media.playbackRate) {
      return;
    }
    const isNudgeEnding = media.playbackRate !== where.group.rate && rate === where.group.rate;
    this.#move({ kind: "rateChanged", returnedBehindMs: isNudgeEnding ? behindMs : undefined });
    media.preservesPitch = where.tuning.shouldNudgeKeepPitch;
    media.playbackRate = rate;
    this.#settle();
  }

  /** Corrects a playing player's drift from the group: nudged away, or sought across. */
  #correct(where: GroupNow): void {
    const media = this.#media;
    const behindMs = where.expectedMs - positionMsOf(media);
    if (this.#isCorrectingBySeeks) {
      this.#correctBySeeks(where, behindMs);
      return;
    }
    const isNudging = media.playbackRate !== where.group.rate;
    const correction = driftCorrection(
      { expectedMs: where.expectedMs, actualMs: positionMsOf(media), groupRate: where.group.rate },
      where.tuning,
      isNudging ? this.#returnCost.costMs : 0,
    );
    // Safari's position stands still up to 250 ms after a change (D250 §7): no nudge on that, but a
    // drift to seek across is far beyond what a still position makes, and is sought across at once.
    if (where.nowMs < this.#settlesAtMs && correction.kind !== "seek") {
      return;
    }
    this.#learnReturnCost(behindMs, where.tuning);
    this.#start.settled(behindMs);
    // A player stalled and refilling is not nudged: its position does not move, and a faster rate
    // only needs a faster link once it plays (the owner's airplane test, 2026-10-06, where a frozen
    // iPhone was nudged to 2x second after second).
    const isNudged = correction.kind === "nudge" && !this.#refill.isRefilling;
    this.#setRate(isNudged ? correction.rate : where.group.rate, where, behindMs);
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
      this.#catchUp(correction.toMs, where);
    }
  }

  /**
   * Corrects a playing player's drift, behindMs, by seeks alone: skipped across past the group by
   * what its skips have learned to lose, or sought across to catch up (D268 §2). Its rate is the
   * group's, a nudge under way when it began correcting so ending.
   */
  #correctBySeeks(where: GroupNow, behindMs: number): void {
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
    this.#setRate(where.group.rate, where, behindMs);
    if (correction.kind === "skip" && !this.#refill.isRefilling) {
      this.#seek(correction.toMs, "skip");
      this.#move({ kind: "skipped", behindMs: where.expectedMs - correction.toMs });
    } else if (correction.kind === "seek") {
      this.#catchUp(correction.toMs, where);
    }
  }

  /**
   * Whether a player that jumped ahead still waits there for the group: until the group is within
   * the tuning's leaveDriftUpToMs of it, or has passed it, or has sought back from it further
   * than it could have jumped. Then it follows as usual, playing on with what it loaded meanwhile.
   */
  #isWaitingAhead(where: GroupNow): boolean {
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

  /**
   * Seeks to catch up with the group at toMs, ahead by catchUpLeadMs, and says it fell behind if it
   * had been in step. It waits there for the group, the host's player too: a host on a slow link
   * stalls as a viewer does (the owner, 2026-10-06), and its pause to wait is no command.
   */
  #catchUp(toMs: number, where: GroupNow): void {
    if (this.#hasCaughtUp) {
      this.#media.dispatchEvent(new Event(fellBehindEvent));
    }
    const targetMs =
      toMs + this.#loadSpeed.catchUpLeadMs(this.#lastLoadMs, where.group.rate, where.tuning);
    this.#media.dispatchEvent(new Event(catchUpSeekEvent));
    this.#seek(targetMs, "catchUp");
    this.#move({ kind: "jumpedAhead", atMs: targetMs });
  }

  /** Holds the player paused at the group's position: Paused, Waiting or Idle. */
  #holdAt(where: GroupNow): void {
    const media = this.#media;
    this.#move({ kind: "stoppedWaiting" }); // a group that stops is followed where it stopped
    media.playbackRate = where.group.rate;
    if (!media.paused) {
      // Its own pause, never its person's: a host's play passes through a sync point's Waiting, and
      // this pause's event, arriving once the group played on, was sent to everyone as the host's
      // pause, so each play stopped 0.4 s later (the owner's test, 2026-10-08).
      this.#ownMoves.mark("pause", this.#options.monotonicMs());
      media.pause();
    }
    // While its own seek still loads, the browser's player moves it a little as it loads (hls.js
    // steps over a gap at the start of what it loaded): a seek for that would start the load again,
    // and again, and the player never loads at all (the owner's test, 2026-10-05, on a slow link).
    const holdAtMs = playableAt(media, where.expectedMs, where.tuning);
    const awayMs = Math.abs(positionMsOf(media) - holdAtMs);
    const isLoadingOwnSeek = this.#seekStartedAtMs !== undefined;
    // A player held just before the first frame it loaded, by however little, is moved onto it: a
    // paused player never steps over the gap, and one not on what it loaded is never ready. Both
    // players held at 0 with 0.04 s to 60 s loaded, a gap under the drift left alone, and held the
    // group up for good (staging, 2026-10-06).
    const isBeforeWhatItLoaded =
      !isLoadingOwnSeek && holdAtMs !== where.expectedMs && !isOnLoaded(media);
    const { leaveDriftUpToMs, seekDriftBeyondMs } = where.tuning;
    const isAway = awayMs > (isLoadingOwnSeek ? seekDriftBeyondMs : leaveDriftUpToMs);
    if ((isAway || isBeforeWhatItLoaded) && this.#maySeekTo(holdAtMs, seekDriftBeyondMs)) {
      this.#seek(holdAtMs, "hold");
    }
  }

  /**
   * Whether it may seek to toMs now: not while it still loads a seek, unless that seek went more than
   * farMs from toMs. A group sought again while this player still loads where it went before is
   * followed at once: an iPhone loading a chunk not yet made went on waiting for it, 3.5 to 9 s,
   * before it asked for the new place (the owner's jumps, 2026-10-12).
   */
  #maySeekTo(toMs: number, farMs: number): boolean {
    return (
      !this.#media.seeking ||
      (this.#soughtToMs !== undefined && Math.abs(toMs - this.#soughtToMs) > farMs)
    );
  }
}

/**
 * Makes media follow the group while it plays options.fileId (watch-together.md, Sync): paused at
 * the group's position while it is Paused, Waiting or Idle; playing while it is Playing, its drift
 * nudged away by the rate or sought across. It tells the group its state on every change, and
 * counts as buffering until it has once been where the group is, so a player still loading holds
 * no one up (D246 §6), and after a stall or a seek until it has refillAheadMs loaded. A leading
 * player's own play, pause and seek become commands; the follower's own do not. Returns the
 * stopping.
 */
export function followGroup(
  media: FollowedMedia,
  connection: TogetherConnection,
  options: FollowOptions,
): () => void {
  const follower = new GroupFollower(media, connection, options);
  const listeners = [
    ...followedEvents.map(
      (event) =>
        [
          event,
          () => {
            follower.heard(event);
          },
        ] as const,
    ),
    ["progress", follower.loaded] as const,
    [playerMovedItselfEvent, follower.playerMovedItself] as const,
    // An iPhone's own full-screen player, which pauses the video as it closes.
    ["webkitendfullscreen", follower.leftFullscreen] as const,
    [
      playerLoadedChunkEvent,
      (event: Event) => {
        const loaded = chunkLoadedOf(event);
        if (loaded !== undefined) {
          follower.chunkLoaded(loaded);
          connection.note({ kind: "chunkLoaded", ...loaded });
        }
      },
    ] as const,
    [
      playerSwitchedQualityEvent,
      (event: Event) => {
        const switched = qualitySwitchedOf(event);
        if (switched !== undefined) {
          connection.note({ kind: "qualitySwitched", ...switched });
        }
      },
    ] as const,
  ];
  for (const [event, listener] of listeners) {
    media.addEventListener(event, listener);
  }
  connection.setPlayer(follower.playerState);
  const unmarkFollowing = markFollowingAGroup();
  const unsubscribe = connection.subscribe(follower.connectionChanged);
  const checker = setInterval(follower.tick, checkDriftEveryMs);
  follower.apply();
  connection.reportState();
  return () => {
    clearInterval(checker);
    follower.stop();
    unmarkFollowing();
    unsubscribe();
    connection.setPlayer(undefined);
    for (const [event, listener] of listeners) {
      media.removeEventListener(event, listener);
    }
  };
}
