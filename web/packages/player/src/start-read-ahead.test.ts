// SPDX-License-Identifier: MPL-2.0

import type {
  Fragment,
  HlsConfig,
  Loader,
  LoaderCallbacks,
  LoaderConfiguration,
  LoaderContext,
  LoaderStats,
} from "hls.js";
import { LoaderContextType, PlaylistLevelType } from "hls.js";
import { beforeEach, describe, expect, it } from "vitest";

import { StartReadAhead } from "./start-read-ahead.ts";

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

/** A load the stand-in was asked: its address, and its answer or failure when the test says. */
interface Asked {
  readonly url: string;
  /** Its head came at headAtMs, and bytes of it have come, all of it not yet. */
  readonly come: (headAtMs: number, bytes: number) => void;
  readonly arrive: (bytes: number) => void;
  readonly fail: (code: number) => void;
  readonly isAborted: () => boolean;
}

const asked: Asked[] = [];

/** A stand-in for hls.js's loader: each load answers when the test says. */
class StandInLoader implements Loader<LoaderContext> {
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
    asked.push({
      url: context.url,
      come: (headAtMs, bytes) => {
        this.stats.loading.first = headAtMs;
        this.stats.loaded = bytes;
      },
      arrive: (bytes) => {
        this.stats.loaded = bytes;
        this.stats.total = bytes;
        callbacks.onSuccess(
          { url: context.url, data: new ArrayBuffer(bytes) },
          this.stats,
          context,
          null,
        );
      },
      fail: (code) => {
        callbacks.onError({ code, text: "" }, context, null, this.stats);
      },
      isAborted: () => this.#isAborted,
    });
  }
}

/** Chunk sn of a film's picture at level, or its sound's. */
function chunk(sn: number, level = 0, type = PlaylistLevelType.MAIN): Fragment {
  return { sn, level, type, url: `/c/${String(level)}/${String(sn)}.m4s` } as unknown as Fragment;
}

/** Two qualities of five chunks each, as hls.js holds them. */
const held = {
  levels: [0, 1].map((level) => ({
    details: { fragments: [0, 1, 2, 3, 4].map((sn) => chunk(sn, level)) },
  })),
};

/** hls.js's load context of frag. */
function contextOf(frag: Fragment): LoaderContext {
  const context: LoaderContext & { readonly frag: Fragment } = {
    url: frag.url,
    responseType: "arraybuffer",
    type: LoaderContextType.MEDIA_FRAGMENT,
    rangeStart: 0,
    rangeEnd: 0,
    frag,
  };
  return context;
}

/** A load's callbacks, and the bytes of each answer they were told. */
function told(): {
  readonly callbacks: LoaderCallbacks<LoaderContext>;
  readonly answered: number[];
} {
  const answered: number[] = [];
  return {
    answered,
    callbacks: {
      onSuccess: (_response, stats) => answered.push(stats.loaded),
      onError: (error) => answered.push(-error.code),
      onTimeout: () => undefined,
    },
  };
}

/** A playback's read-ahead over the stand-in, and the class of loader hls.js makes each load with. */
function readingAhead(): {
  readonly reading: StartReadAhead;
  readonly loaderClass: ReturnType<StartReadAhead["loaderOf"]>;
} {
  const reading = new StartReadAhead();
  reading.readsOn(held);
  return { reading, loaderClass: reading.loaderOf(StandInLoader) };
}

/**
 * Asks for frag as hls.js does, by a loader of its own, frag's stats then the loader's (as hls.js's
 * fragment loader sets them); returns what it was told.
 */
function ask(loaderClass: ReturnType<StartReadAhead["loaderOf"]>, frag: Fragment): number[] {
  const { callbacks, answered } = told();
  const loader = new loaderClass({} as HlsConfig);
  // eslint-disable-next-line no-param-reassign -- as hls.js's fragment loader does
  frag.stats = loader.stats;
  loader.load(contextOf(frag), {} as LoaderConfiguration, callbacks);
  return answered;
}

beforeEach(() => {
  asked.length = 0;
});

// A Fire TV's start asked chunk 1 only once chunk 0 was appended, and its first frame waited on chunk
// 1 (start-chain-2026-11-16.md): the start asks for its first two picture chunks together (D502 §2).
describe("a playback's start reads its second picture chunk ahead (D502 §2)", () => {
  it("asks for the chunk after the start's first as it asks for the first", () => {
    const { loaderClass } = readingAhead();
    ask(loaderClass, chunk(2));
    expect(asked.map((each) => each.url)).toEqual(["/c/0/2.m4s", "/c/0/3.m4s"]);
  });

  it("answers hls.js's ask for that chunk from the load under way, asking nothing more", async () => {
    const { loaderClass } = readingAhead();
    ask(loaderClass, chunk(0));
    const second = ask(loaderClass, chunk(1));
    expect(asked).toHaveLength(2);
    asked[1]?.arrive(736_297);
    await Promise.resolve();
    expect(second).toEqual([736_297]);
  });

  // A start's second chunk, taken over, said none of its bytes to hls.js until all of it came, and
  // the player gave it up as stuck after 6 s, again and again, its film never starting at 300 kb/s
  // (staging, the debug cap, 2026-11-17).
  it("says to hls.js the bytes of the load under way as they come, not only once all of it came", () => {
    const { loaderClass } = readingAhead();
    ask(loaderClass, chunk(0));
    const second = chunk(1);
    ask(loaderClass, second);
    asked[1]?.come(700, 225_000);
    expect(second.stats.loaded).toBe(225_000);
    expect(second.stats.loading.first).toBe(700);
  });

  it("answers it from the answer kept, when the chunk came before hls.js asked", async () => {
    const { loaderClass } = readingAhead();
    ask(loaderClass, chunk(0));
    asked[1]?.arrive(423_000);
    const second = ask(loaderClass, chunk(1));
    expect(second).toEqual([]);
    await Promise.resolve();
    expect(second).toEqual([423_000]);
    expect(asked).toHaveLength(2);
  });

  it("reads ahead at the start alone, not at each chunk after", () => {
    const { loaderClass } = readingAhead();
    ask(loaderClass, chunk(0));
    ask(loaderClass, chunk(1));
    ask(loaderClass, chunk(2));
    expect(asked.map((each) => each.url)).toEqual(["/c/0/0.m4s", "/c/0/1.m4s", "/c/0/2.m4s"]);
  });

  it("gives up the chunk read ahead when hls.js asks for another, at another quality", () => {
    const { loaderClass } = readingAhead();
    ask(loaderClass, chunk(0));
    ask(loaderClass, chunk(1, 1));
    expect(asked[1]?.isAborted()).toBe(true);
    expect(asked.map((each) => each.url)).toEqual(["/c/0/0.m4s", "/c/0/1.m4s", "/c/1/1.m4s"]);
  });

  it("asks again for a chunk whose read-ahead failed before hls.js asked, as one not made yet is", () => {
    const { loaderClass } = readingAhead();
    ask(loaderClass, chunk(0));
    asked[1]?.fail(425);
    const second = ask(loaderClass, chunk(1));
    expect(asked.map((each) => each.url)).toEqual(["/c/0/0.m4s", "/c/0/1.m4s", "/c/0/1.m4s"]);
    asked[2]?.arrive(10);
    expect(second).toEqual([10]);
  });

  it("reads nothing ahead of a sound's chunk, nor past the last chunk", () => {
    const { loaderClass } = readingAhead();
    ask(loaderClass, chunk(0, 0, PlaylistLevelType.AUDIO));
    expect(asked).toHaveLength(1);
    const other = readingAhead();
    ask(other.loaderClass, chunk(4));
    expect(asked).toHaveLength(2);
  });

  it("gives up a read-ahead nothing took when its playback ends", () => {
    const { reading, loaderClass } = readingAhead();
    ask(loaderClass, chunk(0));
    reading.stop();
    expect(asked[1]?.isAborted()).toBe(true);
  });
});
