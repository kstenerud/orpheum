// SPDX-License-Identifier: MPL-2.0

/**
 * One connection of an RtcTransport (rtc-transport.ts): its peer, the channels its requests and
 * sockets hold open, and the room-counting channel wrappers a request's and a socket's channel
 * are given.
 */

import type { CandidatePairSeen } from "./rtc-pairs.ts";
import type { ChannelLike } from "./rtc-request.ts";

/**
 * How many peer connections a client opens at most, while its requests crowd them: one is bound by
 * its congestion window, and four carried three times as much (reachability-approaches-2026-10-09.md
 * §9.4); the core's ClientConnections.
 */
export const clientConnections = 4;

/** What the transport needs of a peer connection: opening a channel, and hearing that it is gone. */
export interface PeerLike {
  createDataChannel(label: string): ChannelLike;
  close(): void;
  /**
   * Tells listener, once, that the connection is gone for good, and why: failed, closed, or
   * disconnected longer than disconnectedGraceMs (rtc-peer.ts). A test's peer may never tell.
   */
  onGone?(listener: (why: string) => void): void;
  /** Its candidate pairs as the browser sees them (rtc-pairs.ts); a test's peer may have none. */
  candidatePairs?(): Promise<CandidatePairSeen[]>;
}

/**
 * The first and the longest wait before a gone connection's replacement is tried again: once the
 * network is back, a page connects again within the longest, or at once when the browser says it is
 * online. At 30 s, a page whose tries had grown through a minute's network loss sat up to half a
 * minute more with none (the owner's Mac, staging, 2026-10-29; VQ130).
 */
export const replaceFirstMs = 1000;
export const replaceMostMs = 10_000;

/**
 * The most requests a connection carries at once: the core serves rtcapi's channelLimit, 16, on each
 * and refuses the rest, and a Watch Together socket holds a channel of its own for as long as it is
 * open; the rest wait their turn here instead of being refused (the owner's browse page, 2026-10-15,
 * whose 60 posters crowded out the requests after them).
 */
export const requestsPerConnection = 12;

/**
 * How long a channel the client closed holds its room at most: its close event, which comes once the
 * core has closed its end too, gives the room back; this is the backstop for a browser that never
 * sends it.
 */
export const closingHoldsMs = 5000;

/**
 * A channel that calls release once, when its close or error event comes: a request's channel lives
 * until its body ends and the core has closed its end, as long as the core counts it. Released when
 * the client merely began closing it, the next request's channel reached the core before the close
 * did, past its 16, and each refusal freed room for another, the queue draining as broken posters
 * (nas.example, 2026-10-19: eighteen refusals in two milliseconds).
 */
export function countedChannel(
  channel: ChannelLike,
  release: () => void,
  delivered: () => void,
): ChannelLike {
  let isReleased = false;
  const releaseOnce = (): void => {
    if (!isReleased) {
      isReleased = true;
      release();
    }
  };
  return {
    get readyState() {
      return channel.readyState;
    },
    send: (data) => {
      channel.send(data);
    },
    whenSent: channel.whenSent,
    close: () => {
      channel.close();
      setTimeout(releaseOnce, closingHoldsMs);
    },
    listen: (listener) => {
      channel.listen({
        ...listener,
        message: (data) => {
          delivered();
          listener.message(data);
        },
        close: () => {
          releaseOnce();
          listener.close();
        },
        error: () => {
          releaseOnce();
          listener.error();
        },
      });
    },
  };
}

/**
 * A channel that calls release once, when it closes, by either end: a socket's, which holds its
 * connection open while it is.
 */
export function heldChannel(channel: ChannelLike, release: () => void): ChannelLike {
  let isReleased = false;
  const releaseOnce = (): void => {
    if (!isReleased) {
      isReleased = true;
      release();
    }
  };
  return {
    get readyState() {
      return channel.readyState;
    },
    send: (data) => {
      channel.send(data);
    },
    close: () => {
      channel.close();
      releaseOnce();
    },
    listen: (listener) => {
      channel.listen({
        ...listener,
        close: () => {
          releaseOnce();
          listener.close();
        },
        error: () => {
          releaseOnce();
          listener.error();
        },
      });
    },
  };
}

/**
 * How many requests every connection up carries before the transport opens another, up to its size:
 * a page browsing or waiting carries one request at a time, which one connection serves as well as
 * four, and four opened for every tab at once filled a network's share of the core's connections
 * with four tabs, connections that carried no request in five minutes (staging, 2026-10-25, the
 * owner's four browsers holding all sixteen and the Fire TV turned away).
 */
export const growWhenEachCarries = 2;

/**
 * How long a connection beyond a transport's last carries no request and no socket before it is
 * closed: long enough that a page loading posters as it scrolls keeps the ones it opened, short
 * enough that a tab left open holds one connection of its network's share, not four.
 */
export const idleClosesAfterMs = 30_000;

/**
 * How long a connection must live before its going is replaced at once: one gone sooner, as one the
 * core closes as soon as it connects, is replaced only after the transport's growing wait, which a
 * connection that lives this long resets. Replaced at once, each one closed made another, about one a
 * second, for as long as the page stayed open (staging, 2026-10-25).
 */
export const settledAfterMs = 10_000;

/** How many connections this page's transports have opened, which numbers each for its log. */
let connectionsOpened = 0;

/** One of the transport's connections, what it carries, and whether it is still new. */
export class Link {
  underWay = 0;
  /** How many sockets' channels it holds open (openChannel), which keep it from closing as idle. */
  sockets = 0;
  isNew = true;
  /** Its number among the page's connections, from 1, for its log. */
  readonly number = (connectionsOpened += 1);
  /** When a byte of any request's answer last came over it, on the transport's clock; undefined before one. */
  deliveredAtMs: number | undefined;
  private idle: ReturnType<typeof setTimeout> | undefined;

  constructor(
    readonly peer: PeerLike,
    /** When it was taken into use, on the transport's clock. */
    readonly upSinceMs: number,
  ) {}

  /** Whether it carries nothing: no request under way, and no socket. */
  get isIdle(): boolean {
    return this.underWay === 0 && this.sockets === 0;
  }

  /** Calls then once it has stayed idle for ms, from now; not while it carries anything. */
  whenIdleFor(ms: number, then: () => void): void {
    this.stopIdle();
    if (this.isIdle) {
      this.idle = setTimeout(() => {
        this.idle = undefined;
        if (this.isIdle) {
          then();
        }
      }, ms);
    }
  }

  stopIdle(): void {
    clearTimeout(this.idle);
    this.idle = undefined;
  }
}

/**
 * The connection carrying a request, as its log names it (carrierOf): its number among the page's
 * connections, how long it has been up, how many requests it carries, and how long since a byte of
 * any came over it, in ms, undefined before one did. A Fire TV's chunk stuck on its way gave up a
 * quality, and nothing said which connection it waited on, nor whether that connection carried
 * anything else meanwhile (the owner, 2026-10-29: the logs are to show whether stalls are
 * per-connection).
 */
export interface CarrierSeen {
  readonly connection: number;
  readonly upMs: number;
  readonly underWay: number;
  readonly sinceDeliveredMs: number | undefined;
  /**
   * Why the transport ended the request itself, once it no longer carries it: it heard nothing for
   * silentAtMostMs, and whether its connection, silent as long, was taken as gone. Undefined while
   * it is under way.
   */
  readonly ended?: string;
}

/** The link of links with the fewest requests under way, or none. */
export function fewestUnderWay(links: readonly Link[]): Link | undefined {
  return links.reduce<Link | undefined>(
    (least, each) => (least === undefined || each.underWay < least.underWay ? each : least),
    undefined,
  );
}
