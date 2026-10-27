// SPDX-License-Identifier: MPL-2.0

import { linkShareForQuality } from "@orpheum/player";
import type { components } from "@orpheum/sdk";

/**
 * A place on a film's ladder: its chunks' master playlist and the quality to start at, where a
 * playback starts (D260) or a stalled player may move to (D253).
 */
export type LadderOffer = components["schemas"]["LadderBody"];

/**
 * Where a chunked player starts on an offer, at positionMs of the film: the quality the offer chose,
 * from there, choosing its own within the share of the link the offer says (D247 §6), at the
 * steadiness level the offer says its device last ended at on its network (D352 Q2).
 */
export function startOn(
  offer: Pick<LadderOffer, "startAtBps" | "linkSharePercent" | "steadiness">,
  positionMs: number,
): {
  readonly startAtBps: number;
  readonly linkSharePercent: number;
  readonly startPositionS: number;
  readonly startSteadiness: number;
} {
  return {
    startAtBps: offer.startAtBps,
    linkSharePercent: offer.linkSharePercent,
    startPositionS: positionMs / 1000,
    startSteadiness: offer.steadiness,
  };
}

/** How often a player told of a lower quality not ready yet asks again whether it is (D253 §1). */
export const askReadyEveryMs = 3_000;

/** What a stalled player knows when its lower quality is ready (D253 §2). */
export interface KeepingUp {
  /** Whether it stalled again since the lower quality was offered. */
  readonly hasStalledSinceOffered: boolean;
  /** The link's speed as the player measured it, undefined when it cannot say. */
  readonly linkKbps: number | undefined;
  /** The film's own picture's average rate, as the offer says it; 0 when not known. */
  readonly filmPictureKbps: number;
  /**
   * The share of a link's measured speed a quality may take, in percent, as the offer says, by which
   * the server chose the lower quality (link_share_for_quality_percent, D247 §6); the player's
   * starting share when 0.
   */
  readonly linkSharePercent: number;
}

/**
 * Whether a player still cannot keep up with the film as it plays, and moves to its lower quality
 * (D253 §2): it stalled again, its link or the film's rate is not known, or the film's picture
 * takes more of the link than a quality may.
 */
export function stillCannotKeepUp(seen: KeepingUp): boolean {
  if (seen.hasStalledSinceOffered || seen.linkKbps === undefined || seen.filmPictureKbps <= 0) {
    return true;
  }
  const share = seen.linkSharePercent > 0 ? seen.linkSharePercent / 100 : linkShareForQuality;
  return seen.filmPictureKbps > share * seen.linkKbps;
}

/** What MovingDown needs: the link's rate, how to ask again, and where a move goes. */
export interface MovingDownOptions {
  readonly readLinkKbps: () => number | undefined;
  /** Sends a report now, whose answer says whether the offer is ready. */
  readonly askAgain: () => void;
  readonly onMove: (offer: LadderOffer) => void;
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
  answered(offer: LadderOffer | undefined): void {
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
      filmPictureKbps: offer.filmPictureKbps,
      linkSharePercent: offer.linkSharePercent,
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
