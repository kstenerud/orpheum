// SPDX-License-Identifier: MPL-2.0

/**
 * The API over WebRTC data channels (D273 §8, plans/webrtc-transport.md): the direct path to a core
 * no HTTPS address reaches. Each request goes on a data channel of its own: a text head (method, path,
 * headers), the body in binary messages of at most `maxMessageBytes`, an empty message ending it;
 * the core answers on the same channel the same way, and the client closes it. A client opens
 * up to `clientConnections` peer connections, one kept and more only while its requests crowd it, and
 * sends each request on the one with the fewest under way; making them is rtc-connect.ts.
 */

import { reportFailure } from "./problem.ts";
import type { CandidatePairSeen } from "./rtc-pairs.ts";
import { backAfter, BackOnline, withinOpenTry, type Without } from "./rtc-reopen.ts";
import { abortReason, sendOnChannel, silentAtMostMs, type ChannelLike } from "./rtc-request.ts";

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
function countedChannel(
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

/** How many connections this page's transports have opened, which numbers each for its log. */
let connectionsOpened = 0;

/** One of the transport's connections, what it carries, and whether it is still new. */
class Link {
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

/** How many requests the transport ended itself it remembers, by URL, for carrierOf. */
const endedKept = 32;

/** The link of links with the fewest requests under way, or none. */
function fewestUnderWay(links: readonly Link[]): Link | undefined {
  return links.reduce<Link | undefined>(
    (least, each) => (least === undefined || each.underWay < least.underWay ? each : least),
    undefined,
  );
}

/**
 * How a transport keeps its connections: opening another, which gives up its try once signal aborts,
 * how many it keeps at most, and hearing the browser come back online (the window's online event),
 * which tries at once; returns the end of hearing.
 */
export interface Growth {
  readonly open: (signal: AbortSignal) => Promise<PeerLike>;
  readonly size: number;
  readonly onOnline?: ((listener: () => void) => () => void) | undefined;
}

/**
 * A client's WebRTC transport to its core: up to `clientConnections` peer connections, each request
 * sent on the one with the fewest under way. With growth, it keeps one connection up, opening another
 * only while every one up carries growWhenEachCarries requests or a request waits for room, one at a
 * time, up to its size, and closing one beyond the last once it has carried nothing for
 * idleClosesAfterMs. It takes one that is gone out of use at once and opens another in its place when
 * it is still wanted, retrying each failure, a refusal included, and each try that takes longer than
 * openTryMostMs, after a wait that doubles up to replaceMostMs, and at once when the browser comes
 * back online, a try under way then given up (plans/webrtc-transport.md W2d). `fetch` is what the
 * API client sends requests with; while no connection is up, a request waits for one.
 */
export class RtcTransport {
  private readonly links: Link[] = [];
  private readonly growth: Growth | undefined;
  private opening = 0;
  private retryMs = replaceFirstMs;
  /** The wait before the next try, while one is waited out. */
  private retryTimer: ReturnType<typeof setTimeout> | undefined;
  /** The try to open a connection under way, to give up when the browser comes back online. */
  private trying: AbortController | undefined;
  /** Since when, and why, it has had no connection; undefined while it has one. */
  private without: Without | undefined;
  private readonly backListeners = new Set<() => void>();
  private readonly stopHearingOnline: () => void;
  private isClosed = false;
  private readonly waiting: (() => void)[] = [];
  /** The connection carrying each request under way, by its URL, the latest asked for of each. */
  private readonly carrying = new Map<string, Link>();
  /** The requests it ended itself, as carrierOf says them, by URL, the latest endedKept. */
  private readonly ended = new Map<string, CarrierSeen>();
  private readonly nowMs: () => number;

  /** A transport over peers, growing as growth says, its connections' times read from nowMs. */
  constructor(
    peers: readonly PeerLike[],
    growth?: Growth,
    nowMs: () => number = () => performance.now(),
  ) {
    if (peers.length === 0) {
      throw new Error("a transport needs at least one connection");
    }
    this.nowMs = nowMs;
    this.growth = growth;
    for (const peer of peers) {
      this.add(peer);
    }
    this.stopHearingOnline =
      growth?.onOnline?.(() => {
        this.backOnline();
      }) ?? ((): void => undefined);
  }

  /**
   * Tells listener each time the way to the core may work again: the browser came back online, or a
   * connection is up after a time with none. The session connection connects again at once on it,
   * rather than waiting out its own growing wait (the owner's Chrome, staging, 2026-10-29: its media
   * played on for 28 s, out of step, before its session's socket tried again). Returns its removal.
   */
  onBack(listener: () => void): () => void {
    this.backListeners.add(listener);
    return () => {
      this.backListeners.delete(listener);
    };
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
    const { url } = request;
    let opened: ChannelLike;
    try {
      opened = link.peer.createDataChannel("request");
    } catch (problem: unknown) {
      // A connection that opens no channel carries nothing more: it is replaced, and its room given back.
      link.underWay -= 1;
      this.gone(link, "it opened no channel");
      this.wakeWaiting();
      throw problem;
    }
    this.ended.delete(url);
    this.carrying.set(url, link);
    const channel = countedChannel(
      opened,
      () => {
        link.underWay -= 1;
        if (this.carrying.get(url) === link) {
          this.carrying.delete(url);
        }
        this.closeWhenIdle(link);
        this.wakeWaiting();
      },
      () => {
        link.deliveredAtMs = this.nowMs();
      },
    );
    try {
      return await sendOnChannel(channel, request, () => {
        this.silent(link, url);
      });
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

  /**
   * The connection carrying the request for url while it is under way, as its log names it; undefined
   * for one this transport does not carry now.
   */
  carrierOf(url: string): CarrierSeen | undefined {
    const link = this.carrying.get(url);
    const ended = this.ended.get(url);
    if (link === undefined || ended !== undefined) {
      return ended;
    }
    const nowMs = this.nowMs();
    return {
      connection: link.number,
      upMs: nowMs - link.upSinceMs,
      underWay: link.underWay,
      sinceDeliveredMs: link.deliveredAtMs === undefined ? undefined : nowMs - link.deliveredAtMs,
    };
  }

  /**
   * The request for url heard nothing for silentAtMostMs on link: remembered for carrierOf, and link,
   * when nothing of any request came over it for as long, taken as gone and replaced, its other
   * requests failing with it. A connection whose data channels stopped while it stayed connected
   * otherwise carried every request after them into the same silence (the owner's Chrome, throttled
   * and unthrottled, staging, 2026-10-29).
   */
  private silent(link: Link, url: string): void {
    const nowMs = this.nowMs();
    const sinceDeliveredMs =
      link.deliveredAtMs === undefined ? undefined : nowMs - link.deliveredAtMs;
    const isLinkSilent = sinceDeliveredMs === undefined || sinceDeliveredMs >= silentAtMostMs;
    this.ended.delete(url);
    this.ended.set(url, {
      connection: link.number,
      upMs: nowMs - link.upSinceMs,
      underWay: link.underWay,
      sinceDeliveredMs,
      ended: `nothing came for ${String(silentAtMostMs)} ms${isLinkSilent ? ", nor over its connection, which was replaced" : ""}`,
    });
    for (const oldest of this.ended.keys()) {
      if (this.ended.size <= endedKept) {
        break;
      }
      this.ended.delete(oldest);
    }
    if (isLinkSilent) {
      this.gone(link, `nothing came over it for ${String(silentAtMostMs)} ms`);
    }
  }

  /** Closes every connection, and opens no more. */
  close(): void {
    this.isClosed = true;
    this.stopHearingOnline();
    clearTimeout(this.retryTimer);
    this.trying?.abort();
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
    const link = new Link(peer, this.nowMs());
    this.links.push(link);
    setTimeout(() => {
      link.isNew = false;
      if (this.links.includes(link)) {
        this.retryMs = replaceFirstMs;
      }
    }, settledAfterMs);
    this.closeWhenIdle(link);
    this.wakeWaiting();
    peer.onGone?.((why) => {
      this.gone(link, why);
    });
    const without = this.without;
    if (without !== undefined) {
      this.without = undefined;
      // Said once the connection is up, so the report goes over it to the core's log.
      reportFailure(new Error(backAfter(without, this.nowMs())));
      this.tellBack();
    }
  }

  /**
   * Takes link out of use for good and closes it, once: its peer said it is gone, or it carries
   * nothing any more, why says which. Another opens in its place when one is wanted: at once for one
   * that had lived settledAfterMs, after the growing wait for one that had not. The last one gone
   * starts a time without one, which the next one up reports.
   */
  private gone(link: Link, why: string): void {
    if (!this.links.includes(link)) {
      return;
    }
    link.stopIdle();
    this.drop(link);
    link.peer.close();
    if (this.links.length === 0 && !this.isClosed) {
      this.without ??= { sinceMs: this.nowMs(), why, tries: 0, lastFailure: undefined };
    }
    if (link.isNew) {
      this.retryLater();
    } else {
      this.grow();
    }
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
    const trying = new AbortController();
    this.trying = trying;
    try {
      const peer = await withinOpenTry(growth.open(trying.signal), trying);
      this.opening -= 1;
      if (this.isClosed) {
        peer.close();
        return;
      }
      this.add(peer);
      this.grow();
    } catch (problem: unknown) {
      this.opening -= 1;
      if (this.without !== undefined) {
        this.without.tries += 1;
        this.without.lastFailure = problem instanceof Error ? problem.message : String(problem);
      }
      if (trying.signal.reason instanceof BackOnline) {
        // The browser came back online: tried anew at once, the wait already started over.
        this.grow();
      } else {
        // A connection that could not be made, or the core turned away, is tried after a wait.
        this.retryLater();
      }
    } finally {
      if (this.trying === trying) {
        this.trying = undefined;
      }
    }
  }

  /** Looks again whether another connection is wanted after a wait, longer each time up to the most. */
  private retryLater(): void {
    if (this.isClosed || this.retryTimer !== undefined) {
      return;
    }
    const wait = this.retryMs;
    this.retryMs = Math.min(this.retryMs * 2, replaceMostMs);
    this.opening += 1;
    this.retryTimer = setTimeout(() => {
      this.retryTimer = undefined;
      this.opening -= 1;
      this.grow();
    }, wait);
  }

  /**
   * The browser came back online: a try under way, begun on the network that went, is given up and
   * one made anew, a wait under way cut short, and the wait starts over from replaceFirstMs; the
   * session connection is told (onBack).
   */
  private backOnline(): void {
    if (this.isClosed) {
      return;
    }
    this.retryMs = replaceFirstMs;
    if (this.retryTimer !== undefined) {
      clearTimeout(this.retryTimer);
      this.retryTimer = undefined;
      this.opening -= 1;
    }
    // Its failure makes the next try at once (openOne).
    this.trying?.abort(new BackOnline());
    this.grow();
    this.tellBack();
  }

  /** Tells each onBack listener that the way to the core may work again. */
  private tellBack(): void {
    for (const listener of this.backListeners) {
      listener();
    }
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
