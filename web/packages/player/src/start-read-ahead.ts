// SPDX-License-Identifier: MPL-2.0

/**
 * A playback's start asks for its second picture chunk beside its first (D502 §2). hls.js loads one
 * chunk of each kind at a time, and asks for the next only once the last is appended, so a start
 * whose gate waits for more than its first chunk waited on one chunk after another: a Fire TV's
 * chunk 1 was asked 115–200 ms after chunk 0 arrived, and its first frame came only once chunk 1
 * was in, 0.8–2.3 s after chunk 0 (start-chain-2026-11-16.md). Here the first picture chunk hls.js
 * asks for starts the load of the chunk after it on the same quality, and hls.js's own ask for
 * that one, when it comes next, takes the load under way, or its answer kept, rather than asking
 * again. hls.js 1.7.3 has no setting that loads two chunks of a kind at once, so the read-ahead is
 * the fragment loader's (StartReadAhead.loaderOf).
 */

import { PlaylistLevelType } from "hls.js";
import type {
  Fragment,
  HlsConfig,
  Loader,
  LoaderCallbacks,
  LoaderConfiguration,
  LoaderContext,
  LoaderResponse,
  LoaderStats,
} from "hls.js";

import type { LoaderClass } from "./link-shaping.ts";

/** What hls.js tells a load's caller of its network, passed on as it is. */
type NetworkDetails = Parameters<LoaderCallbacks<LoaderContext>["onSuccess"]>[3];

/** A load's context, as hls.js's fragment loader gives it: with the chunk it loads. */
type ChunkContext = LoaderContext & { readonly frag?: Fragment };

/** The qualities hls.js holds, as nextChunkOn reads them. */
interface HeldLevels {
  readonly levels: readonly {
    readonly details?: { readonly fragments: readonly Fragment[] } | undefined;
  }[];
}

/** The chunk after frag on its quality in hls's playlists, none after the last or where none is held. */
export function nextChunkOn(hls: HeldLevels, frag: Fragment): Fragment | undefined {
  const { sn } = frag;
  if (typeof sn !== "number") {
    return undefined;
  }
  return hls.levels[frag.level]?.details?.fragments.find((each) => each.sn === sn + 1);
}

/** The picture chunk a load is of, none for a sound's, an init's or a playlist's. */
function pictureChunkOf(context: ChunkContext): Fragment | undefined {
  const { frag } = context;
  return frag?.type === PlaylistLevelType.MAIN && typeof frag.sn === "number" ? frag : undefined;
}

/** Whether a load asks for a byte range of its file: the core's chunks are whole files, never read ahead so. */
function isRange(context: LoaderContext): boolean {
  return (context.rangeStart ?? 0) !== 0 || (context.rangeEnd ?? 0) !== 0;
}

/** hls.js's load that took a read-ahead over: told what the read-ahead hears from then on. */
interface Taker {
  readonly onSuccess: (response: LoaderResponse, networkDetails: NetworkDetails) => void;
  readonly onError: (error: { code: number; text: string }, networkDetails: NetworkDetails) => void;
  readonly onTimeout: (networkDetails: NetworkDetails) => void;
  readonly onProgress: (data: string | ArrayBuffer, networkDetails: NetworkDetails) => void;
}

/** A chunk loading ahead of hls.js's ask, and its end while no ask has taken it. */
interface AheadLoad {
  readonly url: string;
  readonly loader: Loader<LoaderContext>;
  ended?: { readonly response: LoaderResponse; readonly networkDetails: NetworkDetails } | "failed";
  taker?: Taker | undefined;
}

/** An ask of a chunk, as hls.js makes it of a loader. */
interface Ask {
  readonly context: ChunkContext;
  readonly config: LoaderConfiguration;
  readonly callbacks: LoaderCallbacks<LoaderContext>;
}

/** stats as from says them, its count of retries kept: hls.js's frag.stats is the object a load began with. */
function copyStats(into: LoaderStats, from: LoaderStats): void {
  Object.assign(into, {
    aborted: from.aborted,
    loaded: from.loaded,
    total: from.total,
    chunkCount: from.chunkCount,
    bwEstimate: from.bwEstimate,
  });
  Object.assign(into.loading, from.loading);
  Object.assign(into.parsing, from.parsing);
  Object.assign(into.buffering, from.buffering);
}

/** A loader of hls.js's chunks by its carrier, its start's first picture chunk reading the next ahead (StartReadAhead). */
class ReadingAheadLoader implements Loader<LoaderContext> {
  context: LoaderContext | null = null;

  private readonly inner: Loader<LoaderContext>;
  private taken: AheadLoad | undefined;
  private takenCallbacks: LoaderCallbacks<LoaderContext> | undefined;

  constructor(
    private readonly config: HlsConfig,
    private readonly reading: StartReadAhead,
    private readonly carrier: LoaderClass,
  ) {
    this.inner = new carrier(config);
  }

  get stats(): LoaderStats {
    return this.inner.stats;
  }

  destroy(): void {
    this.taken?.loader.destroy();
    this.inner.destroy();
  }

  abort(): void {
    const { taken, takenCallbacks, context } = this;
    if (taken === undefined) {
      this.inner.abort();
      return;
    }
    this.taken = undefined;
    taken.taker = undefined;
    taken.loader.abort();
    this.stats.aborted = true;
    if (context !== null) {
      takenCallbacks?.onAbort?.(this.stats, context, null);
    }
  }

  getCacheAge(): number | null {
    return (this.taken?.loader ?? this.inner).getCacheAge?.() ?? null;
  }

  getResponseHeader(name: string): string | null {
    return (this.taken?.loader ?? this.inner).getResponseHeader?.(name) ?? null;
  }

  load(
    context: ChunkContext,
    config: LoaderConfiguration,
    callbacks: LoaderCallbacks<LoaderContext>,
  ): void {
    this.context = context;
    const frag = pictureChunkOf(context);
    const taken = frag === undefined ? undefined : this.reading.take(context.url);
    if (taken !== undefined && taken.ended !== "failed") {
      this.takeOver(taken, { context, config, callbacks });
      return;
    }
    // A read-ahead failed (a chunk not made yet, 425, or a load lost) is asked again, with hls.js's retries.
    this.inner.load(context, config, callbacks);
    if (frag !== undefined && !isRange(context)) {
      this.reading.startAfter(
        frag,
        { context, config, callbacks },
        () => new this.carrier(this.config),
      );
    }
  }

  /** Answers ask from taken: its answer, kept, or what it hears from now on. */
  private takeOver(taken: AheadLoad, ask: Ask): void {
    const { context, callbacks } = ask;
    const own = this.stats;
    this.taken = taken;
    this.takenCallbacks = callbacks;
    const told = (): boolean => {
      copyStats(own, taken.loader.stats);
      const isStill = this.taken === taken;
      this.taken = undefined;
      return isStill;
    };
    const { ended } = taken;
    if (ended !== undefined && ended !== "failed") {
      // Answered after this ask returns, as a load is.
      queueMicrotask(() => {
        if (told()) {
          callbacks.onSuccess(ended.response, own, context, ended.networkDetails);
        }
      });
      return;
    }
    // eslint-disable-next-line no-param-reassign -- the read-ahead tells its taker from now on
    taken.taker = {
      onSuccess: (response, details) => {
        if (told()) {
          callbacks.onSuccess(response, own, context, details);
        }
      },
      onError: (error, details) => {
        if (told()) {
          callbacks.onError(error, context, details, own);
        }
      },
      onTimeout: (details) => {
        if (told()) {
          callbacks.onTimeout(own, context, details);
        }
      },
      onProgress: (data, details) => {
        copyStats(own, taken.loader.stats);
        callbacks.onProgress?.(own, context, data, details);
      },
    };
  }
}

/**
 * One playback's read-ahead at its start: the first picture chunk asked of its loaders (loaderOf)
 * loads the next ahead, once; the next picture chunk asked takes it when it is that one, and
 * gives it up when it is another, a quality chosen anew or a seek.
 */
export class StartReadAhead {
  private isStarted = false;
  private ahead: AheadLoad | undefined;
  private nextOf: (frag: Fragment) => Fragment | undefined = () => undefined;

  /** Reads the chunk after each start's first from what hls holds. */
  readsOn(hls: HeldLevels): void {
    this.nextOf = (frag) => nextChunkOn(hls, frag);
  }

  /** Gives up a read-ahead no ask took; nothing is read ahead after. */
  readonly stop = (): void => {
    this.isStarted = true;
    this.dropAhead();
  };

  /** The class of loader that loads by carrier, its start's first picture chunk reading the next ahead. */
  loaderOf(carrier: LoaderClass): LoaderClass {
    return loaderReadingBy(this, carrier);
  }

  /** The read-ahead of url, for an ask of it; one of another chunk is given up. */
  take(url: string): AheadLoad | undefined {
    const { ahead } = this;
    if (ahead?.url === url) {
      this.ahead = undefined;
      return ahead;
    }
    this.dropAhead();
    return undefined;
  }

  /** Loads the chunk after frag ahead, by a loader newLoader makes, as ask loads frag: at the start's first ask alone. */
  startAfter(frag: Fragment, ask: Ask, newLoader: () => Loader<LoaderContext>): void {
    if (this.isStarted) {
      return;
    }
    this.isStarted = true;
    const next = this.nextOf(frag);
    if (next === undefined) {
      return;
    }
    const ahead: AheadLoad = { url: next.url, loader: newLoader() };
    this.ahead = ahead;
    const aheadContext: ChunkContext = { ...ask.context, url: next.url, frag: next };
    const endedFailed = (): void => {
      ahead.ended = "failed";
    };
    ahead.loader.load(aheadContext, ask.config, {
      // eslint-disable-next-line max-params -- hls.js's LoaderOnSuccess
      onSuccess: (response, _stats, _context, details) => {
        if (ahead.taker === undefined) {
          ahead.ended = { response, networkDetails: details };
        } else {
          ahead.taker.onSuccess(response, details);
        }
      },
      onError: (error, _context, details) => {
        (ahead.taker?.onError ?? endedFailed)(error, details);
      },
      onTimeout: (_stats, _context, details) => {
        (ahead.taker?.onTimeout ?? endedFailed)(details);
      },
      // eslint-disable-next-line max-params -- hls.js's LoaderOnProgress
      onProgress: (_stats, _context, data, details) => {
        ahead.taker?.onProgress(data, details);
      },
    });
  }

  private dropAhead(): void {
    const { ahead } = this;
    this.ahead = undefined;
    if (ahead !== undefined) {
      ahead.loader.abort();
      ahead.loader.destroy();
    }
  }
}

/** The class of loader that loads by carrier, reading its start's second picture chunk ahead by reading. */
function loaderReadingBy(reading: StartReadAhead, carrier: LoaderClass): LoaderClass {
  return class extends ReadingAheadLoader {
    constructor(config: HlsConfig) {
      super(config, reading, carrier);
    }
  };
}
