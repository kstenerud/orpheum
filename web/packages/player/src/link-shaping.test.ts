// SPDX-License-Identifier: MPL-2.0

import type {
  HlsConfig,
  Loader,
  LoaderCallbacks,
  LoaderConfiguration,
  LoaderContext,
  LoaderStats,
} from "hls.js";
import { LoaderContextType } from "hls.js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { shapeChunkLoads, shapedLoader, shapedProgressEveryMs } from "./link-shaping.ts";

/** A load's stats, as hls.js's own loaders keep them. */
function freshStats(): LoaderStats {
  return {
    aborted: false,
    loaded: 0,
    retry: 0,
    total: 0,
    chunkCount: 0,
    bwEstimate: 0,
    loading: { start: 0, first: 0, end: 0 },
    parsing: { start: 0, end: 0 },
    buffering: { start: 0, first: 0, end: 0 },
  };
}

/** The loads a stand-in loader was asked, each answered with its bytes when the test says. */
const asked: { readonly arrive: (bytes: number) => void; readonly aborted: () => boolean }[] = [];

/** A stand-in for hls.js's loader: each load arrives whole when the test says, at once. */
class ArrivingLoader implements Loader<LoaderContext> {
  context: LoaderContext | null = null;
  stats: LoaderStats = freshStats();
  #isAborted = false;

  destroy(): void {
    // nothing held
  }

  abort(): void {
    this.#isAborted = true;
  }

  load(
    context: LoaderContext,
    _config: LoaderConfiguration,
    callbacks: LoaderCallbacks<LoaderContext>,
  ): void {
    this.context = context;
    this.stats.loading.start = performance.now();
    asked.push({
      arrive: (bytes) => {
        this.stats.loading.first = performance.now();
        this.stats.loaded = bytes;
        this.stats.total = bytes;
        this.stats.loading.end = performance.now();
        callbacks.onSuccess(
          { url: context.url, data: new ArrayBuffer(bytes) },
          this.stats,
          context,
          null,
        );
      },
      aborted: () => this.#isAborted,
    });
  }
}

const Shaped = shapedLoader(ArrivingLoader);
/** A chunk's load, as the stand-in reads it: its address alone. */
const context: LoaderContext = {
  url: "/c/2.m4s",
  responseType: "arraybuffer",
  type: LoaderContextType.LEVEL,
};

/** A shaped load's callbacks, and what they were told. */
function told(): {
  readonly callbacks: LoaderCallbacks<LoaderContext>;
  readonly successes: number[];
  readonly errors: { code: number; text: string }[];
  readonly aborts: number[];
} {
  const successes: number[] = [];
  const errors: { code: number; text: string }[] = [];
  const aborts: number[] = [];
  return {
    successes,
    errors,
    aborts,
    callbacks: {
      onSuccess: () => successes.push(performance.now()),
      onError: (error) => errors.push(error),
      onTimeout: () => undefined,
      onAbort: () => aborts.push(performance.now()),
    },
  };
}

beforeEach(() => {
  vi.useFakeTimers({
    toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval", "performance"],
  });
  asked.length = 0;
});

afterEach(() => {
  shapeChunkLoads(undefined);
  vi.useRealTimers();
});

// Chrome's DevTools throttling never reached the WebRTC carrier (VQ128's run): a debug tool caps the
// tab's chunk loads in the player's own fetch, every carrier alike (D380 §2).
describe("a chunk load under the tab's debug cap (D380)", () => {
  it("lets a load through as it comes with no cap", () => {
    const load = new Shaped({} as HlsConfig);
    const { callbacks, successes } = told();
    load.load(context, {} as LoaderConfiguration, callbacks);
    asked[0]?.arrive(1_000_000);
    expect(successes).toHaveLength(1);
  });

  it("lets a load's bytes through no faster than the cap, its stats saying them as they come", () => {
    shapeChunkLoads({ capKbps: () => 1000, isDropped: () => false });
    const load = new Shaped({} as HlsConfig);
    const { callbacks, successes } = told();
    load.load(context, {} as LoaderConfiguration, callbacks);
    const startMs = performance.now();
    asked[0]?.arrive(250_000); // 2 Mbit at 1 Mb/s: two seconds
    expect(successes).toEqual([]);
    expect(load.stats.loaded).toBe(0);
    vi.advanceTimersByTime(1000);
    expect(load.stats.loaded).toBeGreaterThanOrEqual(125_000 - (shapedProgressEveryMs * 1000) / 8);
    expect(load.stats.loaded).toBeLessThanOrEqual(125_000);
    vi.advanceTimersByTime(1000);
    expect(successes).toEqual([startMs + 2000]);
    expect(load.stats.loaded).toBe(250_000);
    expect(load.stats.loading.end - load.stats.loading.first).toBe(2000);
  });

  it("carries one load after another at the cap, as one link carries them", () => {
    shapeChunkLoads({ capKbps: () => 1000, isDropped: () => false });
    const first = told();
    const second = told();
    new Shaped({} as HlsConfig).load(context, {} as LoaderConfiguration, first.callbacks);
    new Shaped({} as HlsConfig).load(context, {} as LoaderConfiguration, second.callbacks);
    const startMs = performance.now();
    asked[0]?.arrive(125_000);
    asked[1]?.arrive(125_000);
    vi.advanceTimersByTime(3000);
    expect(first.successes).toEqual([startMs + 1000]);
    expect(second.successes).toEqual([startMs + 2000]);
  });

  it("tells hls.js of a held load aborted, and the next load has the link from then", () => {
    shapeChunkLoads({ capKbps: () => 1000, isDropped: () => false });
    const load = new Shaped({} as HlsConfig);
    const { callbacks, successes, aborts } = told();
    load.load(context, {} as LoaderConfiguration, callbacks);
    asked[0]?.arrive(250_000);
    vi.advanceTimersByTime(500);
    load.abort();
    vi.advanceTimersByTime(5000);
    expect(successes).toEqual([]);
    expect(aborts).toHaveLength(1);
    expect(load.stats.aborted).toBe(true);
  });
});

describe("a chunk load while the tab's connection is dropped (D380 §3)", () => {
  it("fails as a lost network's does, asking nothing, while dropped, and loads again after", () => {
    let isDropped = true;
    shapeChunkLoads({ capKbps: () => undefined, isDropped: () => isDropped });
    const during = told();
    new Shaped({} as HlsConfig).load(context, {} as LoaderConfiguration, during.callbacks);
    vi.advanceTimersByTime(0);
    expect(asked).toEqual([]);
    expect(during.errors).toEqual([
      { code: 0, text: "the connection is dropped (a debug tool, D380)" },
    ]);
    isDropped = false;
    const after = told();
    new Shaped({} as HlsConfig).load(context, {} as LoaderConfiguration, after.callbacks);
    asked[0]?.arrive(1000);
    expect(after.successes).toHaveLength(1);
  });
});
