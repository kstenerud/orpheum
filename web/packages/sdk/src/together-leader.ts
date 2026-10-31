// SPDX-License-Identifier: MPL-2.0

/**
 * A session's leader, the screen that is the film (D371 §1): it plays as a device plays a film alone,
 * starting when it is ready and buffering as it needs, never held, sought to the group's place, run
 * up or nudged, and the group's position is its own, which the server reads from its state. What
 * another device asks of the group, a cast's remote or a guest given the right, it carries out once,
 * as its own press (D371 §4). Kept apart from the follower (together-follow.ts), which leads by it
 * while its session's roster says this device leads. Every cast to a Fire TV of 2026-10-28 that went
 * wrong was a move of the group's made on its one screen: a nudge, a hold's seek back, a run-up's
 * jump, a start's play, pause and play (D371).
 */

import type { StartGate } from "./start-gate.ts";
import type { PlaybackSound } from "./playback-sound.ts";
import type { GroupView, PlaybackReport, SyncTuning } from "./together.ts";
import type { TogetherConnection } from "./together-connection.ts";
import type { GroupNow } from "./together-lead.ts";
import { haveFutureData, positionMsOf, type FollowedMedia } from "./together-media.ts";
import type { BufferingCause } from "./together-notes.ts";
import type { OwnMoves } from "./together-own-moves.ts";
import { qualitySwitchedOf, type QualitySwitched } from "./together-player-events.ts";

/** What a leader moves its player by, as its follower holds them. */
export interface LeaderParts {
  readonly media: FollowedMedia;
  readonly connection: Pick<TogetherConnection, "note">;
  /** The follower's marks on its own moves, so its carrying out is never taken for a person's. */
  readonly ownMoves: Pick<OwnMoves, "mark">;
  readonly monotonicMs: () => number;
  /**
   * Plays the player, muted if the browser will not play it with sound, once its start gate has
   * enough loaded ahead (start-gate.ts).
   */
  readonly play: () => void;
  /** Its player's start gate: whether a play waits on it, and giving that play up. */
  readonly startGate: Pick<StartGate, "isWaiting" | "cancel">;
}

/** A leading player: what of its group's changes it carries out, and whether it loads. */
export class GroupLeader {
  readonly #parts: LeaderParts;
  /** The change of its group it last carried out, or found as it began to lead; undefined before. */
  #carriedOutSeq: number | undefined = undefined;
  /** Whether it has found its group yet, as it began to lead. */
  #hasJoined = false;
  /**
   * Where each of its own seeks sent to its group went, oldest first, until its group's change for
   * it comes: a second seek sent before the change for the first leaves the first its own too.
   */
  #ownSeeksToMs: number[] = [];

  constructor(parts: LeaderParts) {
    this.#parts = parts;
  }

  /**
   * Leads where its group is, at the group's change seq: the first time, it takes the group's place
   * as it joins it, should it be far from it, and plays or pauses as the group does; then each change
   * once, carried out as its own press, a seek to where it was asked, a pause, a play. It moves for
   * nothing else, so its own stall, its own seek and its own start move the group, never it.
   */
  lead(where: GroupNow, seq: number | undefined): void {
    if (this.#hasJoined && seq === this.#carriedOutSeq) {
      return;
    }
    if (this.#hasJoined) {
      this.#seekAsAsked(where.group, where.tuning);
    } else {
      this.#joinAt(where);
    }
    this.#hasJoined = true;
    this.#carriedOutSeq = seq;
    this.#playOrPauseAs(where.group);
  }

  /**
   * Its own seek, sent to its group as a command: the group's change for it is its own, carried out
   * already, not another device's to seek to.
   */
  sentSeek(toMs: number): void {
    this.#ownSeeksToMs.push(toMs);
  }

  /**
   * Undoes its person's play, pause or seek their link withholds (D372 §4): back to where its group
   * is, playing or paused as the group is. A guest's leading player carries the film's clock, never
   * a right the host kept.
   */
  undo(where: GroupNow): void {
    if (
      Math.abs(where.expectedMs - positionMsOf(this.#parts.media)) > where.tuning.leaveDriftUpToMs
    ) {
      this.#seek(where.expectedMs, "undone");
    }
    this.#playOrPauseAs(where.group);
  }

  /**
   * Whether its player cannot play on now: loading, seeking, or waiting for enough loaded ahead to
   * start on, which holds its group where it is.
   */
  isLoading(): boolean {
    const { media, startGate } = this.#parts;
    return media.readyState < haveFutureData || media.seeking || startGate.isWaiting();
  }

  /** Why it cannot play on now, while it cannot (isLoading). */
  loadingCause(): BufferingCause {
    return this.#parts.media.seeking ? "seeking" : "notReady";
  }

  /**
   * Takes the group's place as it joins a group already under way, should it be beyond the seek
   * threshold from it, as a film starts from where it is resumed: a host's player opened again on a
   * film the group played on without it, from where it was left (D364 §15). A group standing for its
   * leader is this player's own film put on, or one it is to start, and is joined where it is.
   */
  #joinAt(where: GroupNow): void {
    const { state } = where.group;
    const isUnderWay = state === "Playing" || state === "Paused";
    if (
      isUnderWay &&
      Math.abs(where.expectedMs - positionMsOf(this.#parts.media)) > where.tuning.seekDriftBeyondMs
    ) {
      this.#seek(where.expectedMs, "join");
    }
  }

  /**
   * Seeks where another device's seek asked, as its own press, unless it is there already. Its own
   * seek's change is no other's: a Fire TV leading a cast sought back about 200 ms after each of its
   * own 10 s skips, its group's change for the skip coming as it had played on past where it went
   * (staging, 2026-10-28, 18:32:39 and 18:32:44). A cast's remote is its person's too, so the
   * change's member cannot tell them apart; where its own seeks went can, each of them: two skips in
   * a row, the second sent before the change for the first came, took the first's change for
   * another device's and jumped back to it. A change for one of its own seeks answers it and every
   * one sent before it, which the server has passed; another seek's change ends the wait for all.
   */
  #seekAsAsked(group: GroupView, tuning: SyncTuning): void {
    if (group.event !== "seeked") {
      return;
    }
    const ownAt = this.#ownSeeksToMs.findIndex((toMs) => Math.abs(group.positionMs - toMs) <= 1);
    this.#ownSeeksToMs = ownAt < 0 ? [] : this.#ownSeeksToMs.slice(ownAt + 1);
    const isFar =
      Math.abs(group.positionMs - positionMsOf(this.#parts.media)) > tuning.leaveDriftUpToMs;
    if (isFar && ownAt < 0) {
      this.#seek(group.positionMs, "command");
    }
  }

  /** Pauses as a paused group asks, and plays as a group to play asks, unless it does already. */
  #playOrPauseAs(group: GroupView): void {
    const { media, ownMoves, monotonicMs, play } = this.#parts;
    if (media.playbackRate !== group.rate) {
      media.playbackRate = group.rate;
    }
    if (group.state === "Paused") {
      this.#parts.startGate.cancel(); // a start waiting to play stays paused
    }
    if (group.state === "Paused" && !media.paused) {
      ownMoves.mark("pause", monotonicMs());
      media.pause();
    } else if (
      (group.state === "Playing" || group.state === "Waiting") &&
      media.paused &&
      !media.ended
    ) {
      play();
    }
  }

  #seek(toMs: number, why: "join" | "command" | "undone"): void {
    const { media, connection, ownMoves, monotonicMs } = this.#parts;
    connection.note({ kind: "sought", positionMs: positionMsOf(media), toMs, why });
    ownMoves.mark("seek", monotonicMs());
    media.currentTime = Math.max(0, toMs) / 1000;
  }
}

/**
 * A leader's catching up to its group after a stall the group played on through (D374): the group
 * waits for no one's stall while another device plays the film (D373 §1), so a leader that plays
 * again within its grace, still leading, is behind its group, and catches up to it as a follower
 * does, nudged or sought, before it leads again from there; the server's group moves on its own clock
 * meanwhile, never stepping back for the stall, so no one sees a stretch twice. A stall begins while
 * the leader says it buffers and its group plays, within one change of the group; it ends as it no
 * longer buffers, that change still the group's, and the catching up lasts while it is, until in step.
 * A change of the group (a seek, a pause, the group standing for its only player's stall, D374 §3)
 * ends either: the leader leads that change as it would. Its own command, awaiting its answer, is no
 * stall: its loading is its own seek's.
 */
export class LeaderCatchUp {
  /** The playing group's change in which the leader stalls, while it does. */
  #stall: { readonly seq: number | undefined } | undefined = undefined;
  /**
   * The playing group's change the leader catches up in, after its stall ended, and whether it has
   * followed its group out of step since.
   */
  #catchingUp: { readonly seq: number | undefined; hasFollowed: boolean } | undefined = undefined;

  /**
   * Takes the leader's state as it tells it, isBuffering, with its group as its connection knows it,
   * its change seq; isAwaitingGroup while a command of its own awaits its answer, which no stall is.
   */
  told(
    isBuffering: boolean,
    where: {
      readonly group: Pick<GroupView, "state"> | undefined;
      readonly seq: number | undefined;
    },
    isAwaitingGroup: boolean,
  ): void {
    if (where.group?.state !== "Playing" || isAwaitingGroup) {
      this.#stall = undefined;
      return;
    }
    if (isBuffering) {
      this.#stall = { seq: where.seq };
      return;
    }
    if (this.#stall !== undefined && this.#stall.seq === where.seq) {
      this.#catchingUp = { seq: where.seq, hasFollowed: false };
    }
    this.#stall = undefined;
  }

  /** Whether the leader catches up to its group now, the group at its change seq. */
  isCatchingUp(seq: number | undefined): boolean {
    return this.#catchingUp !== undefined && this.#catchingUp.seq === seq;
  }

  /** The leader, catching up, followed its group, out of step with it. */
  followed(): void {
    if (this.#catchingUp !== undefined) {
      this.#catchingUp.hasFollowed = true;
    }
  }

  /**
   * The leader is in step with its group: it leads from here. Whether it followed its group to get
   * here: a stall that ended in step said so in its own state, and needs no other.
   */
  caughtUp(): boolean {
    const hasFollowed = this.#catchingUp?.hasFollowed === true;
    this.#catchingUp = undefined;
    return hasFollowed;
  }
}

/**
 * Whether a leader catching up is in step with its group, expectedMs, and so leads again: playing,
 * ready, not seeking, within the drift a follower leaves alone (leaveDriftUpToMs, D250 §1), as the
 * server judges it in step by its state.
 */
export function isCaughtUp(media: FollowedMedia, expectedMs: number, tuning: SyncTuning): boolean {
  return (
    !media.paused &&
    !media.seeking &&
    media.readyState >= haveFutureData &&
    Math.abs(positionMsOf(media) - expectedMs) <= tuning.leaveDriftUpToMs
  );
}

/**
 * How a player plays its film, as it tells its session for a cast's remote to show (D371 §4): the
 * delivery it was given, and the quality it plays and the link it estimated as it chose it, from
 * its player's word of each quality it begins (playerSwitchedQualityEvent).
 */
export class PlaybackTold {
  readonly #delivery: PlaybackReport["delivery"] | undefined;
  readonly #readSound: (() => PlaybackSound | undefined) | undefined;
  #quality: QualitySwitched | undefined = undefined;

  /** readSound says how the film sounds now, for its remote to show and change; absent, nothing. */
  constructor(
    delivery: PlaybackReport["delivery"] | undefined,
    readSound?: () => PlaybackSound | undefined,
  ) {
    this.#delivery = delivery;
    this.#readSound = readSound;
  }

  /** Keeps the quality a playerSwitchedQualityEvent says its player began; whether it said one. */
  heard(event: Event): boolean {
    const switched = qualitySwitchedOf(event);
    this.#quality = switched ?? this.#quality;
    return switched !== undefined;
  }

  /** How the player plays its film, or undefined where it was given no delivery to say. */
  report(): PlaybackReport | undefined {
    const quality = this.#quality;
    const sound = this.#readSound?.();
    return this.#delivery === undefined
      ? undefined
      : {
          delivery: this.#delivery,
          ...(quality === undefined ? {} : { qualityKbps: quality.qualityKbps }),
          ...(quality?.linkEstimateKbps === undefined
            ? {}
            : { linkKbps: quality.linkEstimateKbps }),
          ...(sound === undefined ? {} : { sound }),
        };
  }
}
