// SPDX-License-Identifier: MPL-2.0

/**
 * The tab's debug link (D380): a cap on its download rate, applied in the player's chunk fetch so
 * every carrier is capped alike (`shapeChunkLoads`, @orpheum/player), and a drop of its connection
 * for a while, which closes the tab's sockets, refuses its requests, sockets and chunk loads until it
 * ends, then lets them reconnect as after a real network loss, the browser's `online` said. Chrome's
 * DevTools throttling never reached the WebRTC carrier (VQ128's run), so a slow or lost link could be
 * tested only by turning Wi-Fi off. It lasts for the tab only: the cap is kept in the tab's session
 * storage, and the page says so the whole time (LinkCapBadge), as a DevTools box left ticked once
 * cost an afternoon (agent-failures A51).
 */

import { shapeChunkLoads } from "@orpheum/player";
import type { OpenTogetherSocket } from "@orpheum/sdk";

import type { Reaches } from "./client-reaches.ts";

/** Where the tab keeps its cap, kbit/s, in its session storage. */
export const linkCapKey = "orpheum.debug.linkCapKbps";

/** The caps a person may choose, kbit/s (D380 §1). */
export const linkCapChoicesKbps: readonly number[] = [500, 1000, 2000, 5000];

/** How long a drop may last, in seconds (D380 §1). */
export const dropChoicesS: readonly number[] = [10, 30, 60];

/** A socket the tab opened, as its opener gives it. */
type TogetherSocket = ReturnType<OpenTogetherSocket>;

/** The tab's storage, as far as the debug link keeps its cap there. */
type CapStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;

/** The clock a drop is timed on: the page's, or a test's. */
interface Clock {
  readonly nowMs: () => number;
  readonly setTimeout: (run: () => void, ms: number) => ReturnType<typeof setTimeout>;
  readonly clearTimeout: (timer: ReturnType<typeof setTimeout>) => void;
}

/** The tab's debug link: its cap, its drop, and the reaching of the core it shapes. */
export class LinkDebug {
  readonly #storage: CapStorage | undefined;
  readonly #clock: Clock;
  readonly #page: Pick<EventTarget, "dispatchEvent"> | undefined;
  readonly #listeners = new Set<() => void>();
  readonly #sockets = new Set<TogetherSocket>();
  #capKbps: number | undefined;
  #droppedUntilMs: number | undefined;
  #dropEnd: ReturnType<typeof setTimeout> | undefined;

  /**
   * The tab's debug link, its cap read back from storage; page is told the browser is back
   * `online` as a drop ends.
   */
  constructor(
    storage: CapStorage | undefined,
    clock: Clock,
    page?: Pick<EventTarget, "dispatchEvent">,
  ) {
    this.#storage = storage;
    this.#clock = clock;
    this.#page = page;
    const kept = Number(storage?.getItem(linkCapKey) ?? "");
    this.#capKbps = Number.isFinite(kept) && kept > 0 ? kept : undefined;
  }

  /** The tab's cap on its download rate, kbit/s; undefined for none. */
  readonly capKbps = (): number | undefined => this.#capKbps;

  /** Caps the tab's download rate at kbps, or lifts the cap (undefined), kept for the tab. */
  readonly setCap = (kbps: number | undefined): void => {
    this.#capKbps = kbps !== undefined && kbps > 0 ? kbps : undefined;
    if (this.#capKbps === undefined) {
      this.#storage?.removeItem(linkCapKey);
    } else {
      this.#storage?.setItem(linkCapKey, String(this.#capKbps));
    }
    this.#changed();
  };

  /** Whether the tab's connection is dropped now. */
  readonly isDropped = (): boolean =>
    this.#droppedUntilMs !== undefined && this.#clock.nowMs() < this.#droppedUntilMs;

  /** How many ms of the drop are left; 0 while there is none. */
  readonly droppedForMs = (): number =>
    this.isDropped() ? (this.#droppedUntilMs ?? 0) - this.#clock.nowMs() : 0;

  /**
   * Drops the tab's connection for ms: closes its every socket and refuses its requests, sockets and
   * chunk loads until then, then says the browser is back online, so they reconnect as after a loss.
   */
  readonly dropFor = (ms: number): void => {
    this.#droppedUntilMs = this.#clock.nowMs() + ms;
    for (const socket of [...this.#sockets]) {
      socket.close();
    }
    if (this.#dropEnd !== undefined) {
      this.#clock.clearTimeout(this.#dropEnd);
    }
    this.#dropEnd = this.#clock.setTimeout(() => {
      this.#dropEnd = undefined;
      this.#droppedUntilMs = undefined;
      this.#changed();
      this.#page?.dispatchEvent(new Event("online"));
    }, ms);
    this.#changed();
  };

  /** Calls listener at each change of the cap or the drop; returns the unsubscribing. */
  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  /**
   * reaches, its requests and sockets refused while the tab is dropped, and its sockets closed as a
   * drop begins; and the player's chunk loads shaped by the cap and the drop.
   */
  shape(reaches: Reaches): Reaches {
    shapeChunkLoads({ capKbps: this.capKbps, isDropped: this.isDropped });
    return {
      ...reaches,
      fetch: this.#fetchOf(reaches),
      openTogetherSocket: this.#socketsOf(reaches),
    };
  }

  #fetchOf(reaches: Reaches): Reaches["fetch"] {
    return async (request) => {
      if (this.isDropped()) {
        throw new TypeError("Failed to fetch: the connection is dropped (a debug tool, D380)");
      }
      return await reaches.fetch(request);
    };
  }

  #socketsOf(reaches: Reaches): OpenTogetherSocket {
    return (listeners) => {
      if (this.isDropped()) {
        // Refused as a lost network refuses it: the socket closes without opening.
        const timer = this.#clock.setTimeout(() => {
          listeners.onClose(abnormalClosure);
        }, 0);
        return {
          send: () => undefined,
          close: () => {
            this.#clock.clearTimeout(timer);
          },
        };
      }
      const socket = reaches.openTogetherSocket(listeners);
      const kept: TogetherSocket = {
        send: socket.send,
        close: () => {
          this.#sockets.delete(kept);
          socket.close();
        },
      };
      this.#sockets.add(kept);
      return kept;
    };
  }

  #changed(): void {
    for (const listener of this.#listeners) {
      listener();
    }
  }
}

/** The close code of a connection lost without a closing handshake (RFC 6455 §7.4.1). */
const abnormalClosure = 1006;
