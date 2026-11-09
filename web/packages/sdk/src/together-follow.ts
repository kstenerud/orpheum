// SPDX-License-Identifier: MPL-2.0

import {
  expectedGroupPositionMs,
  type GroupCommand,
  type PlayerState,
  type TogetherConnection,
} from "./together-connection.ts";
import {
  isHostAwaitingLead,
  isAskedByItsGroup,
  isFollowerOf,
  LeaderAwaited,
  isLeaderOf,
} from "./together-roster.ts";
import { GroupJumpsTold, type ChunkLoaded } from "./together-player-events.ts";
import { GroupLeader, isCaughtUp, LeaderCatchUp, PlaybackTold } from "./together-leader.ts";
import { commandOfEvent, mayLead, sendAfterMs, type GroupNow } from "./together-lead.ts";
import { FollowedChanges, heldByOf, notePlayedWhileHeld } from "./together-followed.ts";
import { OwnMoves } from "./together-own-moves.ts";
import { DriftCorrector } from "./together-follow-correction.ts";
import { haveFutureData, isInStep, positionMsOf, type FollowedMedia } from "./together-media.ts";
import { LoadSpeed } from "./together-load-speed.ts";
import { GroupLoading } from "./together-loading.ts";
import { Refill } from "./together-refill.ts";
import { ShownAgain } from "./together-sound.ts";
import { startGateOf, type StartGate } from "./start-gate.ts";
import { bufferingCauseOf, bufferingNote, HoldingNotes } from "./together-buffering.ts";
import { RecentEvents, type FollowOptions } from "./together-follow-options.ts";

export { browserPauseWindowMs, scrubPauseWindowMs } from "./together-lead.ts";
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
 * is owned by its DriftCorrector (together-follow-correction.ts). The rest of its state is facts
 * that hold beside any phase: whether it has once been in step, is refilling, waits for the group's
 * answer to its command, and has a seek loading.
 */
export class GroupFollower {
  readonly #media: FollowedMedia;
  readonly #connection: TogetherConnection;
  readonly #options: FollowOptions;
  /** The moves it makes itself, marked so their events are not taken for its person's. */
  readonly #ownMoves = new OwnMoves();
  /** When it moved for each change of its group, against when the change reached it. */
  readonly #followed: FollowedChanges;
  /** The group's jumps it told its player of. */
  readonly #jumps = new GroupJumpsTold();
  /** What it tells its player to load while its group waits or another is behind (D283 §2, D285 §2). */
  readonly #loading: GroupLoading;
  /** The player's last events, as "<event>[(own)]@<monotonic ms>", for a command's note. */
  readonly #recentEvents = new RecentEvents();
  /** Whether it has once been where the group is: until then it holds no one up (D246 §6). */
  #hasCaughtUp = false;
  /** Whether it played the film with its group on this page: a position of its own (D377 §3, §4). */
  #hasPlayedAlong = false;
  /** Its wait, on each new connection, to hear of its group's leader (D377 §3). */
  readonly #leaderAwaited = new LeaderAwaited();
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
  /** How fast it loads, measured while it loads (D250 §4). */
  readonly #loadSpeed = new LoadSpeed();
  /** Corrects its drift from a playing group: nudged away, sought across, or held at its position. */
  readonly #corrector: DriftCorrector;
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
  /** Whether its page was shown again and it has not played since, a follower. */
  readonly #shownAgain = new ShownAgain();
  /** Whether, leading, it catches up to its group after a stall the group played on through (D374). */
  readonly #leaderCatchUp = new LeaderCatchUp();

  constructor(media: FollowedMedia, connection: TogetherConnection, options: FollowOptions) {
    this.#media = media;
    this.#connection = connection;
    this.#options = options;
    this.#loading = new GroupLoading(media, connection);
    this.#followed = new FollowedChanges(connection.snapshot().groupSeq);
    this.#corrector = new DriftCorrector({
      media,
      connection,
      monotonicMs: options.monotonicMs,
      ownMoves: this.#ownMoves,
      refill: this.#refill,
      loadSpeed: this.#loadSpeed,
      shownAgain: this.#shownAgain,
    });
    const startGate = startGateOf(media);
    this.#startGate = startGate;
    // Aside only once its welcome says it follows, or while it catches up as a follower would:
    // until then a play its page asks waits, as a leader's does, and a follower's is given up at its
    // welcome, its follower playing it.
    this.#stopStandingAside = startGate.standAsideWhile(
      () => isFollowerOf(this.#connection.snapshot()) || this.#isCatchingUp(),
    );
    this.#stopAsking = startGate.asksToPlayWhile(() => isAskedByItsGroup(connection.snapshot()));
    this.#leader = new GroupLeader({
      media,
      connection,
      ownMoves: this.#ownMoves,
      monotonicMs: options.monotonicMs,
      play: () => {
        startGate.play(() => {
          this.#corrector.startPlaying();
        });
      },
      startGate,
    });
    this.#playback = new PlaybackTold(options.delivery, options.readSound, options.readPicture);
  }

  /** Whether this device leads its session, as its roster says (D371 §1). */
  #isLeading(): boolean {
    return isLeaderOf(this.#connection.snapshot());
  }

  /** Whether it leads, and catches up to its group after a stall the group played on through (D374). */
  #isCatchingUp(): boolean {
    return (
      this.#isLeading() && this.#leaderCatchUp.isCatchingUp(this.#connection.snapshot().groupSeq)
    );
  }

  /**
   * The player's state as the group is told it, with the film it plays, by which it contends for its
   * session's lead (D374 §1): a leader buffers only while it cannot play on, and its stall, told while
   * its group plays, is one it catches up after (LeaderCatchUp).
   */
  readonly playerState = (): PlayerState => {
    const isLeading = this.#isLeading();
    const isBuffering = isLeading ? this.#leader.isLoading() : this.#isBuffering();
    if (isLeading) {
      const { group, groupSeq } = this.#connection.snapshot();
      this.#leaderCatchUp.told(isBuffering, { group, seq: groupSeq }, this.#isAwaitingGroup);
    }
    this.#noteBuffering(isBuffering, isLeading);
    this.#wasReportedBuffering = isBuffering;
    const remembered = this.#placeRemembered(isBuffering, isLeading);
    const loadsFaster = this.#refill.loadsFaster();
    const playback = this.#playback.report();
    return {
      fileId: this.#options.fileId,
      positionMs: positionMsOf(this.#media),
      isPlaying: !this.#media.paused,
      isBuffering,
      readyOnMs: this.#refill.readyOnMs(this.#media),
      ...(loadsFaster === undefined ? {} : { loadsFaster }),
      ...(playback === undefined ? {} : { playback }),
      ...(this.#options.page?.visibilityState === "hidden" ? { isPageHidden: true } : {}),
      ...remembered,
    };
  };

  /**
   * Notes that it played along, ready and playing as its group plays, or as its leader, and says,
   * for its state, whether it ever has: a position of its own to present (D377 §4).
   */
  #placeRemembered(
    isBuffering: boolean,
    isLeading: boolean,
  ): { readonly isPlaceRemembered?: true } {
    const isPlaying = !this.#media.paused && !isBuffering;
    this.#hasPlayedAlong ||=
      isPlaying && (isLeading || this.#connection.snapshot().group?.state === "playing");
    return this.#hasPlayedAlong ? { isPlaceRemembered: true } : {};
  }

  /** Notes what holds it, or its group, up, when HoldingNotes says to (bufferingNote). */
  #noteBuffering(isBuffering: boolean, isLeading: boolean): void {
    const cause = isLeading
      ? this.#leader.loadingCause()
      : bufferingCauseOf(this.#media, this.#hasCaughtUp, this.#refill);
    const why = isBuffering ? cause : undefined;
    if (this.#holding.isToNote(why, this.#connection.snapshot()) && why !== undefined) {
      const changedAtMs = this.#corrector.rateChangedAtMs();
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
   * A leader is put nowhere: it carries out what another device asked, and loads as it would alone;
   * one catching up after a stall its group played on through follows as a follower does until in
   * step (D374).
   */
  readonly apply = (): void => {
    const where = this.#whereNow();
    const heldBy = heldByOf(this.#isAwaitingGroup, this.#options.page, this.#media);
    const nowMs = this.#options.monotonicMs();
    if (where === undefined || !this.#followed.mayFollow(heldBy, nowMs, this.#connection)) {
      return;
    }
    if (this.#leaderAwaited.holds(this.#connection.snapshot(), this.#hasPlayedAlong)) {
      return; // it keeps where it was until it hears of its leader (D377 §3)
    }
    if (this.#isLeading() && !this.#catchesUp(where)) {
      this.#hasCaughtUp = true; // its person's moves are the group's from the first
      this.#loading.stop();
      this.#leader.lead(where, this.#connection.snapshot().groupSeq);
      this.#jumps.tell(this.#media, where.group, true);
      return;
    }
    if (where.group.state === "playing") {
      this.#followPlaying(where);
    } else {
      this.#corrector.holdAt(where);
    }
    this.#hasCaughtUp ||= isInStep(this.#media, where.expectedMs, where.tuning);
    this.#loading.follow(where.group.state, !this.#isBuffering());
    this.#jumps.tell(this.#media, where.group);
  };

  /**
   * Whether this leader still catches up to its group, where, after a stall the group played on
   * through (D374), following it as a follower does, nudged or sought: once in step it leads from
   * there, at the group's rate, and tells its state, by which the server has it place the group again.
   */
  #catchesUp(where: GroupNow): boolean {
    if (!this.#isCatchingUp()) {
      return false;
    }
    if (!isCaughtUp(this.#media, where.expectedMs, where.tuning)) {
      this.#leaderCatchUp.followed();
      return true;
    }
    if (this.#leaderCatchUp.caughtUp()) {
      this.#corrector.setRate(
        where.group.rate,
        where,
        where.expectedMs - positionMsOf(this.#media),
      );
      this.#connection.reportState();
    }
    return false;
  }

  /**
   * Follows a playing group: waiting ahead of it, or playing along at once, as the group plays from
   * where its leader plays, and catching up to it, nudged or sought (D371 §2): no group names a
   * moment to start at, and none is run up to.
   */
  #followPlaying(where: GroupNow): void {
    if (this.#corrector.isWaitingAhead(where)) {
      // A player that jumped ahead to catch up waits there, paused and loading, for the group.
      if (!this.#media.paused) {
        this.#ownMoves.mark("pause", this.#options.monotonicMs());
        this.#media.pause();
      }
    } else {
      this.#corrector.play(where, this.#hasCaughtUp);
    }
  }

  /** One of the player's own events: a person's command when it leads, else a reason to apply. */
  readonly heard = (event: string): void => {
    this.#keepEvent(event);
    if (event === "playing" || event === "seeked") {
      this.#corrector.settle();
    }
    this.#shownAgain.heard(event);
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
   * The page was hidden, where hiding pauses its film, or shown again, and then put where its group
   * is, not paused till its next check (VQ105): the group is told at once (D372 §2, §5).
   */
  readonly visibilityChanged = (isHidden: boolean): void => {
    this.#shownAgain.set(!isHidden && !this.#isLeading());
    if (isHidden) {
      this.browserPaused();
    } else {
      this.apply();
    }
    this.#connection.reportState();
  };

  /**
   * Each checkDriftEveryMs: applies the group, and tells it this player's state while it waits (D247
   * §4), while this is the host's player following a guest's lead, to be seen in step (D372 §3),
   * while it leads stalled, to have its lead handed on past the grace (D373 §2), and while it leads
   * catching up, to be seen in step (D374).
   */
  readonly tick = (): void => {
    for (const move of this.#ownMoves.expired(this.#options.monotonicMs())) {
      this.#connection.note({ kind: "ownMarkExpired", why: move });
    }
    this.#loadSpeed.sample(this.#media, this.#options.monotonicMs());
    this.apply();
    const snapshot = this.#connection.snapshot();
    const isStalledLeader = this.#wasReportedBuffering === true && this.#isLeading();
    const isToTell = isHostAwaitingLead(snapshot) || isStalledLeader || this.#isCatchingUp();
    if (snapshot.group?.state === "waiting" || isToTell) {
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
    this.#loadSpeed.chunkLoaded(loaded);
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
    this.#corrector.notedProgress();
    this.#refill.heard(event, this.#media);
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
        recentEvents: this.#recentEvents.text(),
      });
    }
    this.#connection.reportState();
  }

  /** Keeps event, its time and whether the follower made it, among its recent events. */
  #keepEvent(event: string): void {
    const nowMs = this.#options.monotonicMs();
    this.#recentEvents.keep(event, this.#ownMoves.isOwn(event, nowMs), nowMs);
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
}
