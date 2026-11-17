// SPDX-License-Identifier: MPL-2.0

/**
 * The chunks a film waiting for data waits on, as they come over its link, for the page's bar under
 * its spinner (the owner, 2026-11-17). The core holds a chunk's request without a word while its
 * node makes the chunk (chunks.md §3), so a chunk whose answer's head has not come is the server's
 * to prepare, and one whose head came is the link's to carry: its head says its length
 * (Content-Length), and its bytes loaded say how much of it has come. A wait is watched from when
 * the page's spinner shows: the picture and sound chunks then on their way and each asked for after,
 * a start's second picture chunk among them (D502 §2), each kept once it has come, until the wait
 * ends; one that failed, timed out or was given up is waited on no more, and the chunk asked again
 * in its place is a load of its own. A chunk asked again exists, its head having come before, so its
 * wait is the link's from that first head on, its head come or not: "retries are not the server's
 * fault" (D506 §1).
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
  /** Bytes the wait's chunks are, as their heads say (Content-Length). */
  readonly neededBytes: number;
  /**
   * How long since the last head of the chunks on their way came, in ms: since the wait became the
   * link's to carry, every chunk it waits on begun by the server (the owner, 2026-11-17). A new
   * chunk, its head not come, makes the wait the server's again, and its head starts this anew; a
   * chunk asked again counts from its first head (D506 §1).
   */
  readonly sinceHeadMs: number;
}

/** A wait watched: how its chunks come, and the end of the watching. */
export interface LinkWaitWatch {
  /**
   * How the wait's chunks come; undefined while none is on its way, while one has no head yet (the
   * core still makes it), or while one's head said no length.
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

/** How much of loads has come: bytes come and needed, and the latest head, -Infinity for none. */
interface Arrival {
  readonly arrivedBytes: number;
  readonly neededBytes: number;
  readonly headAtMs: number;
}

/** A chunk's first head: when it came, and the length it said. */
interface Head {
  readonly atMs: number;
  readonly length: number;
}

/** load's head, where it came and said a length; undefined otherwise. */
function headOf(load: FollowedLoad): Head | undefined {
  const atMs = load.loader.stats.loading.first;
  const length = lengthOf(load.loader);
  return atMs <= 0 || length === undefined ? undefined : { atMs, length };
}

/** How many chunks' first heads a playback keeps, the latest, for the chunks asked again. */
export const headsKept = 32;

/**
 * How much of load has come; undefined where its head has not come, or said no length. firstHead is
 * its chunk's head by an earlier load, where one came: a chunk asked again, whose wait counts from
 * that head, its own come or not (D506 §1).
 */
function arrivalOf(load: FollowedLoad, firstHead: Head | undefined): Arrival | undefined {
  const { arrivedBytes } = load;
  if (arrivedBytes !== undefined) {
    return { arrivedBytes, neededBytes: arrivedBytes, headAtMs: Number.NEGATIVE_INFINITY };
  }
  const head = headOf(load);
  const length = head?.length ?? firstHead?.length;
  const headAtMs = firstHead?.atMs ?? head?.atMs;
  return length === undefined || headAtMs === undefined
    ? undefined
    : { arrivedBytes: Math.min(load.loader.stats.loaded, length), neededBytes: length, headAtMs };
}

/** How loads come at nowMs, as LinkWaitWatch.read says it, firstHeadOf their chunks' first heads. */
function linkWaitOf(
  loads: Iterable<FollowedLoad>,
  firstHeadOf: (load: FollowedLoad) => Head | undefined,
  nowMs: number,
): LinkWait | undefined {
  let sums: Arrival = { arrivedBytes: 0, neededBytes: 0, headAtMs: Number.NEGATIVE_INFINITY };
  for (const load of loads) {
    const arrival = arrivalOf(load, firstHeadOf(load));
    if (arrival === undefined) {
      return undefined;
    }
    sums = {
      arrivedBytes: sums.arrivedBytes + arrival.arrivedBytes,
      neededBytes: sums.neededBytes + arrival.neededBytes,
      headAtMs: Math.max(sums.headAtMs, arrival.headAtMs),
    };
  }
  const { arrivedBytes, neededBytes, headAtMs } = sums;
  return Number.isFinite(headAtMs)
    ? { arrivedBytes, neededBytes, sinceHeadMs: nowMs - headAtMs }
    : undefined;
}

/** A playback's chunk loads on their way, and the waits watched over them (watch). */
export class LinkWaits {
  readonly #onTheirWay = new Set<FollowedLoad>();
  readonly #watched = new Set<Set<FollowedLoad>>();
  /** Each chunk's first head, by its URL, the latest headsKept. */
  readonly #heads = new Map<string, Head>();
  readonly #nowMs: () => number;

  /** nowMs is the clock hls.js times its loads by, performance.now's. */
  constructor(nowMs: () => number) {
    this.#nowMs = nowMs;
  }

  /** Watches a wait from now: the chunks on their way, and each asked for until it is stopped. */
  watch(): LinkWaitWatch {
    const loads = new Set(this.#onTheirWay);
    this.#watched.add(loads);
    return {
      read: () => linkWaitOf(loads, (load) => this.#firstHeadOf(load), this.#nowMs()),
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
   * load ended: all of it come, kept by each wait with its bytes, where isArrived; else waited on no
   * more. A load ended once is ended.
   */
  ended(load: FollowedLoad, isArrived: boolean): void {
    if (!this.#onTheirWay.delete(load)) {
      return;
    }
    this.#firstHeadOf(load);
    if (isArrived) {
      load.arrived();
      return;
    }
    for (const loads of this.#watched) {
      loads.delete(load);
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

/** hls.js's callbacks for a load, told first, by ended, whether all of it came. */
function endingCallbacks(
  callbacks: LoaderCallbacks<LoaderContext>,
  ended: (isArrived: boolean) => void,
): LoaderCallbacks<LoaderContext> {
  const { onAbort } = callbacks;
  return {
    ...callbacks,
    // eslint-disable-next-line max-params -- hls.js's LoaderOnSuccess
    onSuccess: (response, stats, context, networkDetails) => {
      ended(true);
      callbacks.onSuccess(response, stats, context, networkDetails);
    },
    // eslint-disable-next-line max-params -- hls.js's LoaderOnError
    onError: (error, context, networkDetails, stats) => {
      ended(false);
      callbacks.onError(error, context, networkDetails, stats);
    },
    onTimeout: (stats, context, networkDetails) => {
      ended(false);
      callbacks.onTimeout(stats, context, networkDetails);
    },
    ...(onAbort === undefined
      ? {}
      : {
          onAbort: (stats, context, networkDetails) => {
            ended(false);
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
    this.#ended(false);
    if (!isWaitedOn(context)) {
      this.#inner.load(context, config, callbacks);
      return;
    }
    const load = this.#waits.began(context.url, this.#inner);
    this.#load = load;
    this.#inner.load(
      context,
      config,
      endingCallbacks(callbacks, (isArrived) => {
        this.#waits.ended(load, isArrived);
      }),
    );
  }

  abort(): void {
    this.#ended(false);
    this.#inner.abort();
  }

  destroy(): void {
    this.#ended(false);
    this.#inner.destroy();
  }

  /** Ends the load followed, if one is: one all come is kept as it came. */
  #ended(isArrived: boolean): void {
    const load = this.#load;
    this.#load = undefined;
    if (load !== undefined) {
      this.#waits.ended(load, isArrived);
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
