// SPDX-License-Identifier: MPL-2.0

/**
 * The chunks a film waiting for data waits on, as they come over its link, for the page's bar under
 * its spinner (the owner, 2026-11-17). The core answers a chunk's request at once (chunks.md §3):
 * with its head, saying its length (Content-Length), where the chunk is made, or with 425 where its
 * node is still making it. So a wait is the server's only while a chunk of it was last answered 425,
 * and the link's otherwise, a request no answer of which has come yet among it: that answer is small
 * and immediate, so one not come is the network's trouble (the owner, 2026-11-17; D506 §4). A chunk
 * answered 425 is the server's until its head comes, or until makingLapsesAfterMs passes with no 425
 * again: hls.js asks again every second or two, and a chunk not asked again was given up. A wait is
 * watched from when the page's spinner shows: the picture and sound chunks then on their way and each
 * asked for after, a start's second picture chunk among them (D502 §2), each kept once it has come,
 * until the wait ends; one that failed, timed out or was given up is waited on no more, and the
 * chunk asked again in its place is a load of its own, counted against the length its chunk's first
 * head said (D506 §1).
 */

import { PlaylistLevelType } from "hls.js";
import type {
  Fragment,
  HlsConfig,
  Loader,
  LoaderCallbacks,
  LoaderConfiguration,
  LoaderContext,
  LoaderStats,
} from "hls.js";

import type { LoaderClass } from "./link-shaping.ts";

/** A wait's chunks as they come: their bytes come and needed, and how long since a head came. */
export interface LinkWait {
  /** Bytes of the wait's chunks come so far. */
  readonly arrivedBytes: number;
  /**
   * Bytes the wait's chunks are, as their heads say (Content-Length): none for a chunk no head of
   * which has come yet, nor said its length.
   */
  readonly neededBytes: number;
  /**
   * How long the wait has been the link's, in ms: since it was watched, or since the server last made
   * a chunk of it, that chunk's head come or its 425 lapsed (D506 §4).
   */
  readonly linksForMs: number;
}

/** A wait watched: how its chunks come, and the end of the watching. */
export interface LinkWaitWatch {
  /**
   * How the wait's chunks come; undefined while the server makes one of them, its last answer a 425
   * (D506 §4).
   */
  readonly read: () => LinkWait | undefined;
  readonly stop: () => void;
}

/** A watch of no wait, for a playback whose loads no loader of ours makes: the element's own HLS. */
export const noLinkWaitWatch: LinkWaitWatch = { read: () => undefined, stop: () => undefined };

/** A loader as a load followed reads it: its stats, and its answer's head. */
type FollowedLoader = Pick<Loader<LoaderContext>, "stats" | "getResponseHeader">;

/** One chunk's load followed: its URL, its loader, and its bytes once all of it came. */
class FollowedLoad {
  readonly url: string;
  readonly loader: FollowedLoader;
  #arrivedBytes: number | undefined;

  constructor(url: string, loader: FollowedLoader) {
    this.url = url;
    this.loader = loader;
  }

  /** Its bytes, once all of it came; undefined while it is on its way. */
  get arrivedBytes(): number | undefined {
    return this.#arrivedBytes;
  }

  /** All of it came: its bytes are kept as they came, whatever its loader loads after. */
  arrived(): void {
    this.#arrivedBytes = this.loader.stats.loaded;
  }
}

/** A load's context, as hls.js's fragment loader gives it: with the chunk it loads. */
type ChunkContext = LoaderContext & { readonly frag?: Fragment };

/** Whether a load is of a picture's or a sound's chunk (or its init), which a wait for data waits on. */
function isWaitedOn(context: ChunkContext): boolean {
  const type = context.frag?.type;
  return type === PlaylistLevelType.MAIN || type === PlaylistLevelType.AUDIO;
}

/**
 * The bytes a load's head says its chunk is: hls.js's total, once its progress or its fetch's head
 * said it, else the Content-Length its XMLHttpRequest holds, which hls.js reads only at its first
 * progress, and a link carrying none yet has made none; undefined where neither says it.
 */
function lengthOf(loader: FollowedLoader): number | undefined {
  if (loader.stats.total > 0) {
    return loader.stats.total;
  }
  const length = Number(loader.getResponseHeader?.("Content-Length") ?? "");
  return Number.isInteger(length) && length > 0 ? length : undefined;
}

/** How much of loads has come: bytes come and needed. */
interface Arrival {
  readonly arrivedBytes: number;
  readonly neededBytes: number;
}

/** A chunk's first head: when it came, and the length it said. */
interface Head {
  readonly atMs: number;
  readonly length: number;
}

/**
 * load's head, where it came and said a length; undefined otherwise, and for a 425's, which says
 * Retry-After: hls.js times its head as any other's, an instant before it fails the load.
 */
function headOf(load: FollowedLoad): Head | undefined {
  const atMs = load.loader.stats.loading.first;
  const length = lengthOf(load.loader);
  const isMaking = (load.loader.getResponseHeader?.("Retry-After") ?? null) !== null;
  return atMs <= 0 || length === undefined || isMaking ? undefined : { atMs, length };
}

/**
 * How long, in ms, a chunk the core answered 425 is the server's to make with no 425 again: past
 * hls.js's asking again every second or two, so a chunk given up, not asked again, holds no wait.
 */
export const makingLapsesAfterMs = 5000;

/** How a chunk's load ended: all of it come, the core still making it (425), or lost. */
type LoadEnd = "arrived" | "making" | "lost";

/** How many chunks' first heads a playback keeps, the latest, for the chunks asked again. */
export const headsKept = 32;

/**
 * How much of load has come; none where no head of its chunk has come, or said no length. firstHeadOf
 * says its chunk's head by an earlier load, where one came: a chunk asked again is counted against
 * that head's length, its own come or not (D506 §1). It is asked only of a load on its way: one all
 * come may have had its loader destroyed, whose stats hls.js then empties.
 */
function arrivalOf(
  load: FollowedLoad,
  firstHeadOf: (load: FollowedLoad) => Head | undefined,
): Arrival {
  const { arrivedBytes } = load;
  if (arrivedBytes !== undefined) {
    return { arrivedBytes, neededBytes: arrivedBytes };
  }
  const length = headOf(load)?.length ?? firstHeadOf(load)?.length;
  return length === undefined
    ? { arrivedBytes: 0, neededBytes: 0 }
    : { arrivedBytes: Math.min(load.loader.stats.loaded, length), neededBytes: length };
}

/** How much of loads has come, firstHeadOf their chunks' first heads. */
function arrivalsOf(
  loads: Iterable<FollowedLoad>,
  firstHeadOf: (load: FollowedLoad) => Head | undefined,
): Arrival {
  let sums: Arrival = { arrivedBytes: 0, neededBytes: 0 };
  for (const load of loads) {
    const arrival = arrivalOf(load, firstHeadOf);
    sums = {
      arrivedBytes: sums.arrivedBytes + arrival.arrivedBytes,
      neededBytes: sums.neededBytes + arrival.neededBytes,
    };
  }
  return sums;
}

/** A playback's chunk loads on their way, and the waits watched over them (watch). */
export class LinkWaits {
  readonly #onTheirWay = new Set<FollowedLoad>();
  readonly #watched = new Set<Set<FollowedLoad>>();
  /** Each chunk's first head, by its URL, the latest headsKept. */
  readonly #heads = new Map<string, Head>();
  /** The chunks the server makes, by URL: when it last answered one 425. */
  readonly #making = new Map<string, number>();
  /** When the server last made a chunk it was making: its head came, or its 425 lapsed. */
  #madeAtMs = Number.NEGATIVE_INFINITY;
  readonly #nowMs: () => number;

  /** nowMs is the clock hls.js times its loads by, performance.now's. */
  constructor(nowMs: () => number) {
    this.#nowMs = nowMs;
  }

  /** Watches a wait from now: the chunks on their way, and each asked for until it is stopped. */
  watch(): LinkWaitWatch {
    const loads = new Set(this.#onTheirWay);
    const sinceMs = this.#nowMs();
    this.#watched.add(loads);
    return {
      read: () => {
        const nowMs = this.#nowMs();
        if (this.#isMaking(nowMs)) {
          return undefined;
        }
        const arrival = arrivalsOf(loads, (load) => this.#firstHeadOf(load));
        return { ...arrival, linksForMs: nowMs - Math.max(sinceMs, this.#madeAtMs) };
      },
      stop: () => {
        this.#watched.delete(loads);
      },
    };
  }

  /** The class of loader that loads by carrier, each chunk's load followed for the waits watched. */
  loaderOf(carrier: LoaderClass): LoaderClass {
    return followingBy(this, carrier);
  }

  /** A load of url by loader began: followed until it ends. */
  began(url: string, loader: FollowedLoader): FollowedLoad {
    const load = new FollowedLoad(url, loader);
    this.#onTheirWay.add(load);
    for (const loads of this.#watched) {
      loads.add(load);
    }
    return load;
  }

  /**
   * load ended as end says: all of it come, kept by each wait with its bytes; else waited on no more,
   * its chunk the server's to make where the core answered 425. A load ended once is ended.
   */
  ended(load: FollowedLoad, end: LoadEnd): void {
    if (!this.#onTheirWay.delete(load)) {
      return;
    }
    if (end === "making") {
      this.#making.set(load.url, this.#nowMs());
    } else {
      this.#firstHeadOf(load);
      const headAtMs = headOf(load)?.atMs ?? (end === "arrived" ? this.#nowMs() : undefined);
      if (headAtMs !== undefined) {
        this.#made(load.url, headAtMs);
      }
    }
    if (end === "arrived") {
      load.arrived();
      return;
    }
    for (const loads of this.#watched) {
      loads.delete(load);
    }
  }

  /**
   * Whether the server makes a chunk at nowMs: one answered 425 whose head has not come since, on its
   * load asked again, and whose 425 has not lapsed (makingLapsesAfterMs).
   */
  #isMaking(nowMs: number): boolean {
    for (const [url, atMs] of this.#making) {
      if (nowMs - atMs >= makingLapsesAfterMs) {
        this.#made(url, atMs + makingLapsesAfterMs);
      }
    }
    for (const load of this.#onTheirWay) {
      const answeredAtMs = this.#making.get(load.url);
      const headAtMs = headOf(load)?.atMs;
      if (answeredAtMs !== undefined && headAtMs !== undefined && headAtMs > answeredAtMs) {
        this.#made(load.url, headAtMs);
      }
    }
    return this.#making.size > 0;
  }

  /** The server made url's chunk, which it was making, at atMs: its wait is the link's from then. */
  #made(url: string, atMs: number): void {
    if (this.#making.delete(url)) {
      this.#madeAtMs = Math.max(this.#madeAtMs, atMs);
    }
  }

  /**
   * The first head of load's chunk, by an earlier load of it; none where load's own is its first.
   * Kept from load's own head where none was, the latest headsKept.
   */
  #firstHeadOf(load: FollowedLoad): Head | undefined {
    const kept = this.#heads.get(load.url);
    if (kept !== undefined) {
      return kept.atMs < load.loader.stats.loading.first || headOf(load) === undefined
        ? kept
        : undefined;
    }
    const head = headOf(load);
    if (head !== undefined) {
      this.#heads.set(load.url, head);
      const [oldest] = this.#heads.keys();
      if (this.#heads.size > headsKept && oldest !== undefined) {
        this.#heads.delete(oldest);
      }
    }
    return undefined;
  }
}

/** hls.js's callbacks for a load, told first, by ended, how it ended. */
function endingCallbacks(
  callbacks: LoaderCallbacks<LoaderContext>,
  ended: (end: LoadEnd) => void,
): LoaderCallbacks<LoaderContext> {
  const { onAbort } = callbacks;
  return {
    ...callbacks,
    // eslint-disable-next-line max-params -- hls.js's LoaderOnSuccess
    onSuccess: (response, stats, context, networkDetails) => {
      ended("arrived");
      callbacks.onSuccess(response, stats, context, networkDetails);
    },
    // eslint-disable-next-line max-params -- hls.js's LoaderOnError
    onError: (error, context, networkDetails, stats) => {
      ended(error.code === 425 ? "making" : "lost");
      callbacks.onError(error, context, networkDetails, stats);
    },
    onTimeout: (stats, context, networkDetails) => {
      ended("lost");
      callbacks.onTimeout(stats, context, networkDetails);
    },
    ...(onAbort === undefined
      ? {}
      : {
          onAbort: (stats, context, networkDetails) => {
            ended("lost");
            onAbort(stats, context, networkDetails);
          },
        }),
  };
}

/** A loader by carrier, its picture and sound chunks' loads followed by waits (LinkWaits). */
class FollowingLoader implements Loader<LoaderContext> {
  context: LoaderContext | null = null;
  readonly #inner: Loader<LoaderContext>;
  readonly #waits: LinkWaits;
  #load: FollowedLoad | undefined;

  constructor(config: HlsConfig, waits: LinkWaits, carrier: LoaderClass) {
    this.#inner = new carrier(config);
    this.#waits = waits;
  }

  get stats(): LoaderStats {
    return this.#inner.stats;
  }

  getCacheAge(): number | null {
    return this.#inner.getCacheAge?.() ?? null;
  }

  getResponseHeader(name: string): string | null {
    return this.#inner.getResponseHeader?.(name) ?? null;
  }

  load(
    context: ChunkContext,
    config: LoaderConfiguration,
    callbacks: LoaderCallbacks<LoaderContext>,
  ): void {
    this.context = context;
    this.#ended();
    if (!isWaitedOn(context)) {
      this.#inner.load(context, config, callbacks);
      return;
    }
    const load = this.#waits.began(context.url, this.#inner);
    this.#load = load;
    this.#inner.load(
      context,
      config,
      endingCallbacks(callbacks, (end) => {
        this.#waits.ended(load, end);
      }),
    );
  }

  abort(): void {
    this.#ended();
    this.#inner.abort();
  }

  destroy(): void {
    this.#ended();
    this.#inner.destroy();
  }

  /** Ends the load followed, if one is, as lost: one all come was ended as it came. */
  #ended(): void {
    const load = this.#load;
    this.#load = undefined;
    if (load !== undefined) {
      this.#waits.ended(load, "lost");
    }
  }
}

/** The class of loader that loads by carrier, following its chunks' loads for waits. */
function followingBy(waits: LinkWaits, carrier: LoaderClass): LoaderClass {
  return class extends FollowingLoader {
    constructor(config: HlsConfig) {
      super(config, waits, carrier);
    }
  };
}
