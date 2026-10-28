// SPDX-License-Identifier: MPL-2.0

/**
 * When a following player is ready to play on after it stalled or sought: refillAheadMs of film
 * loaded after a stall, and after a seek its group or its person made, enough to outlast loading its
 * next chunk with a margin (readyAheadOf, the rule every player starts by; the owner, 2026-10-28),
 * refillAheadMs where its player cannot say how long that takes. Every other player started on a
 * second or two after a seek (Media3 1 s, Jellyfin SyncPlay its first frame); ten seconds kept a
 * jump's group waiting about ten (seek-resync-2026-10-12.md), and a fixed 3 s once the link loaded
 * well faster than real time replaced it until a Fire TV's chunk of 10.4 s took 2.3 s to come.
 */

import { readyAheadOf } from "./ready-ahead.ts";
import type { ChunkLoaded } from "./together-player-events.ts";
import {
  hasRefilled,
  haveFutureData,
  refillAheadMs,
  type FollowedMedia,
} from "./together-media.ts";

/** Why the player is loading before it can play on: a stall or a catch-up's seek, or a seek. */
type RefillCause = "stall" | "seek";

/** What began a refill: a seek, the video waiting for data, or its readyState too low to play on. */
export type RefillStart = "seeking" | "waiting" | "notReady";

/** What event, heard with media as it is, begins a refill with. */
function startedByOf(event: string, media: FollowedMedia): RefillStart {
  if (event === "seeking" || media.seeking) {
    return "seeking";
  }
  return event === "waiting" ? "waiting" : "notReady";
}

/** Whether a player refills, why, and how fast its last chunk of picture loaded. */
export class Refill {
  #isRefilling = false;
  #cause: RefillCause = "stall";
  /** Whether the next seeking is the player's catch-up, a stall in all but name (D247 §2). */
  #isNextSeekCatchUp = false;
  /** Film loaded per wall time of its last chunk of picture; undefined before one loaded. */
  #loadsFaster: number | undefined;

  /** What began the refill under way, or the last one. */
  #startedBy: RefillStart = "waiting";

  /** What began the refill under way, or the last one: a seek, a wait for data, or too little to play. */
  get startedBy(): RefillStart {
    return this.#startedBy;
  }

  /** Whether it is refilling. */
  get isRefilling(): boolean {
    return this.#isRefilling;
  }

  /** It seeks itself, why as its note says: a catch-up's seek refills as a stall does. */
  readonly sought = (why: string): void => {
    this.#isNextSeekCatchUp = why === "catchUp";
  };

  /** A chunk loaded: a picture's says how fast the link carries the film, for the log. */
  readonly chunkLoaded = (loaded: ChunkLoaded): void => {
    if (loaded.track === "video" && loaded.loadMs > 0 && loaded.filmMs > 0) {
      this.#loadsFaster = loaded.filmMs / loaded.loadMs;
    }
  };

  /** One of the player's events: a stall or a seek starts refilling, enough loaded ends it. */
  readonly heard = (event: string, media: FollowedMedia): void => {
    if (event === "seeking") {
      this.#cause = this.#isNextSeekCatchUp ? "stall" : "seek";
      this.#isNextSeekCatchUp = false;
    } else if (event === "waiting" && !media.seeking) {
      this.#cause = "stall";
    }
    if (event === "waiting" || event === "seeking" || media.readyState < haveFutureData) {
      this.#startRefilling(event, media);
    } else if (this.#isRefilling && hasRefilled(media, this.#readyAheadMs(media))) {
      this.#isRefilling = false;
    }
  };

  /** Refills, by event as media is, unless it already does. */
  #startRefilling(event: string, media: FollowedMedia): void {
    if (!this.#isRefilling) {
      this.#startedBy = startedByOf(event, media);
      this.#isRefilling = true;
    }
  }

  /** How much film loaded ahead media is ready on now. */
  readonly readyOnMs = (media: FollowedMedia): number => this.#readyAheadMs(media);

  /** How many times faster than it plays its last chunk of picture loaded, to a tenth; undefined before one. */
  readonly loadsFaster = (): number | undefined =>
    this.#loadsFaster === undefined ? undefined : Math.round(this.#loadsFaster * 10) / 10;

  /** Whether it is refilling and has not yet loaded enough to play on. */
  readonly isShort = (media: FollowedMedia): boolean =>
    this.#isRefilling && !hasRefilled(media, this.#readyAheadMs(media));

  /** After a seek, what readyAheadOf says; after a stall, refillAheadMs. */
  #readyAheadMs(media: FollowedMedia): number {
    return this.#cause === "seek" ? readyAheadOf(media).aheadMs : refillAheadMs;
  }
}
