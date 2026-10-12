// SPDX-License-Identifier: MPL-2.0

/**
 * When a following player is ready to play on after it stalled or sought: refillAheadMs of film
 * loaded after a stall, and after a seek its group or its person made, only seekReadyAheadMs when
 * its last chunk loaded well faster than it plays. Every other player starts on a second or two
 * after a seek (Media3 1 s, Jellyfin SyncPlay its first frame); ten seconds kept a jump's group
 * waiting about ten (seek-resync-2026-10-12.md; the owner, 2026-10-12: "Those sound good").
 */

import type { ChunkLoaded } from "./together-player-events.ts";
import {
  hasRefilled,
  haveFutureData,
  refillAheadMs,
  type FollowedMedia,
} from "./together-media.ts";

/** How much film a player loaded after a seek is ready on, when its link keeps up well. */
export const seekReadyAheadMs = 3000;

/**
 * How many times faster than it plays the player's last chunk of picture must have loaded for
 * seekReadyAheadMs to be enough: a link only just keeping up stalls again at once.
 */
export const seekReadyLoadsAtLeast = 1.5;

/** Why the player is loading before it can play on: a stall or a catch-up's seek, or a seek. */
type RefillCause = "stall" | "seek";

/** Whether a player refills, why, and how fast its last chunk of picture loaded. */
export class Refill {
  #isRefilling = false;
  #cause: RefillCause = "stall";
  /** Whether the next seeking is the player's catch-up, a stall in all but name (D247 §2). */
  #isNextSeekCatchUp = false;
  /** Film loaded per wall time of its last chunk of picture; undefined before one loaded. */
  #loadsFaster: number | undefined;

  /** Whether it is refilling. */
  get isRefilling(): boolean {
    return this.#isRefilling;
  }

  /** It seeks itself, why as its note says: a catch-up's seek refills as a stall does. */
  readonly sought = (why: string): void => {
    this.#isNextSeekCatchUp = why === "catchUp";
  };

  /** A chunk loaded: a picture's says how fast the link carries the film. */
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
      this.#isRefilling = true;
    } else if (this.#isRefilling && hasRefilled(media, this.#readyAheadMs())) {
      this.#isRefilling = false;
    }
  };

  /** Whether it is refilling and has not yet loaded enough to play on. */
  readonly isShort = (media: FollowedMedia): boolean =>
    this.#isRefilling && !hasRefilled(media, this.#readyAheadMs());

  #readyAheadMs(): number {
    return this.#cause === "seek" && (this.#loadsFaster ?? 0) >= seekReadyLoadsAtLeast
      ? seekReadyAheadMs
      : refillAheadMs;
  }
}
