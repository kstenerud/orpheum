// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";

/** A lower quality a stalled player may move to, as a report's answer offers it (D253). */
export type LowerQuality = components["schemas"]["LowerQualityBody"];

/** How often a player told of a lower quality not ready yet asks again whether it is (D253 §1). */
export const askReadyEveryMs = 3_000;

/**
 * The share of a link's measured speed a quality may take: the server chooses the lower quality by
 * the same share (D247 §6, the core's linkShareForQuality).
 */
const linkShareForQuality = 0.7;

/** What a stalled player knows when its lower quality is ready (D253 §2). */
export interface KeepingUp {
  /** Whether it stalled again since the lower quality was offered. */
  readonly hasStalledSinceOffered: boolean;
  /** The link's speed as the player measured it, undefined when it cannot say. */
  readonly linkKbps: number | undefined;
  /** The film's own rate, as it plays now. */
  readonly filmKbps: number;
}

/**
 * Whether a player still cannot keep up with the film as it plays, and moves to its lower quality
 * (D253 §2): it stalled again, its link is not measured, or the film takes more of the link than a
 * quality may.
 */
export function stillCannotKeepUp(seen: KeepingUp): boolean {
  if (seen.hasStalledSinceOffered || seen.linkKbps === undefined) {
    return true;
  }
  return seen.filmKbps > linkShareForQuality * seen.linkKbps;
}

/** What MovingDown needs: the film's rate, the link's, how to ask again, and where a move goes. */
export interface MovingDownOptions {
  readonly filmKbps: number;
  readonly readLinkKbps: () => number | undefined;
  /** Sends a report now, whose answer says whether the offer is ready. */
  readonly askAgain: () => void;
  readonly onMove: (offer: LowerQuality) => void;
}

/**
 * Moves a stalled player to the lower quality its first stall started, once the reports' answers
 * say it is ready and the player still cannot keep up (D253): until it is ready, it asks again every
 * askReadyEveryMs; ready, a player keeping up stays, and its next stall moves it. It moves once.
 */
export class MovingDown {
  readonly #options: MovingDownOptions;
  #isOffered = false;
  #hasStalledSinceOffered = false;
  #hasMoved = false;
  #asking: ReturnType<typeof setInterval> | undefined;

  constructor(options: MovingDownOptions) {
    this.#options = options;
  }

  /** The player stalled. */
  stalled(): void {
    if (this.#isOffered) {
      this.#hasStalledSinceOffered = true;
    }
  }

  /** A report was answered, with the lower quality offered or without one. */
  answered(offer: LowerQuality | undefined): void {
    if (offer === undefined || this.#hasMoved) {
      return;
    }
    this.#isOffered = true;
    if (!offer.isReady) {
      this.#asking ??= setInterval(this.#options.askAgain, askReadyEveryMs);
      return;
    }
    this.#stopAsking();
    const seen = {
      hasStalledSinceOffered: this.#hasStalledSinceOffered,
      linkKbps: this.#options.readLinkKbps(),
      filmKbps: this.#options.filmKbps,
    };
    if (stillCannotKeepUp(seen)) {
      this.#hasMoved = true;
      this.#options.onMove(offer);
    }
  }

  /** Stops asking; nothing moves after. */
  stop(): void {
    this.#stopAsking();
    this.#hasMoved = true;
  }

  #stopAsking(): void {
    if (this.#asking !== undefined) {
      clearInterval(this.#asking);
      this.#asking = undefined;
    }
  }
}
