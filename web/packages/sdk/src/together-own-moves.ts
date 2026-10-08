// SPDX-License-Identifier: MPL-2.0

/**
 * A following player's marks on the moves it makes itself, so their events are not taken for its
 * person's: a seek it made, or that hls.js made over a gap, and a pause it made to hold or wait.
 */

/**
 * How long a mark waits for its event: the seeking or pause a move fires follows it within
 * milliseconds, and a mark left waiting would take a person's later move for the follower's own
 * (D268 §4, until the M5 controls send a person's moves as commands).
 */
export const ownMarkLastsMs = 2000;

/** Which of its own moves a mark waits for. */
export type OwnMove = "seek" | "pause";

/** The marks of one player's own moves, on its monotonic clock. */
export class OwnMoves {
  /** Until when each mark waits for its event; undefined when none waits. */
  #seekUntilMs: number | undefined = undefined;
  #pauseUntilMs: number | undefined = undefined;
  /** Whether the seek under way is its own: its seeking was, so its seeked is. */
  #isOwnSeekUnderWay = false;

  /** Marks the move about to be made, at nowMs, as its own. */
  mark(move: OwnMove, nowMs: number): void {
    if (move === "seek") {
      this.#seekUntilMs = nowMs + ownMarkLastsMs;
    } else {
      this.#pauseUntilMs = nowMs + ownMarkLastsMs;
    }
  }

  /** Whether event, heard at nowMs, is of its own move, without using a mark up. */
  isOwn(event: string, nowMs: number): boolean {
    if (event === "seeked") {
      return this.#isOwnSeekUnderWay || isWaiting(this.#seekUntilMs, nowMs);
    }
    if (event === "seeking") {
      return isWaiting(this.#seekUntilMs, nowMs);
    }
    return event === "pause" && isWaiting(this.#pauseUntilMs, nowMs);
  }

  /** Hears event at nowMs: whether it is of its own move, the mark it answers used up. */
  heard(event: string, nowMs: number): boolean {
    const isOwn = this.isOwn(event, nowMs);
    if (event === "seeking") {
      this.#isOwnSeekUnderWay = isOwn;
      this.#seekUntilMs = undefined;
    } else if (event === "seeked") {
      // A seek that ends answers its mark too, its seeking heard or not: a mark left waiting would
      // take its person's next seek for the follower's own.
      this.#isOwnSeekUnderWay = false;
      this.#seekUntilMs = undefined;
    } else if (event === "pause") {
      this.#pauseUntilMs = undefined;
    }
    return isOwn;
  }

  /** The marks whose event never came by nowMs, given up: each is a move that fired nothing. */
  expired(nowMs: number): OwnMove[] {
    const moves: OwnMove[] = [];
    if (this.#seekUntilMs !== undefined && !isWaiting(this.#seekUntilMs, nowMs)) {
      this.#seekUntilMs = undefined;
      moves.push("seek");
    }
    if (this.#pauseUntilMs !== undefined && !isWaiting(this.#pauseUntilMs, nowMs)) {
      this.#pauseUntilMs = undefined;
      moves.push("pause");
    }
    return moves;
  }
}

/** Whether a mark waiting until untilMs still waits at nowMs. */
function isWaiting(untilMs: number | undefined, nowMs: number): boolean {
  return untilMs !== undefined && nowMs <= untilMs;
}
