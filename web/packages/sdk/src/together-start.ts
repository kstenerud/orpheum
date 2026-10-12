// SPDX-License-Identifier: MPL-2.0

/**
 * How late a player starts, learned at each start and kept by the device, and the run-up a player
 * that has not learned it plays through (D282). A phone's picture moves some 170 ms after it is
 * told to play, and Safari's clock stands still a moment after: every player started at the
 * moment the server named, and the phones then spent seconds nudged into step (the owner's test,
 * 2026-10-12). A player that has learned its lag presses play that much early; one that has not
 * plays from before where the group starts, making its lag up on the way.
 */

import { learnedLossMs, type LearnedLoss } from "./together-drift.ts";

/**
 * Where a device keeps how late its player starts, across sessions: one show a session is the most
 * common, and a lag learned only for the session would rarely be used (the owner, 2026-10-12).
 */
export interface StartLagMemory {
  readonly read: () => LearnedLoss | undefined;
  readonly keep: (learned: LearnedLoss) => void;
}

/** A memory kept for as long as the follower: what a device with nowhere to keep it learns. */
export function memoryOfTheSession(): StartLagMemory {
  let kept: LearnedLoss | undefined;
  return {
    read: () => kept,
    keep: (learned) => {
      kept = learned;
    },
  };
}

/**
 * Where a playing group is at nowMs on its way to where it starts, at startsAtMs: before then, as
 * far short of its position as it is early, so a player running up is judged by where it would be
 * had it run all the way; from then, moving on at its rate.
 */
export function runningPositionMs(
  group: { readonly positionMs: number; readonly rate: number },
  startsAtMs: number,
  nowMs: number,
): number {
  return group.positionMs + (nowMs - startsAtMs) * group.rate;
}

/** Learns how late the player starts from each start it presses play early for (D282). */
export class StartLearner {
  readonly #memory: StartLagMemory;
  /** How far behind where the group runs the player was as it pressed play; undefined between starts. */
  #behindAtPlayMs: number | undefined = undefined;
  /** Whether its player said it plays since it pressed play: only then does its position move. */
  #hasMoved = false;

  constructor(memory: StartLagMemory) {
    this.#memory = memory;
  }

  /** How late the player has learned it starts, in ms; undefined until it has started once. */
  readonly lagMs = (): number | undefined => {
    const learned = this.#memory.read();
    return learned === undefined || learned.moves === 0 ? undefined : Math.round(learned.costMs);
  };

  /** It pressed play behindMs behind where the group runs: ahead, by its lag, when early. */
  readonly pressedPlay = (behindMs: number): void => {
    this.#behindAtPlayMs = behindMs;
    this.#hasMoved = false;
  };

  /** Its player said it plays, its "playing" event: its position read from now on has moved. */
  readonly movedAfterPress = (): void => {
    this.#hasMoved = this.#behindAtPlayMs !== undefined;
  };

  /**
   * Its position, once settled after a start, was behindMs behind the group: what it lost from
   * pressing play to moving is how late it starts, learned and kept.
   */
  readonly settled = (behindMs: number): void => {
    if (this.#behindAtPlayMs === undefined || !this.#hasMoved) {
      return;
    }
    const learned = this.#memory.read() ?? { costMs: 0, moves: 0 };
    this.#memory.keep(learnedLossMs(learned, this.#behindAtPlayMs, behindMs));
    this.#behindAtPlayMs = undefined;
  };
}
