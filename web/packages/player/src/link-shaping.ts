// SPDX-License-Identifier: MPL-2.0

/**
 * A debug tool's shaping of this tab's chunk loads (D380 §2, §3): a cap on the rate they arrive at,
 * shared by every load of the tab as one link is, and a drop, during which each fails as a lost
 * network's does. Applied in the player's chunk fetch, hls.js's loader, so every carrier, WebRTC and
 * HTTP alike, is capped, and the bitrate controller's estimate sees what a slow link gives: a load's
 * bytes are let through no faster than the cap, its stats saying them as they are let through, so the
 * controller watching a chunk on its way sees it come slowly, as it would. Chrome's DevTools
 * throttling never reached the WebRTC carrier (VQ128's run).
 */

import type {
  HlsConfig,
  Loader,
  LoaderCallbacks,
  LoaderConfiguration,
  LoaderContext,
  LoaderResponse,
  LoaderStats,
  NullableNetworkDetails,
} from "hls.js";

/** What shapes the tab's chunk loads: its cap, kbit/s, undefined for none, and whether it is dropped. */
export interface LinkShaping {
  readonly capKbps: () => number | undefined;
  readonly isDropped: () => boolean;
}

/** Nothing shapes the loads: the tab's own link, as it is. */
const unshaped: LinkShaping = { capKbps: () => undefined, isDropped: () => false };

/** The tab's shaping, which every chunked playback's loads read as each begins and arrives. */
let shaping: LinkShaping = unshaped;

/**
 * The tab's link as one, shared by its loads: until when, on the clock of `performance.now()`, the
 * bytes let through so far take it at the cap.
 */
const link = { busyUntilMs: 0 };

/** How often a held load's stats move on, in ms: as hls.js's own progress events would. */
export const shapedProgressEveryMs = 100;

/** Has every chunked playback of the tab shaped by given, as a debug tool sets it (D380). */
export function shapeChunkLoads(given: LinkShaping | undefined): void {
  shaping = given ?? unshaped;
  link.busyUntilMs = 0;
}

/** How long bytes take at capKbps, in ms. */
export function msAtCap(bytes: number, capKbps: number): number {
  return (bytes * 8) / capKbps;
}

type LoaderClass = new (config: HlsConfig) => Loader<LoaderContext>;

/**
 * A loader that lets what loaderClass loads through as the tab's shaping says: as it is with no cap;
 * no faster than the cap, its stats saying the bytes as they are let through, with one; and, while
 * the tab is dropped, failing each load as a lost network does (status 0), so hls.js and the
 * controller meet it as a real loss.
 */
export function shapedLoader(loaderClass: LoaderClass): LoaderClass {
  return class ShapedLoader implements Loader<LoaderContext> {
    context: LoaderContext | null = null;
    private readonly inner: Loader<LoaderContext>;
    private held: { readonly stop: () => void; readonly abort: () => void } | undefined;
    private readonly release = (): void => {
      this.held = undefined;
    };

    constructor(config: HlsConfig) {
      this.inner = new loaderClass(config);
    }

    get stats(): LoaderStats {
      return this.inner.stats;
    }

    destroy(): void {
      this.held?.stop();
      this.held = undefined;
      this.inner.destroy();
    }

    abort(): void {
      const held = this.held;
      this.held = undefined;
      if (held === undefined) {
        this.inner.abort();
      } else {
        held.abort();
      }
    }

    getCacheAge(): number | null {
      return this.inner.getCacheAge?.() ?? null;
    }

    getResponseHeader(name: string): string | null {
      return this.inner.getResponseHeader?.(name) ?? null;
    }

    load(
      context: LoaderContext,
      config: LoaderConfiguration,
      callbacks: LoaderCallbacks<LoaderContext>,
    ): void {
      this.context = context;
      if (shaping.isDropped()) {
        this.held = failAsDropped({ context, callbacks, stats: this.inner.stats }, this.release);
        return;
      }
      this.inner.load(context, config, {
        ...callbacks,
        // eslint-disable-next-line max-params -- hls.js's LoaderOnSuccess
        onSuccess: (response, stats, _context, details) => {
          const cap = shaping.capKbps();
          if (cap === undefined || cap <= 0) {
            callbacks.onSuccess(response, stats, context, details);
            return;
          }
          const arrived = { response, stats, context, details, callbacks, capKbps: cap };
          this.held = holdAtCap(arrived, this.release);
        },
      });
    }
  };
}

/** The error a load meets while the tab is dropped: a network's, which says no status. */
const droppedError = { code: 0, text: "the connection is dropped (a debug tool, D380)" };

/**
 * Fails a load begun while the tab is dropped, as a lost network fails it, a moment later; done is
 * told as it fails. Returns its stop, and its abort, which tells hls.js the load was aborted.
 */
function failAsDropped(
  load: {
    readonly context: LoaderContext;
    readonly callbacks: LoaderCallbacks<LoaderContext>;
    readonly stats: LoaderStats;
  },
  done: () => void,
): { readonly stop: () => void; readonly abort: () => void } {
  const { context, callbacks, stats } = load;
  const timer = setTimeout(() => {
    done();
    callbacks.onError(droppedError, context, null, stats);
  }, 0);
  return {
    stop: () => {
      clearTimeout(timer);
    },
    abort: () => {
      clearTimeout(timer);
      callbacks.onAbort?.(stats, context, null);
    },
  };
}

/** One load arrived, held to the cap before hls.js is told. */
interface Arrived {
  readonly response: LoaderResponse;
  readonly stats: LoaderStats;
  readonly context: LoaderContext;
  readonly details: NullableNetworkDetails;
  readonly callbacks: LoaderCallbacks<LoaderContext>;
  readonly capKbps: number;
}

/**
 * Holds an arrived load until the tab's link at the cap has carried its bytes after the bytes before
 * them, its stats meanwhile saying as many bytes as the cap has let through, its end when it is let
 * go; then tells hls.js. Returns the hold's stop, and its abort, which tells hls.js the load was
 * aborted. done is told once it is let go.
 */
function holdAtCap(
  arrived: Arrived,
  done: () => void,
): { readonly stop: () => void; readonly abort: () => void } {
  const { stats, capKbps, callbacks, context } = arrived;
  const bytes = stats.loaded;
  const nowMs = performance.now();
  const firstMs = stats.loading.first > 0 ? stats.loading.first : nowMs;
  const fromMs = Math.max(firstMs, link.busyUntilMs);
  const dueMs = fromMs + msAtCap(bytes, capKbps);
  link.busyUntilMs = dueMs;
  if (dueMs <= nowMs) {
    callbacks.onSuccess(arrived.response, stats, context, arrived.details);
    done();
    return { stop: () => undefined, abort: () => undefined };
  }
  const letThrough = (): void => {
    const atMs = performance.now();
    stats.loaded = Math.min(bytes, Math.max(0, Math.floor(((atMs - fromMs) * capKbps) / 8)));
  };
  letThrough();
  const progress = setInterval(letThrough, shapedProgressEveryMs);
  const due = setTimeout(() => {
    clearInterval(progress);
    stats.loaded = bytes;
    stats.loading.end = Math.max(stats.loading.end, performance.now());
    done();
    callbacks.onSuccess(arrived.response, stats, context, arrived.details);
  }, dueMs - nowMs);
  const stop = (): void => {
    clearInterval(progress);
    clearTimeout(due);
  };
  return {
    stop,
    abort: () => {
      stop();
      link.busyUntilMs = Math.min(link.busyUntilMs, performance.now());
      stats.aborted = true;
      callbacks.onAbort?.(stats, context, arrived.details);
    },
  };
}
