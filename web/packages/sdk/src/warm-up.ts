// SPDX-License-Identifier: MPL-2.0

/**
 * A playback's decision asked before its film plays, so the server starts its chunks (D258 §1), and
 * the one way a player takes it or lets it go: the web app's Watching (its film page's warm-up and
 * its next episode's) and the Google Cast receiver's next episode alike (the owner, 2026-11-27: a
 * Chromecast's next episode started cold, 10 to 20 s, where a browser's was warmed as Up next showed).
 */

/**
 * How long a warmed decision may wait for its film to play before a new one is asked for: half the
 * least mediaTokenIdleExpiryHours can be, an hour, so its token is never idled out by the time it
 * plays.
 */
export const warmUpKeptMs: number = 30 * 60_000;

/** Why a warm-up is not played on, its film asked for again: the cause each such ask names. */
export type WarmUpAskedAgain = "warmUpStale" | "warmUpOffLadder" | "warmUpFailed";

/** What a warmed decision needs to be: its playback's token, and its offer on the film's ladder. */
export interface WarmedDecision {
  readonly mediaToken: string;
  readonly onLadder?: unknown;
}

/** A decision asked ahead: its answer to come, when it was asked, and the title it was asked as. */
export interface WarmUp<D extends WarmedDecision> {
  readonly decision: Promise<D>;
  readonly askedMs: number;
  /** The title the file plays as (B0 question 20); undefined for the one it is filed as first. */
  readonly itemId: number | undefined;
}

/**
 * What a player plays on, given warm, the warm-up of the film it plays now: warm's decision where it
 * is fresh and on the film's ladder, which the warm-up may have been asked before the film was cut
 * onto (D260 §1); else a new one, asked again saying why (askAgain). A warm-up older than
 * warmUpKeptMs is stale (stale is told, then it is asked again); one whose decision is off the ladder
 * is let go of (offLadder, then asked again); one that failed is asked again, as its page left it to
 * be (the M4 closing audit, 2026-10-08). played is told the decision played on, and gives what plays.
 */
export function playOnWarmUp<D extends WarmedDecision>(
  warm: WarmUp<D>,
  ways: {
    readonly nowMs: number;
    readonly askAgain: (cause: WarmUpAskedAgain) => Promise<D>;
    readonly stale: () => void;
    readonly offLadder: (decision: D) => void;
    readonly played: (decision: D) => D;
  },
): Promise<D> {
  if (ways.nowMs - warm.askedMs >= warmUpKeptMs) {
    ways.stale();
    return ways.askAgain("warmUpStale");
  }
  return warm.decision.then(
    (decision) => {
      if (decision.onLadder === undefined) {
        ways.offLadder(decision);
        return ways.askAgain("warmUpOffLadder");
      }
      return ways.played(decision);
    },
    () => ways.askAgain("warmUpFailed"),
  );
}

/**
 * The next episode's warm-up, asked as Up next shows, beside the film playing, whose playbacks it
 * leaves alone: the film playing keeps its runs to its end (the owner's request, 2026-11-11). At most
 * one is kept: a different next replaces it, its playback let go of; the next played takes it, its
 * playback the taker's from then, decided or still to be; anything else played lets go of it.
 */
export class NextWarmUp<D extends WarmedDecision> {
  private next: Kept<D> | undefined;

  constructor(
    private readonly ways: {
      /** Lets go of a playback no one will play on (DELETE /playback/{token}). */
      readonly leave: (mediaToken: string) => void;
      readonly nowMs: () => number;
    },
  ) {}

  /** Whether fileId's warm-up is kept, asked or decided. */
  isWarming(fileId: number): boolean {
    return this.next?.fileId === fileId;
  }

  /**
   * Warms fileId up, as the title itemId, asking its decision by ask, unless it is warmed already;
   * another next kept is let go of.
   */
  warm(fileId: number, itemId: number | undefined, ask: () => Promise<D>): void {
    if (this.isWarming(fileId)) {
      return;
    }
    this.letGo();
    const decision = ask();
    const next: Kept<D> = {
      fileId,
      warm: { decision, askedMs: this.ways.nowMs(), itemId },
      mediaToken: undefined,
      isTaken: false,
    };
    this.next = next;
    decision.then(
      (decided) => {
        if (this.next === next) {
          next.mediaToken = decided.mediaToken;
        } else if (!next.isTaken) {
          this.ways.leave(decided.mediaToken); // let go of while it was asked for
        }
      },
      // A warm-up that fails is asked again as its film plays (playOnWarmUp); nothing waits on it.
      () => undefined,
    );
  }

  /**
   * fileId plays now: its warm-up where it is the next kept, its playback the taker's from here; none
   * otherwise, the next kept let go of.
   */
  take(fileId: number): WarmUp<D> | undefined {
    const next = this.next;
    if (next?.fileId !== fileId) {
      this.letGo();
      return undefined;
    }
    this.next = undefined;
    next.isTaken = true;
    return next.warm;
  }

  /** Lets go of the next kept: its playback now where it is decided, else once it is. */
  letGo(): void {
    const next = this.next;
    this.next = undefined;
    if (next?.mediaToken !== undefined) {
      this.ways.leave(next.mediaToken);
    }
  }
}

/** The next kept: its file, its warm-up, its playback's token once decided, and whether it was taken. */
interface Kept<D extends WarmedDecision> {
  readonly fileId: number;
  readonly warm: WarmUp<D>;
  mediaToken: string | undefined;
  isTaken: boolean;
}
