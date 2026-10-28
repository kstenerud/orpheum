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
  /** Where its own last seek, sent to its group, went, until its group's change for it comes. */
  #ownSeekToMs: number | undefined = undefined;

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
    this.#ownSeekToMs = toMs;
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
   * change's member cannot tell them apart; where its own seek went can.
   */
  #seekAsAsked(group: GroupView, tuning: SyncTuning): void {
    const ownToMs = this.#ownSeekToMs;
    const isOwn =
      group.event === "seeked" &&
      ownToMs !== undefined &&
      Math.abs(group.positionMs - ownToMs) <= 1;
    if (group.event === "seeked") {
      this.#ownSeekToMs = undefined;
    }
    const isFar =
      Math.abs(group.positionMs - positionMsOf(this.#parts.media)) > tuning.leaveDriftUpToMs;
    if (group.event === "seeked" && isFar && !isOwn) {
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
 * How a player plays its film, as it tells its session for a cast's remote to show (D371 §4): the
 * delivery it was given, and the quality it plays and the link it estimated as it chose it, from
 * its player's word of each quality it begins (playerSwitchedQualityEvent).
 */
export class PlaybackTold {
  readonly #delivery: PlaybackReport["delivery"] | undefined;
  #quality: QualitySwitched | undefined = undefined;

  constructor(delivery: PlaybackReport["delivery"] | undefined) {
    this.#delivery = delivery;
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
    return this.#delivery === undefined
      ? undefined
      : {
          delivery: this.#delivery,
          ...(quality === undefined ? {} : { qualityKbps: quality.qualityKbps }),
          ...(quality?.linkEstimateKbps === undefined
            ? {}
            : { linkKbps: quality.linkEstimateKbps }),
        };
  }
}
