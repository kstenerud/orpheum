// SPDX-License-Identifier: MPL-2.0

import type { SyncTuning } from "./together.ts";
import {
  expectedGroupPositionMs,
  isHostAwaitingLead,
  isAskedByItsGroup,
  isFollowerOf,
  isLeaderOf,
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
  GroupJumpsTold,
  playerLoadedChunkEvent,
  playerMovedItselfEvent,
  playerSwitchedQualityEvent,
  noteQualitySwitched,
  playerMovedSoundEvent,
  noteSoundMoved,
  type ChunkLoaded,
} from "./together-player-events.ts";
import { GroupLeader, PlaybackTold } from "./together-leader.ts";
import {
  commandOfEvent,
  mayLead,
  sendAfterMs,
  whenVisibilityChanges,
  type GroupNow,
} from "./together-lead.ts";
import { FollowedChanges, heldByOf, notePlayedWhileHeld } from "./together-followed.ts";
import { OwnMoves } from "./together-own-moves.ts";
import {
  nextPhase,
  startingPhase,
  type CorrectionPhase,
  type PhaseEvent,
} from "./together-phase.ts";
import {
  haveFutureData,
  holdSeekMs,
  isInStep,
  positionMsOf,
  type FollowedMedia,
} from "./together-media.ts";
import { catchUpTargetMs, LoadSpeed, waitAheadAtMostMs } from "./together-load-speed.ts";
import { markFollowingAGroup } from "./following-a-group.ts";
import { GroupLoading } from "./together-loading.ts";
import { Refill } from "./together-refill.ts";
import { playOrPlayMuted } from "./together-sound.ts";
import { startGateOf, type StartGate } from "./start-gate.ts";
import { bufferingCauseOf, bufferingNote, HoldingNotes } from "./together-buffering.ts";
import {
  checkDriftEveryMs,
  followedEvents,
  recentEventsKept,
  returnsBeforeSeeksOnly,
  type FollowOptions,
} from "./together-follow-options.ts";

export { browserPauseWindowMs, scrubPauseWindowMs, type ShownPage } from "./together-lead.ts";
export {
  aimAheadAtMostMs,
  loadedCatchUpLeadMs,
  loadSampleMarginMs,
  loadSpanAtLeastMs,
  loadWindowMs,
  playerLoadsAheadMs,
  waitAheadAtMostMs,
} from "./together-load-speed.ts";
export {
  checkDriftEveryMs,
  recentEventsKept,
  returnsBeforeSeeksOnly,
  type FollowOptions,
} from "./together-follow-options.ts";

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
  /** When it moved for each change of its group, against when the change reached it. */
  readonly #followed: FollowedChanges;
  /** Where it is in correcting its drift. */
  #phase: CorrectionPhase = startingPhase;
  /** The group's jumps it told its player of. */
  readonly #jumps = new GroupJumpsTold();
  /** Where its own last seek went, in ms; undefined before it made one. */
  #soughtToMs: number | undefined = undefined;
  /** What it tells its player to load while its group waits or another is behind (D283 §2, D285 §2). */
  readonly #loading: GroupLoading;
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
  /** When (monotonic) it last changed its rate itself; undefined before it has. */
  #rateChangedAtMs: number | undefined = undefined;
  /** Until when (monotonic) it is not nudged: Safari's position stands still after a change. */
  #settlesAtMs = 0;
  /** Whether it has once been where the group is: until then it holds no one up (D246 §6). */
  #hasCaughtUp = false;
  /** Whether it has stalled or sought, why, and whether it has loaded enough ahead to play on. */
  readonly #refill = new Refill();
  /** Whether the group was last told it buffers, so loading only tells it of a change. */
  #wasReportedBuffering: boolean | undefined = undefined;
  /** When it notes what holds its group up. */
  readonly #holding = new HoldingNotes();
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
  /** A leading player's pause or seek not yet sent, which a seek that follows it at once replaces. */
  #pendingCommand: ReturnType<typeof setTimeout> | undefined = undefined;
  /** The command pendingCommand will send, while one waits. */
  #pendingKind: GroupCommand["command"] | undefined = undefined;
  /** What it carries out of its group's changes while it leads (D371 §1). */
  readonly #leader: GroupLeader;
  /** How it plays its film, for a cast's remote (D371 §4). */
  readonly #playback: PlaybackTold;
  /** The wait before it plays while it leads, as a player alone waits (start-gate.ts). */
  readonly #startGate: StartGate;
  /** Ends its start gate's standing aside, which it does while it follows, its follower moving it. */
  readonly #stopStandingAside: () => void;
  /** Ends its start gate's counting it asked to play while its group asks it (isAskedByItsGroup). */
  readonly #stopAsking: () => void;

  constructor(media: FollowedMedia, connection: TogetherConnection, options: FollowOptions) {
    this.#media = media;
    this.#connection = connection;
    this.#options = options;
    this.#loading = new GroupLoading(media, connection);
    this.#followed = new FollowedChanges(connection.snapshot().groupSeq);
    const startGate = startGateOf(media);
    this.#startGate = startGate;
    // Aside only once its welcome says it follows: until then a play its page asks waits, as a
    // leader's does, and a follower's is given up at its welcome, its follower playing it.
    this.#stopStandingAside = startGate.standAsideWhile(() =>
      isFollowerOf(this.#connection.snapshot()),
    );
    this.#stopAsking = startGate.asksToPlayWhile(() => isAskedByItsGroup(connection.snapshot()));
    this.#leader = new GroupLeader({
      media,
      connection,
      ownMoves: this.#ownMoves,
      monotonicMs: options.monotonicMs,
      play: () => {
        startGate.play(() => {
          this.#startPlaying();
        });
      },
      startGate,
    });
    this.#playback = new PlaybackTold(options.delivery);
  }

  /** Whether this device leads its session, as its roster says (D371 §1). */
  #isLeading(): boolean {
    return isLeaderOf(this.#connection.snapshot());
  }

  /** The player's state as the group is told it: a leader buffers only while it cannot play on. */
  readonly playerState = (): PlayerState => {
    const isLeading = this.#isLeading();
    const isBuffering = isLeading ? this.#leader.isLoading() : this.#isBuffering();
    this.#noteBuffering(isBuffering, isLeading);
    this.#wasReportedBuffering = isBuffering;
    const loadsFaster = this.#refill.loadsFaster();
    const playback = this.#playback.report();
    return {
      positionMs: positionMsOf(this.#media),
      isPlaying: !this.#media.paused,
      isBuffering,
      readyOnMs: this.#refill.readyOnMs(this.#media),
      ...(loadsFaster === undefined ? {} : { loadsFaster }),
      ...(playback === undefined ? {} : { playback }),
      ...(this.#options.page?.visibilityState === "hidden" ? { isPageHidden: true } : {}),
    };
  };

  /** Notes what holds it, or its group, up, when HoldingNotes says to (bufferingNote). */
  #noteBuffering(isBuffering: boolean, isLeading: boolean): void {
    const cause = isLeading
      ? this.#leader.loadingCause()
      : bufferingCauseOf(this.#media, this.#hasCaughtUp, this.#refill);
    const why = isBuffering ? cause : undefined;
    if (this.#holding.isToNote(why, this.#connection.snapshot()) && why !== undefined) {
      const changedAtMs = this.#rateChangedAtMs;
      const sinceMs =
        changedAtMs === undefined ? undefined : this.#options.monotonicMs() - changedAtMs;
      this.#connection.note(bufferingNote(this.#media, why, sinceMs));
    }
  }

  /** The connection changed: a command this player sent is answered once an answer was heard since. */
  readonly connectionChanged = (): void => {
    const snapshot = this.#connection.snapshot();
    if (snapshot.answersHeard !== this.#answersHeardAtCommand) {
      this.#isAwaitingGroup = false;
    }
    this.#followed.heard(snapshot.groupSeq, this.#options.monotonicMs());
    this.apply();
  };

  /** Awaits the server's answer to a command this player is sending, or will once its drag ends. */
  #awaitAnswer(): void {
    this.#isAwaitingGroup = true;
    this.#answersHeardAtCommand = this.#connection.snapshot().answersHeard;
  }

  /**
   * Puts the player where the group is now, when the group plays its film; not while held (heldByOf).
   * A leader is put nowhere: it carries out what another device asked, and loads as it would alone.
   */
  readonly apply = (): void => {
    const where = this.#whereNow();
    const heldBy = heldByOf(this.#isAwaitingGroup, this.#options.page, this.#media);
    const nowMs = this.#options.monotonicMs();
    if (where === undefined || !this.#followed.mayFollow(heldBy, nowMs, this.#connection)) {
      return;
    }
    if (this.#isLeading()) {
      this.#hasCaughtUp = true; // its person's moves are the group's from the first
      this.#loading.stop();
      this.#leader.lead(where, this.#connection.snapshot().groupSeq);
      this.#jumps.tell(this.#media, where.group);
      return;
    }
    if (where.group.state === "Playing") {
      this.#followPlaying(where);
    } else {
      this.#holdAt(where);
    }
    this.#hasCaughtUp ||= isInStep(this.#media, where.expectedMs, where.tuning);
    this.#loading.follow(where.group.state, !this.#isBuffering());
    this.#jumps.tell(this.#media, where.group);
  };

  /**
   * Follows a playing group: waiting ahead of it, or playing along at once, as the group plays from
   * where its leader plays, and catching up to it, nudged or sought (D371 §2): no group names a
   * moment to start at, and none is run up to.
   */
  #followPlaying(where: GroupNow): void {
    if (this.#isWaitingAhead(where)) {
      // A player that jumped ahead to catch up waits there, paused and loading, for the group.
      if (!this.#media.paused) {
        this.#ownMoves.mark("pause", this.#options.monotonicMs());
        this.#media.pause();
      }
    } else {
      this.#play(where);
    }
  }

  /** One of the player's own events: a person's command when it leads, else a reason to apply. */
  readonly heard = (event: string): void => {
    this.#keepEvent(event);
    if (event === "playing" || event === "seeked") {
      this.#settle();
    }
    this.#noteLoading(event);
    const isOwn = this.#isOwnMove(event);
    const where = this.#whereNow();
    // A player not yet once in step with its group moves as it loads, not as its person says: an
    // iPhone joining late jumped to where it would start, and as it may seek, sent that jump to the
    // group, which waited for everyone again (the owner's test, 2026-10-12).
    const command =
      where !== undefined && !isOwn && this.#hasCaughtUp
        ? commandOfEvent(event, this.#media, where)
        : undefined;
    this.#takePersonsMove(event, command, where);
    // While its person's command waits to be sent, the player's loading is of a place the group
    // has not been told of, and would be taken for a stall: it is told once the command is sent.
    if (this.#pendingCommand === undefined) {
      this.#connection.reportState();
    }
  };

  /**
   * Takes its person's move, event, the group's command when it is one: sent when their rights let
   * them make it; else undone, a guest's leading player put back where its group is, as leading is
   * not hosting (D372 §4), or a follower put back by applying the group.
   */
  #takePersonsMove(
    event: string,
    command: GroupCommand | undefined,
    where: GroupNow | undefined,
  ): void {
    const canMake = command !== undefined && mayLead(command, this.#options.leads);
    if (canMake) {
      this.#lead(command);
    } else if (command !== undefined && where !== undefined && this.#isLeading()) {
      this.#leader.undo(where);
    } else {
      const held = { connection: this.#connection, media: this.#media, where };
      notePlayedWhileHeld({ ...held, event, isLeading: this.#isLeading() });
      this.apply();
    }
  }

  /**
   * Whether event is of a seek or pause the follower made itself, or its start gate did holding a
   * seek until enough is loaded, not its person: no command.
   */
  #isOwnMove(event: string): boolean {
    const isMarked = this.#ownMoves.heard(event, this.#options.monotonicMs());
    return isMarked || (event === "pause" && this.#startGate.isOwnPause());
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
    }, sendAfterMs(command));
  }

  /**
   * The browser paused the video itself, not its person: iOS as its full-screen player closes, a
   * pause sent for everyone each time a guest left full screen (the owner's test, 2026-10-11), and
   * as its page is hidden (2026-10-12). A pause waiting to be sent is dropped, one to come is the
   * player's own, and the video is put back where the group is.
   */
  readonly browserPaused = (): void => {
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
   * The page was hidden, where hiding pauses its film, or shown again: the group is told at once,
   * as a hidden page cannot lead and one shown again comes back (D372 §2, §5).
   */
  readonly visibilityChanged = (isHidden: boolean): void => {
    if (isHidden) {
      this.browserPaused();
    }
    this.#connection.reportState();
  };

  /**
   * Each checkDriftEveryMs: applies the group, and tells it this player's state while it waits, by
   * which the server ends a wait whose time is up (D247 §4), and while this is the host's player
   * following a guest's lead, by which the server sees it in step to take the lead back (D372 §3).
   */
  readonly tick = (): void => {
    for (const move of this.#ownMoves.expired(this.#options.monotonicMs())) {
      this.#connection.note({ kind: "ownMarkExpired", why: move });
    }
    this.#loadSpeed.sample(this.#media, this.#options.monotonicMs());
    this.apply();
    const snapshot = this.#connection.snapshot();
    if (snapshot.group?.state === "Waiting" || isHostAwaitingLead(snapshot)) {
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

  /** Its player began another quality: told the session with its state, for a cast's remote. */
  readonly qualitySwitched = (event: Event): void => {
    if (this.#playback.heard(event)) {
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
    this.#loading.stop();
    this.#stopStandingAside();
    this.#stopAsking();
  };

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
    if (command.command === "seek" && this.#isLeading()) {
      this.#leader.sentSeek(command.positionMs);
    }
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
    return {
      group,
      nowMs,
      expectedMs: expectedGroupPositionMs(snapshot, nowMs) ?? group.positionMs,
      tuning: snapshot.tuning,
    };
  }

  #seek(toMs: number, why: "catchUp" | "hold" | "skip"): void {
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
    this.#rateChangedAtMs = this.#options.monotonicMs();
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
   * Seeks to catch up with the group at toMs, ahead as catchUpTargetMs says, and says it fell behind
   * if it had been in step. It waits there for the group, the host's player too: a host on a slow
   * link stalls as a viewer does (the owner, 2026-10-06), and its pause to wait is no command.
   */
  #catchUp(toMs: number, where: GroupNow): void {
    if (this.#hasCaughtUp) {
      this.#media.dispatchEvent(new Event(fellBehindEvent));
    }
    const targetMs = catchUpTargetMs(this.#media, { toMs, rate: where.group.rate }, () =>
      this.#loadSpeed.catchUpLeadMs(this.#lastLoadMs, where.group.rate, where.tuning),
    );
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
    const ownSeek = { isLoading: this.#seekStartedAtMs !== undefined, toMs: this.#soughtToMs };
    const toMs = holdSeekMs(media, where, ownSeek);
    if (toMs !== undefined) {
      this.#seek(toMs, "hold");
    }
  }
}

/**
 * Makes media follow the group while it plays options.fileId (watch-together.md, Sync): paused at
 * the group's position while it is Paused, Waiting or Idle; playing while it is Playing, its drift
 * nudged away by the rate or sought across. It tells the group its state on every change, and
 * counts as buffering until it has once been where the group is, and after a stall or a seek until
 * it has refillAheadMs loaded. While its roster says this device leads the session, it moves media
 * by none of that: the leader plays as a device plays a film alone, carrying out once each change
 * another device made, and the group follows it (D371 §1, GroupLeader). A player's person's own
 * play, pause and seek become commands, as their rights allow; the follower's own do not. Returns
 * the stopping.
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
    ["webkitendfullscreen", follower.browserPaused] as const,
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
        noteQualitySwitched(connection)(event);
        follower.qualitySwitched(event);
      },
    ] as const,
    [playerMovedSoundEvent, noteSoundMoved(connection)] as const,
  ];
  for (const [event, listener] of listeners) {
    media.addEventListener(event, listener);
  }
  connection.setPlayer(follower.playerState);
  // A leader chooses its quality as a device alone does (D371 §1): only a follower keeps the film's
  // own picture for its group (D286 §2).
  const unmarkFollowing = markFollowingAGroup(() => isFollowerOf(connection.snapshot()));
  const unsubscribe = connection.subscribe(follower.connectionChanged);
  const checker = setInterval(follower.tick, checkDriftEveryMs);
  const stopHearingPage = whenVisibilityChanges(options.page, follower.visibilityChanged);
  follower.apply();
  connection.reportState();
  return () => {
    stopHearingPage();
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
