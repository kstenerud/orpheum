// SPDX-License-Identifier: MPL-2.0

/**
 * A following player's marks on the moves it makes itself, so their events are not taken for its
 * person's: a seek it made, or that hls.js made over a gap, and a pause it made to hold or wait; and,
 * while its film plays on a wireless receiver, the pauses and plays its element fires in answer to
 * them (D522 §2).
 */

/**
 * How long a mark waits for its event: the seeking or pause a move fires follows it within
 * milliseconds, and a mark left waiting would take a person's later move for the follower's own
 * (D268 §4, until the M5 controls send a person's moves as commands).
 */
export const ownMarkLastsMs = 2000;

/**
 * How long after the follower's own move a film on a wireless receiver, AirPlay's, fires the pauses
 * and plays that answer it: Safari tells the page the receiver's play state late, and after a seek,
 * a pause or a play the follower made, the element fired pause and play again within a quarter of a
 * second, each pause sent to the group as the guest's, which paused everyone (VQ364, staging,
 * 2026-11-18). Short of a person's next press: the guest's play came 1.5 s after the follower's seek.
 */
export const receiverAnswersWithinMs = 1000;

/** Which of its own moves a mark waits for. */
export type OwnMove = "seek" | "pause" | "play";

/** The marks of one player's own moves, on its monotonic clock. */
export class OwnMoves {
  /** Until when each mark waits for its event; undefined when none waits. */
  #seekUntilMs: number | undefined = undefined;
  #pauseUntilMs: number | undefined = undefined;
  #playUntilMs: number | undefined = undefined;
  /** Whether the seek under way is its own: its seeking was, so its seeked is. */
  #isOwnSeekUnderWay = false;
  /** Until when a pause or play its element fires answers its own move on a receiver; else undefined. */
  #receiverAnswersUntilMs: number | undefined = undefined;
  /** Whether its film plays on a wireless receiver now (FollowOptions' isOnWirelessTarget). */
  readonly #isOnReceiver: () => boolean;

  constructor(isOnReceiver: () => boolean = () => false) {
    this.#isOnReceiver = isOnReceiver;
  }

  /** Marks the move about to be made, at nowMs, as its own. */
  mark(move: OwnMove, nowMs: number): void {
    if (move === "seek") {
      this.#seekUntilMs = nowMs + ownMarkLastsMs;
    } else if (move === "play") {
      this.#playUntilMs = nowMs + ownMarkLastsMs;
    } else {
      this.#pauseUntilMs = nowMs + ownMarkLastsMs;
    }
    this.#awaitReceiversAnswer(nowMs);
  }

  /**
   * Notes the follower's own play, at nowMs: no mark, as a play while its group plays is no command,
   * but a receiver's answer to it is awaited (receiverAnswersWithinMs).
   */
  played(nowMs: number): void {
    this.#awaitReceiversAnswer(nowMs);
  }

  /** Takes, where its film plays on a receiver, the pauses and plays of the next while for its own. */
  #awaitReceiversAnswer(nowMs: number): void {
    if (this.#isOnReceiver()) {
      this.#receiverAnswersUntilMs = nowMs + receiverAnswersWithinMs;
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
    if ((event === "pause" || event === "play") && isWaiting(this.#receiverAnswersUntilMs, nowMs)) {
      return true; // the receiver's answer to its own move, never its person's press (D522 §2)
    }
    if (event === "play") {
      return isWaiting(this.#playUntilMs, nowMs);
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
      // take its person's next seek for the follower's own. A receiver answers its own seek's end.
      if (isOwn) {
        this.#awaitReceiversAnswer(nowMs);
      }
      this.#isOwnSeekUnderWay = false;
      this.#seekUntilMs = undefined;
    } else if (event === "pause") {
      this.#pauseUntilMs = undefined;
    } else if (event === "play") {
      this.#playUntilMs = undefined;
    }
    return isOwn;
  }

  /** The marks whose event never came by nowMs, given up: each is a move that fired nothing. */
  expired(nowMs: number): Exclude<OwnMove, "play">[] {
    const moves: Exclude<OwnMove, "play">[] = [];
    if (this.#seekUntilMs !== undefined && !isWaiting(this.#seekUntilMs, nowMs)) {
      this.#seekUntilMs = undefined;
      moves.push("seek");
    }
    if (this.#pauseUntilMs !== undefined && !isWaiting(this.#pauseUntilMs, nowMs)) {
      this.#pauseUntilMs = undefined;
      moves.push("pause");
    }
    if (this.#playUntilMs !== undefined && !isWaiting(this.#playUntilMs, nowMs)) {
      this.#playUntilMs = undefined; // a play that fired no event leaves nothing to answer
    }
    return moves;
  }
}

/** Whether a mark waiting until untilMs still waits at nowMs. */
function isWaiting(untilMs: number | undefined, nowMs: number): boolean {
  return untilMs !== undefined && nowMs <= untilMs;
}
