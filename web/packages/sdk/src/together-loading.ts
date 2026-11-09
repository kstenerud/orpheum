// SPDX-License-Identifier: MPL-2.0

/**
 * What a following player tells its player to load: nothing more while its group waits at a sync
 * point and it is ready (D283 §2), and no more than a short runway while another player in its
 * group is still loading (D285 §2), so those behind have the server's upload. Kept apart from the
 * follower, which asks it.
 */

import type { GroupState } from "./together.ts";
import type { TogetherConnection, TogetherSnapshot } from "./together-connection.ts";
import { otherPlayersOf } from "./together-roster.ts";
import type { FollowedMedia } from "./together-media.ts";
import { holdLoadingEvent, loadingRunwayEvent } from "./together-player-events.ts";

/**
 * How far ahead a player loads at most while another player in its group is still loading: enough
 * to ride out a hiccup, little enough to leave the one behind the server's upload (D285 §2).
 */
export const cooperativeRunwayS = 10;

/** Whether another player in the session than this device is still loading, by the roster. */
function isAnotherLoading(snapshot: TogetherSnapshot): boolean {
  return otherPlayersOf(snapshot).some((device) => device.isBuffering);
}

/** What a following player last told its player to load, told again only when it changes. */
export class GroupLoading {
  readonly #media: Pick<FollowedMedia, "dispatchEvent">;
  readonly #connection: TogetherConnection;
  #isHeld = false;
  #runwayS: number | undefined = undefined;

  constructor(media: Pick<FollowedMedia, "dispatchEvent">, connection: TogetherConnection) {
    this.#media = media;
    this.#connection = connection;
  }

  /** Follows the group's state, this player ready or not: held while it waits and this is ready. */
  readonly follow = (state: GroupState, isReady: boolean): void => {
    this.#hold(state === "waiting" && isReady);
    this.#keepRunway(state === "playing" && isAnotherLoading(this.#connection.snapshot()));
  };

  /** Lets the player load as it would, as the follower stops. */
  readonly stop = (): void => {
    this.#hold(false);
    this.#keepRunway(false);
  };

  #hold(isHeld: boolean): void {
    if (isHeld !== this.#isHeld) {
      this.#isHeld = isHeld;
      this.#media.dispatchEvent(new CustomEvent(holdLoadingEvent, { detail: { isHeld } }));
      this.#connection.note({ kind: "loadHeld", why: isHeld ? "held" : "goesOn" });
    }
  }

  #keepRunway(isShort: boolean): void {
    const runwayS = isShort ? cooperativeRunwayS : undefined;
    if (runwayS !== this.#runwayS) {
      this.#runwayS = runwayS;
      this.#media.dispatchEvent(
        new CustomEvent(loadingRunwayEvent, { detail: { aheadS: runwayS } }),
      );
    }
  }
}
