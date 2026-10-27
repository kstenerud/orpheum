// SPDX-License-Identifier: MPL-2.0

/**
 * The API over WebRTC data channels (D273 §8, plans/webrtc-transport.md): the direct path to a core
 * no HTTPS address reaches. Each request goes on a data channel of its own: a text head (method, path,
 * headers), the body in binary messages of at most `maxMessageBytes`, an empty message ending it;
 * the core answers on the same channel the same way, and the client closes it. A client opens
 * up to `clientConnections` peer connections, one kept and more only while its requests crowd it, and
 * sends each request on the one with the fewest under way; making them is rtc-connect.ts.
 */

import { candidatePairsOf, type CandidatePairSeen } from "./rtc-pairs.ts";
import { abortReason, channelOf, sendOnChannel, type ChannelLike } from "./rtc-request.ts";

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
   * Tells listener, once, that the connection is gone for good: failed, closed, or disconnected
   * longer than disconnectedGraceMs. A test's peer may never tell.
   */
  onGone?(listener: () => void): void;
  /** Its candidate pairs as the browser sees them (rtc-pairs.ts); a test's peer may have none. */
  candidatePairs?(): Promise<CandidatePairSeen[]>;
}

/**
 * How long a connection may stay disconnected before it counts as gone and is replaced: a phone's
 * moment out of signal comes back by itself, but one moving from Wi-Fi to mobile data never does
 * until an ICE restart, which the core cannot take yet (plans/webrtc-transport.md W2d).
 */
export const disconnectedGraceMs = 5000;

/** The first and the longest wait before a gone connection's replacement is tried again. */
export const replaceFirstMs = 1000;
export const replaceMostMs = 30_000;

/**
 * A browser's peer connection as the transport uses it: each channel delivers ArrayBuffers, and it
 * says once when it is gone: failed, closed, or disconnected past disconnectedGraceMs.
 */
export function peerOf(peer: RTCPeerConnection): PeerLike {
  return {
    createDataChannel: (label) => {
      const channel = peer.createDataChannel(label);
      channel.binaryType = "arraybuffer";
      return channelOf(channel);
    },
    close: () => {
      peer.close();
    },
    candidatePairs: async () => candidatePairsOf((await peer.getStats()).values()),
    onGone: (listener) => {
      let isGone = false;
      let grace: ReturnType<typeof setTimeout> | undefined;
      const gone = (): void => {
        clearTimeout(grace);
        if (!isGone) {
          isGone = true;
          listener();
        }
      };
      peer.addEventListener("connectionstatechange", () => {
        const state = peer.connectionState;
        if (state === "failed" || state === "closed") {
          gone();
        } else if (state === "disconnected") {
          grace ??= setTimeout(gone, disconnectedGraceMs);
        } else {
          clearTimeout(grace);
          grace = undefined;
        }
      });
    },
  };
}

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
function countedChannel(channel: ChannelLike, release: () => void): ChannelLike {
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
      setTimeout(releaseOnce, closingHoldsMs);
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
 * A channel that calls release once, when it closes, by either end: a socket's, which holds its
 * connection open while it is.
 */
function heldChannel(channel: ChannelLike, release: () => void): ChannelLike {
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

/** One of the transport's connections, what it carries, and whether it is still new. */
class Link {
  underWay = 0;
  /** How many sockets' channels it holds open (openChannel), which keep it from closing as idle. */
  sockets = 0;
  isNew = true;
  private idle: ReturnType<typeof setTimeout> | undefined;

  constructor(readonly peer: PeerLike) {}

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

/** The link of links with the fewest requests under way, or none. */
function fewestUnderWay(links: readonly Link[]): Link | undefined {
  return links.reduce<Link | undefined>(
    (least, each) => (least === undefined || each.underWay < least.underWay ? each : least),
    undefined,
  );
}

/** How a transport keeps its connections: opening another, and how many it keeps at most. */
export interface Growth {
  readonly open: () => Promise<PeerLike>;
  readonly size: number;
}

/**
 * A client's WebRTC transport to its core: up to `clientConnections` peer connections, each request
 * sent on the one with the fewest under way. With growth, it keeps one connection up, opening another
 * only while every one up carries growWhenEachCarries requests or a request waits for room, one at a
 * time, up to its size, and closing one beyond the last once it has carried nothing for
 * idleClosesAfterMs. It takes one that is gone out of use at once and opens another in its place when
 * it is still wanted, retrying each failure, a refusal included, after a wait that doubles
 * (plans/webrtc-transport.md W2d). `fetch` is what the API client sends requests with; while no
 * connection is up, a request waits for one.
 */
export class RtcTransport {
  private readonly links: Link[] = [];
  private readonly growth: Growth | undefined;
  private opening = 0;
  private retryMs = replaceFirstMs;
  private isClosed = false;
  private readonly waiting: (() => void)[] = [];

  constructor(peers: readonly PeerLike[], growth?: Growth) {
    if (peers.length === 0) {
      throw new Error("a transport needs at least one connection");
    }
    this.growth = growth;
    for (const peer of peers) {
      this.add(peer);
    }
  }

  /** How many connections are up. */
  get size(): number {
    return this.links.length;
  }

  /**
   * Sends a request to the core and resolves with its response. A request whose signal has aborted
   * fails with its reason, as fetch does: at once and opening nothing when it aborted before, out of
   * the queue when it aborts waiting for room, and with its channel closed once it has one.
   */
  readonly fetch = async (request: Request): Promise<Response> => {
    const { signal } = request;
    if (signal.aborted) {
      throw abortReason(signal);
    }
    // Chosen at once while one has room, so requests made together spread across the connections.
    const link = this.claimNow() ?? (await this.claim(signal));
    this.grow();
    const channel = countedChannel(link.peer.createDataChannel("request"), () => {
      link.underWay -= 1;
      this.closeWhenIdle(link);
      this.wakeWaiting();
    });
    try {
      return await sendOnChannel(channel, request);
    } catch (problem: unknown) {
      channel.close();
      throw problem;
    }
  };

  /**
   * A channel of its own on the connection with the fewest under way, for a request that holds it
   * open: a WebSocket's messages (rtc-together.ts). With no connection up, it throws, and the socket
   * is opened again later. Its connection is not closed as idle while it is open.
   */
  openChannel(): ChannelLike {
    // A socket's channel is held apart from the requests' room (requestsPerConnection).
    const link = fewestUnderWay(this.links);
    if (link === undefined) {
      throw new Error("no WebRTC connection to the core is up");
    }
    link.sockets += 1;
    link.stopIdle();
    return heldChannel(link.peer.createDataChannel("upgrade"), () => {
      link.sockets -= 1;
      this.closeWhenIdle(link);
    });
  }

  /**
   * The candidate pairs of the connection the transport holds longest, as the browser sees them, or
   * none while it holds none or its connections cannot say.
   */
  async candidatePairs(): Promise<readonly CandidatePairSeen[]> {
    return (await this.links[0]?.peer.candidatePairs?.()) ?? [];
  }

  /** Closes every connection, and opens no more. */
  close(): void {
    this.isClosed = true;
    for (const link of this.links.splice(0)) {
      link.stopIdle();
      link.peer.close();
    }
    // A request waiting for a connection fails now rather than waiting for good.
    for (const wake of this.waiting.splice(0)) {
      wake();
    }
  }

  /**
   * Takes peer into use, and out of it once it is gone, opening another in its place when one is
   * wanted: at once for one that had lived settledAfterMs, after the growing wait for one that had not.
   */
  private add(peer: PeerLike): void {
    const link = new Link(peer);
    this.links.push(link);
    setTimeout(() => {
      link.isNew = false;
      if (this.links.includes(link)) {
        this.retryMs = replaceFirstMs;
      }
    }, settledAfterMs);
    this.closeWhenIdle(link);
    this.wakeWaiting();
    peer.onGone?.(() => {
      link.stopIdle();
      this.drop(link);
      peer.close();
      if (link.isNew) {
        this.retryLater();
      } else {
        this.grow();
      }
    });
  }

  /** Takes link out of use. */
  private drop(link: Link): void {
    const index = this.links.indexOf(link);
    if (index >= 0) {
      this.links.splice(index, 1);
    }
  }

  /**
   * Closes link once it has carried nothing for idleClosesAfterMs, unless it is the last up: the
   * page keeps one connection, and opens more only as its requests crowd it.
   */
  private closeWhenIdle(link: Link): void {
    link.whenIdleFor(idleClosesAfterMs, () => {
      if (this.links.length > 1 && this.links.includes(link)) {
        this.drop(link);
        link.peer.close();
      }
    });
  }

  /**
   * Whether another connection is wanted: none is up, a request waits for room, or every one up
   * carries growWhenEachCarries; never while one is opening or waiting to be tried again, nor past
   * the growth's size.
   */
  private wantsAnother(growth: Growth): boolean {
    if (this.isClosed || this.opening > 0 || this.links.length >= growth.size) {
      return false;
    }
    return (
      this.links.length === 0 ||
      this.waiting.length > 0 ||
      this.links.every((link) => link.underWay >= growWhenEachCarries)
    );
  }

  /** Opens another connection when one is wanted. */
  private grow(): void {
    const growth = this.growth;
    if (growth !== undefined && this.wantsAnother(growth)) {
      this.opening += 1;
      void this.openOne(growth);
    }
  }

  private async openOne(growth: Growth): Promise<void> {
    try {
      const peer = await growth.open();
      this.opening -= 1;
      if (this.isClosed) {
        peer.close();
        return;
      }
      this.add(peer);
      this.grow();
    } catch {
      // A connection that could not be made, or the core turned away, is tried after a wait.
      this.opening -= 1;
      this.retryLater();
    }
  }

  /** Looks again whether another connection is wanted after a wait, longer each time. */
  private retryLater(): void {
    const wait = this.retryMs;
    this.retryMs = Math.min(this.retryMs * 2, replaceMostMs);
    this.opening += 1;
    setTimeout(() => {
      this.opening -= 1;
      this.grow();
    }, wait);
  }

  /** Wakes every request waiting for a connection with room, to look again. */
  private wakeWaiting(): void {
    for (const wake of this.waiting.splice(0)) {
      wake();
    }
  }

  /**
   * A room on the connection with the fewest under way, taken at once, when one is up and has room.
   * Taken in the same step as it is found: every request waiting is woken together when a room frees,
   * and one that only found it, to take it after an await, let each of them see the same room free,
   * so one freed room sent the whole queue past the core's 16 on one connection, which refused them
   * (staging, 2026-10-20).
   */
  private claimNow(): Link | undefined {
    const link = fewestUnderWay(this.links.filter((each) => each.underWay < requestsPerConnection));
    if (link !== undefined) {
      link.underWay += 1;
      link.stopIdle();
    }
    return link;
  }

  /**
   * A room claimNow takes, waiting for one while none is up or has room, until signal aborts: the
   * request then leaves the queue and fails with its reason.
   */
  private async claim(signal: AbortSignal): Promise<Link> {
    for (;;) {
      const link = this.claimNow();
      if (link !== undefined) {
        return link;
      }
      if (this.isClosed) {
        throw new Error("the WebRTC transport is closed");
      }
      if (signal.aborted) {
        throw abortReason(signal);
      }
      await new Promise<void>((resolve, reject) => {
        const wake = (): void => {
          signal.removeEventListener("abort", aborted);
          resolve();
        };
        const aborted = (): void => {
          const index = this.waiting.indexOf(wake);
          if (index >= 0) {
            this.waiting.splice(index, 1);
          }
          reject(abortReason(signal));
        };
        this.waiting.push(wake);
        signal.addEventListener("abort", aborted, { once: true });
        this.grow();
      });
    }
  }
}

/** Sends a request and resolves with its response: the browser's fetch, or a transport's. */
export type FetchRequest = (request: Request) => Promise<Response>;

/**
 * The fetch the API client sends with, whose path can change while the app runs: the first path to
 * the core that answers is used at once, and a better one answering later takes over (D273 §7).
 * A request already sent finishes on the path it took.
 */
export class SwitchableFetch {
  private current: FetchRequest;

  /** Starts on first: the browser's own fetch when omitted. */
  constructor(first: FetchRequest = (request) => fetch(request)) {
    this.current = first;
  }

  /** Sends a request on the path in use now. */
  readonly fetch: FetchRequest = (request) => this.current(request);

  /** Moves the requests sent from now on to next, the path called name, and says so in the console. */
  use(next: FetchRequest, name: string): void {
    this.current = next;
    // eslint-disable-next-line no-console -- which path a tab's requests take is read in its console (D273 §7)
    console.info(`Orpheum: requests now go over ${name}`);
  }
}
