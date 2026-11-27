// SPDX-License-Identifier: MPL-2.0

/**
 * The API over WebRTC data channels (D273 §8, plans/webrtc-transport.md): the direct path to a core
 * no HTTPS address reaches. Each request goes on a data channel of its own: a text head (method, path,
 * headers), the body in binary messages of at most `maxMessageBytes`, an empty message ending it;
 * the core answers on the same channel the same way, and the client closes it. A client opens
 * up to `clientConnections` peer connections, one kept and more only while its requests crowd it, and
 * sends each request on the one with the fewest under way; making them is rtc-connect.ts.
 */

import { apiPrefix, artPathPrefix } from "./client.ts";
import { reportFailure } from "./problem.ts";
import type { CandidatePairSeen } from "./rtc-pairs.ts";
import {
  backAfter,
  BackOnline,
  HiddenPace,
  isRefusedForGood,
  openTryMostMs,
  isUnreached,
  unreachedRetryMs,
  withinOpenTry,
  type HiddenPacing,
  type Without,
} from "./rtc-reopen.ts";
import { abortReason, sendOnChannel, silentAtMostMs, type ChannelLike } from "./rtc-request.ts";
import {
  clientConnections,
  closingHoldsMs,
  cosmeticRoomsPerConnection,
  countedChannel,
  fewestUnderWay,
  growWhenEachCarries,
  heldChannel,
  idleClosesAfterMs,
  Link,
  replaceFirstMs,
  replaceMostMs,
  requestsPerConnection,
  settledAfterMs,
  type CarrierSeen,
  type PeerLike,
} from "./rtc-transport-link.ts";

export {
  clientConnections,
  closingHoldsMs,
  cosmeticRoomsPerConnection,
  growWhenEachCarries,
  idleClosesAfterMs,
  replaceFirstMs,
  replaceMostMs,
  requestsPerConnection,
  settledAfterMs,
  type CarrierSeen,
  type PeerLike,
};

/** How many requests the transport ended itself it remembers, by URL, for carrierOf. */
const endedKept = 32;

/**
 * The rung a request waits for room on (D479): "cosmetic" for what the page works
 * without, an image and an interest note; "waited-on" for every other, what a person waits on: an
 * API read, a press's request, a film's bytes. A waited-on request is given a freed room before any
 * cosmetic one, and the cosmetic ones hold cosmeticRoomsPerConnection of each connection's rooms at
 * most.
 */
export type Rung = "waited-on" | "cosmetic";

/** The rungs, the highest first: the order the requests waiting are given rooms in. */
const rungsHighestFirst: readonly Rung[] = ["waited-on", "cosmetic"];

/**
 * Where a cosmetic request's path begins: an image's (artUrl, the service worker's loads of an
 * <img>), and the notes of what the person is looking at (PUT and DELETE /interest/tiles and
 * /interest/page), which tell the core what to ready and are never waited on.
 */
const cosmeticPaths: readonly string[] = [artPathPrefix, `${apiPrefix}/interest/`];

/** The rung the request for url waits on, read from its path. */
export function rungOf(url: string): Rung {
  const { pathname } = new URL(url);
  return cosmeticPaths.some((prefix) => pathname.startsWith(prefix)) ? "cosmetic" : "waited-on";
}

/**
 * A request waiting for room: its rung, and how it is given the room claimed for it, or told, with
 * none, to look again (the transport closed, or was refused for good).
 */
interface Waiter {
  readonly rung: Rung;
  readonly give: (link: Link | undefined) => void;
}

/**
 * Why the way to the core may work again (whenShownOrBack): its page was shown again ("shown"), or the
 * browser came back online ("back"); a mere show keeps a try under way (D484 §2). Or why the
 * connections up may no longer carry anything: the browser said its network changed ("changed",
 * navigator.connection's change event, onWayBack), and each is judged at once (networkChanged).
 */
export type WayBack = "shown" | "back" | "changed";

/**
 * How a transport keeps its connections: opening another, which gives up its try once signal aborts,
 * how many it keeps at most, hearing the browser come back online (the window's online event)
 * or its page shown again (connectRtc's onWayBack), which tries at once, returning the end of
 * hearing; and, where there is a page, the pace of its tries while the page is hidden (D482).
 */
export interface Growth {
  readonly open: (signal: AbortSignal) => Promise<PeerLike>;
  readonly size: number;
  readonly onOnline?: ((listener: (why?: WayBack) => void) => () => void) | undefined;
  readonly hidden?: HiddenPacing | undefined;
}

/**
 * A client's WebRTC transport to its core: up to `clientConnections` peer connections, each request
 * sent on the one with the fewest under way. With growth, it keeps one connection up, opening another
 * only while every one up carries growWhenEachCarries requests or a request waits for room, one at a
 * time, up to its size, and closing one beyond the last once it has carried nothing for
 * idleClosesAfterMs. It takes one that is gone out of use at once and opens another in its place when
 * it is still wanted, retrying each failure, a refusal included, and each try that takes longer than
 * openTryMostMs, after a wait that doubles up to replaceMostMs, one whose offer reached no server
 * (Unreached) after unreachedRetryMs, and at once when the browser comes back online, a try under
 * way then given up (plans/webrtc-transport.md W2d); while its page is hidden, and not needed as
 * when shown, a try at most every hiddenTryEveryMs, and none after hiddenTriesForMs hidden until it
 * is shown again (D482); a try refused for good
 * (RefusedForGood) ends the tries, and is told (onRefused). `fetch` is what the API client sends
 * requests with; while no connection is up, a request waits for one, and fails once one is refused
 * for good.
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
  /** When the try under way (trying) began, on nowMs's clock. */
  private triedAtMs = 0;
  /** Since when, and why, it has had no connection; undefined while it has one. */
  private without: Without | undefined;
  private readonly backListeners = new Set<() => void>();
  /** The try refused for good, after which none is made; undefined until one is. */
  private refusal: Error | undefined;
  private readonly refusedListeners = new Set<(refusal: Error) => void>();
  private readonly stopHearingOnline: () => void;
  /** The pace of its tries while its page is hidden (D482); undefined with no page. */
  private readonly hiddenPace: HiddenPace | undefined;
  private isClosed = false;
  /** The requests waiting for room, in the order they came. */
  private readonly waiting: Waiter[] = [];
  /** How many rooms the cosmetic rung's requests hold, across the connections. */
  private cosmeticUnderWay = 0;
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
    this.hiddenPace =
      growth?.hidden === undefined ? undefined : new HiddenPace(growth.hidden, nowMs);
    for (const peer of peers) {
      this.add(peer);
    }
    this.stopHearingOnline =
      growth?.onOnline?.((why) => {
        if (why === "changed") {
          this.networkChanged();
        } else {
          this.backOnline(why);
        }
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

  /**
   * Tells listener once the core refused a try to connect for good (RefusedForGood), at once when it
   * did already: no try is made after it, and the page tells its person how to go on. Returns its
   * removal.
   */
  onRefused(listener: (refusal: Error) => void): () => void {
    if (this.refusal !== undefined) {
      listener(this.refusal);
    }
    this.refusedListeners.add(listener);
    return () => {
      this.refusedListeners.delete(listener);
    };
  }

  /** How many connections are up. */
  get size(): number {
    return this.links.length;
  }

  /**
   * Sends a request to the core and resolves with its response. A request whose signal has aborted
   * fails with its reason, as fetch does: at once and opening nothing when it aborted before, out of
   * the queue when it aborts waiting for room, and with its channel closed once it has one. It waits
   * for room on its rung (rungOf), the waited-on given rooms before the cosmetic, first-come within
   * each.
   */
  readonly fetch = async (request: Request): Promise<Response> => {
    const { signal, url } = request;
    if (signal.aborted) {
      throw abortReason(signal);
    }
    const rung = rungOf(url);
    // Chosen at once while one has room, so requests made together spread across the connections. A
    // request waits only while there is no room it may take, so one taken here passes none of its
    // rung or above.
    const link = this.claimNow(rung) ?? (await this.claim(signal, rung));
    this.grow();
    let opened: ChannelLike;
    try {
      opened = link.peer.createDataChannel("request");
    } catch (problem: unknown) {
      // A connection that opens no channel carries nothing more: it is replaced, and its room given back.
      link.underWay -= 1;
      this.rungGivenBack(rung);
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
        this.rungGivenBack(rung);
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
        this.silent(link, { url, stream: channel.id });
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
   * The request for url heard nothing for silentAtMostMs on link, on the channel of SCTP stream
   * stream: remembered for carrierOf, and link, when nothing of any request came over it for as long,
   * taken as gone and replaced, its other requests failing with it. A connection whose data channels
   * stopped while it stayed connected otherwise carried every request after them into the same
   * silence (the owner's Chrome, throttled and unthrottled, staging, 2026-10-29). The stream is named
   * so the core's log of the same channel can be found: a playback's answer whose head came and
   * whose body never did said nothing of where its bytes stopped (staging, 2026-10-30).
   */
  private silent(
    link: Link,
    request: { readonly url: string; readonly stream: number | null | undefined },
  ): void {
    const { url, stream } = request;
    const nowMs = this.nowMs();
    const sinceDeliveredMs =
      link.deliveredAtMs === undefined ? undefined : nowMs - link.deliveredAtMs;
    const isLinkSilent = sinceDeliveredMs === undefined || sinceDeliveredMs >= silentAtMostMs;
    const onStream = typeof stream === "number" ? ` on stream ${String(stream)}` : "";
    this.ended.delete(url);
    this.ended.set(url, {
      connection: link.number,
      upMs: nowMs - link.upSinceMs,
      underWay: link.underWay,
      sinceDeliveredMs,
      ended: `nothing came for ${String(silentAtMostMs)} ms${onStream}${isLinkSilent ? ", nor over its connection, which was replaced" : ""}`,
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
    this.hiddenPace?.stop();
    clearTimeout(this.retryTimer);
    this.trying?.abort();
    for (const link of this.links.splice(0)) {
      link.stopIdle();
      link.peer.close();
    }
    // A request waiting for a connection fails now rather than waiting for good.
    this.wakeWaiting();
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
    // One back from disconnected takes the requests that waited while it could carry none.
    peer.onJudged?.(() => {
      if (this.links.includes(link)) {
        this.wakeWaiting();
      }
    });
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
    if (
      this.isClosed ||
      this.refusal !== undefined ||
      this.opening > 0 ||
      this.links.length >= growth.size
    ) {
      return false;
    }
    return (
      this.links.length === 0 ||
      this.waiting.length > 0 ||
      this.links.every((link) => link.underWay >= growWhenEachCarries)
    );
  }

  /**
   * Opens another connection when one is wanted, unless its page is hidden and its pace holds the
   * try back (D482): then it looks again once the pace allows one, or, past hiddenTriesForMs
   * hidden, when the page is shown again (backOnline, through onWayBack).
   */
  private grow(): void {
    const growth = this.growth;
    if (growth === undefined || !this.wantsAnother(growth)) {
      return;
    }
    const heldMs = this.hiddenPace?.waitMs() ?? 0;
    if (heldMs > 0) {
      if (Number.isFinite(heldMs)) {
        this.retryLater(heldMs);
      }
      return;
    }
    this.opening += 1;
    this.hiddenPace?.tried();
    void this.openOne(growth);
  }

  private async openOne(growth: Growth): Promise<void> {
    const trying = new AbortController();
    this.trying = trying;
    this.triedAtMs = this.nowMs();
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
      if (isRefusedForGood(problem)) {
        this.refuse(problem instanceof Error ? problem : new Error(String(problem)));
      } else if (trying.signal.reason instanceof BackOnline) {
        // The browser came back online: tried anew at once, the wait already started over.
        this.grow();
      } else if (isUnreached(problem)) {
        // Its offer reached no server: the network is away, and the next try, soon, is how the page
        // hears it back where the browser says nothing (Firefox fires no online event); the growing
        // wait starts over, as on online.
        this.retryMs = replaceFirstMs;
        this.retryLater(unreachedRetryMs);
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

  /**
   * Ends the tries for good: the core refused what the page proves its offers by. The core's log is
   * told when any path reaches it (the discovery logs each refusal itself), the requests waiting fail,
   * and each onRefused listener is told.
   */
  private refuse(refusal: Error): void {
    if (this.refusal !== undefined) {
      return;
    }
    this.refusal = refusal;
    reportFailure(
      new Error(
        `the core refused what this page proves its WebRTC connections by, and none is tried again: ${refusal.message}`,
      ),
    );
    this.wakeWaiting();
    for (const listener of this.refusedListeners) {
      listener(refusal);
    }
  }

  /**
   * Looks again whether another connection is wanted after a wait: the one given, else one longer
   * each time up to the most.
   */
  private retryLater(given?: number): void {
    if (this.isClosed || this.retryTimer !== undefined) {
      return;
    }
    const wait = given ?? this.retryMs;
    if (given === undefined) {
      this.retryMs = Math.min(this.retryMs * 2, replaceMostMs);
    }
    this.opening += 1;
    this.retryTimer = setTimeout(() => {
      this.retryTimer = undefined;
      this.opening -= 1;
      this.grow();
    }, wait);
  }

  /**
   * The browser came back online, or its page was shown again (why, "back" where not told): a try
   * under way, begun on the network that went, is given up and one made anew; a wait under way is
   * cut short, and the wait starts over from replaceFirstMs; the session connection is told (onBack).
   * A mere show keeps a try under way that has not run past its own deadline (openTryMostMs) by the
   * clock (D484 §2): on flapping mobile data a page shown more often than one 6–8 s connect takes
   * would never connect (the slow-link war game, 2026-11-14), while one a frozen page held past its
   * deadline, its timer not yet run, is given up as on online (the owner's phone, staging, 2026-11-14).
   */
  private backOnline(why: "shown" | "back" = "back"): void {
    if (this.isClosed || this.refusal !== undefined) {
      return;
    }
    const isTryKept = why === "shown" && this.nowMs() - this.triedAtMs < openTryMostMs;
    this.retryMs = replaceFirstMs;
    if (this.retryTimer !== undefined) {
      clearTimeout(this.retryTimer);
      this.retryTimer = undefined;
      this.opening -= 1;
    }
    // Its failure makes the next try at once (openOne).
    if (!isTryKept) {
      this.trying?.abort(new BackOnline());
    }
    this.grow();
    this.tellBack();
  }

  /**
   * The browser said its network changed: each connection up is judged at once (PeerLike's
   * networkChanged), so one the change disconnected is gone, and replaced, without waiting out its
   * grace, and one still connected carries on as it was; tries and waits under way are left as they
   * are, for the change may be a mere estimate of a network that still works (plan item 10, D).
   */
  private networkChanged(): void {
    if (this.isClosed || this.refusal !== undefined) {
      return;
    }
    for (const link of [...this.links]) {
      link.peer.networkChanged?.();
    }
  }

  /** Tells each onBack listener that the way to the core may work again. */
  private tellBack(): void {
    for (const listener of this.backListeners) {
      listener();
    }
  }

  /**
   * Gives the rooms free to the requests waiting, the highest rung first and first-come within each
   * (D479): a freed room goes to a waiting read, never to a poster that came before it. Each room is
   * claimed for its request here, in the same step as it is found, so one freed room lets one request
   * through: woken together to look again, each saw the same room free, and one freed room sent the
   * whole queue past the core's 16 on one connection, which refused them (staging, 2026-10-20). Once
   * the transport is closed or refused for good, every request waiting is told, to fail.
   */
  private wakeWaiting(): void {
    if (this.isClosed || this.refusal !== undefined) {
      for (const waiter of this.waiting.splice(0)) {
        waiter.give(undefined);
      }
      return;
    }
    for (const rung of rungsHighestFirst) {
      for (const waiter of this.waiting.filter((each) => each.rung === rung)) {
        const link = this.claimNow(rung);
        if (link === undefined) {
          break;
        }
        this.waiting.splice(this.waiting.indexOf(waiter), 1);
        waiter.give(link);
      }
    }
  }

  /**
   * A room for a request of rung on the connection with the fewest under way, taken at once, when one
   * is up, can carry it (PeerLike.canCarry) and has room, and, for a cosmetic one, while the cosmetic
   * hold fewer than cosmeticRoomsPerConnection for each connection up. One disconnected is passed by,
   * and with none other the request waits, as with none up, for one back or new: a phone whose network
   * changed had all four disconnected together, each held for disconnectedGraceMs, and a read asked
   * again three times failed on them at once, before a new one was up (the owner's phone, production,
   * 2026-11-26).
   */
  private claimNow(rung: Rung): Link | undefined {
    if (
      rung === "cosmetic" &&
      this.cosmeticUnderWay >= cosmeticRoomsPerConnection * this.links.length
    ) {
      return undefined;
    }
    const link = fewestUnderWay(
      this.links.filter(
        (each) => each.underWay < requestsPerConnection && (each.peer.canCarry?.() ?? true),
      ),
    );
    if (link !== undefined) {
      link.underWay += 1;
      link.stopIdle();
      if (rung === "cosmetic") {
        this.cosmeticUnderWay += 1;
      }
    }
    return link;
  }

  /** Gives back to its rung's count the room a request of rung held; its connection's is given back beside. */
  private rungGivenBack(rung: Rung): void {
    if (rung === "cosmetic") {
      this.cosmeticUnderWay -= 1;
    }
  }

  /**
   * A room for a request of rung, taken at once, else given it as one frees (wakeWaiting), waiting
   * while none is up or has room it may take, until signal aborts: the request then leaves the queue
   * and fails with its reason.
   */
  private async claim(signal: AbortSignal, rung: Rung): Promise<Link> {
    for (;;) {
      const link = this.claimNow(rung);
      if (link !== undefined) {
        return link;
      }
      if (this.isClosed) {
        throw new Error("the WebRTC transport is closed");
      }
      if (this.refusal !== undefined) {
        throw this.refusal;
      }
      if (signal.aborted) {
        throw abortReason(signal);
      }
      const given = await new Promise<Link | undefined>((resolve, reject) => {
        const waiter: Waiter = {
          rung,
          give: (room) => {
            signal.removeEventListener("abort", aborted);
            resolve(room);
          },
        };
        const aborted = (): void => {
          const index = this.waiting.indexOf(waiter);
          if (index >= 0) {
            this.waiting.splice(index, 1);
          }
          reject(abortReason(signal));
        };
        this.waiting.push(waiter);
        signal.addEventListener("abort", aborted, { once: true });
        this.grow();
      });
      if (given !== undefined) {
        return given;
      }
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
