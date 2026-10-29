// SPDX-License-Identifier: MPL-2.0

/**
 * How long a socket has to answer the ping a device sends as its page is shown again before it is
 * taken for dead and connected again, as deadAfterUnansweredPings takes it, without the half minute
 * those pings take: an S23 unlocked after 35 s sat paused for 20 s on a socket that sent but heard
 * nothing, out of step, until its pings ran out (the owner's VQ105, 2026-10-29). A live socket
 * answers in a round trip, tens of ms on Wi-Fi and a TV's 6–30 ms (webrtc-throughput-review,
 * 2026-10-28), and a call is judged poor at 500 ms (network-profiles.md); three times that leaves a
 * phone's radio waking from its lock time to answer, and keeps a wrong verdict rare, which costs
 * only a reconnect. Fixed rather than a setting: a bound on a dead socket, as the pings' is, not a
 * way to follow the group.
 */
export const shownAnswerWithinMs = 1500;

/** What a check of a socket, as its page is shown again, does with the socket. */
export interface ShownCheckDoes {
  /** The monotonic clock, in whole ms. */
  readonly nowMs: () => number;
  /** Sends a ping of tMono on the socket. */
  readonly ping: (tMono: number) => void;
  /** The socket answered, roundTripMs after it was pinged. */
  readonly alive: (roundTripMs: number) => void;
  /**
   * Nothing answered within shownAnswerWithinMs of the ping sent at atMs: the socket is dead, found so
   * as its page was shown again ("shown") or the way to the core came back ("back").
   */
  readonly dead: (atMs: number, why: "shown" | "back") => void;
}

/**
 * The check a connection makes of its socket as its page is shown again: a ping at once, and the
 * socket taken for dead unless that ping's pong comes within shownAnswerWithinMs; one at a time.
 */
export class ShownCheck {
  readonly #does: ShownCheckDoes;
  /** The ping under way, by its tMono, when it went and the wait for it; undefined while none is. */
  #pending:
    | {
        readonly pingTMono: number;
        readonly atMs: number;
        readonly timer: ReturnType<typeof setTimeout>;
      }
    | undefined;

  constructor(does: ShownCheckDoes) {
    this.#does = does;
  }

  /** Pings the socket, unless a check is under way already; why is what set it off. */
  start(why: "shown" | "back"): void {
    if (this.#pending !== undefined) {
      return;
    }
    const atMs = this.#does.nowMs();
    const pingTMono = Math.max(1, atMs);
    const timer = setTimeout(() => {
      this.#pending = undefined;
      this.#does.dead(atMs, why);
    }, shownAnswerWithinMs);
    this.#pending = { pingTMono, atMs, timer };
    this.#does.ping(pingTMono);
  }

  /** A pong of tMono came: the check's answer, if it is the ping it sent. */
  heardPong(tMono: number): void {
    const pending = this.#pending;
    if (pending?.pingTMono !== tMono) {
      return;
    }
    this.stop();
    this.#does.alive(this.#does.nowMs() - pending.atMs);
  }

  /** Gives up the check under way, as its socket is given up or welcomed anew. */
  stop(): void {
    clearTimeout(this.#pending?.timer);
    this.#pending = undefined;
  }
}
