// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import Hls, {
  LoadStats,
  type LoaderCallbacks,
  type LoaderConfiguration,
  type LoaderContext,
} from "hls.js";

import { headsKept, LinkWaits, makingLapsesAfterMs } from "./link-wait.ts";
import type { LoaderClass } from "./link-shaping.ts";

/**
 * A film waiting for data waits on its chunks as they come over its link (the owner, 2026-11-17).
 * The core answers at once, so a chunk whose answer has not come is the link's, and only one it
 * answered 425 is the server's to make, which says nothing (D506 §4); one whose head came says its
 * bytes against its length, a chunk all come kept with its bytes.
 */

/** A load a test drives: its head, its bytes and its end. */
interface DrivenLoad {
  readonly url: string;
  /** Its head came at atMs, saying length in Content-Length, or none where undefined. */
  readonly head: (atMs: number, length: number | undefined) => void;
  readonly arrive: (bytes: number) => void;
  readonly succeed: () => void;
  /** Its load lost: a network error, no answer of the core's. */
  readonly lose: () => void;
  /** The core answered 425 at atMs, still making it: a head saying Retry-After, then the error. */
  readonly tooEarly: (atMs: number) => void;
  /** Its loader destroyed, its stats emptied, as hls.js's loaders do. */
  readonly destroy: () => void;
}

/** A carrier whose loads the test drives, as hls.js's XMLHttpRequest loader keeps its stats. */
function drivenCarrier(): { readonly carrier: LoaderClass; readonly loads: DrivenLoad[] } {
  const loads: DrivenLoad[] = [];
  class Driven {
    context: LoaderContext | null = null;
    stats = new LoadStats();
    #length: number | undefined;
    #isTooEarly = false;

    load(context: LoaderContext, _config: unknown, callbacks: LoaderCallbacks<LoaderContext>) {
      this.context = context;
      const { stats } = this;
      loads.push({
        url: context.url,
        head: (atMs, length) => {
          stats.loading.first = atMs;
          this.#length = length;
        },
        arrive: (bytes) => {
          stats.loaded += bytes;
          // hls.js's XMLHttpRequest loader takes total from its progress, not its head.
          stats.total = this.#length ?? 0;
        },
        succeed: () => {
          callbacks.onSuccess({ url: context.url, data: new ArrayBuffer(0) }, stats, context, null);
        },
        lose: () => {
          callbacks.onError({ code: 0, text: "network error" }, context, null, stats);
        },
        tooEarly: (atMs) => {
          stats.loading.first = atMs;
          this.#length = 81;
          this.#isTooEarly = true;
          callbacks.onError({ code: 425, text: "Too Early" }, context, null, stats);
        },
        destroy: () => {
          Object.assign(this, { stats: null });
        },
      });
    }

    getResponseHeader(name: string): string | null {
      if (name === "Retry-After") {
        return this.#isTooEarly ? "1" : null;
      }
      return name === "Content-Length" && this.#length !== undefined ? String(this.#length) : null;
    }

    abort(): void {
      // nothing under way to stop
    }

    destroy(): void {
      // nothing held
    }
  }
  const carrier: LoaderClass = Driven;
  return { carrier, loads };
}

const callbacks: LoaderCallbacks<LoaderContext> = {
  onSuccess: () => undefined,
  onError: () => undefined,
  onTimeout: () => undefined,
};

/** A load's context for a chunk of type, picture ("main") or sound ("audio"), or none. */
function chunkContext(url: string, type?: "main" | "audio" | "subtitle"): LoaderContext {
  const context = {
    url,
    responseType: "arraybuffer",
    ...(type === undefined ? {} : { frag: { type, sn: 0 } }),
  };
  return context as unknown as LoaderContext;
}

/** A playback's waits on a driven carrier, its clock nowMs the test sets, and a loader to ask by. */
function waitsOnADrivenLink(): {
  readonly waits: LinkWaits;
  readonly loads: DrivenLoad[];
  readonly clock: { nowMs: number };
  readonly ask: (url: string, type?: "main" | "audio" | "subtitle") => DrivenLoad;
} {
  const clock = { nowMs: 0 };
  const waits = new LinkWaits(() => clock.nowMs);
  const { carrier, loads } = drivenCarrier();
  const Following = waits.loaderOf(carrier);
  const ask = (url: string, type?: "main" | "audio" | "subtitle"): DrivenLoad => {
    new Following(Hls.DefaultConfig).load(
      chunkContext(url, type),
      {} as LoaderConfiguration,
      callbacks,
    );
    const load = loads.at(-1);
    if (load === undefined) {
      throw new Error("the carrier made no load");
    }
    return load;
  };
  return { waits, loads, clock, ask };
}

describe("a wait for data, its chunks as they come over the link", () => {
  // The core's answer is small and at once: none come is the network's trouble (the owner, 2026-11-17).
  it("is the link's while a chunk's answer has not come, the core answering at once (D506 §4)", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    ask("/c/0.m4s", "main");
    const watch = waits.watch();
    clock.nowMs = 3000;
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 0, linksForMs: 3000 });
  });

  it("says a chunk's bytes against the length its head said, before any byte", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    const chunk = ask("/c/0.m4s", "main");
    const watch = waits.watch();
    chunk.head(1000, 400_000);
    clock.nowMs = 4500;
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 400_000, linksForMs: 4500 });
    chunk.arrive(100_000);
    expect(watch.read()).toEqual({ arrivedBytes: 100_000, neededBytes: 400_000, linksForMs: 4500 });
  });

  it("sums a start's chunks, its second picture chunk and its sound, keeping one all come (D502 §2)", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    const watch = waits.watch();
    const first = ask("/c/0.m4s", "main");
    const second = ask("/c/1.m4s", "main");
    const sound = ask("/a/0.m4s", "audio");
    first.head(100, 300_000);
    second.head(200, 300_000);
    sound.head(150, 50_000);
    first.arrive(300_000);
    first.succeed();
    sound.arrive(10_000);
    clock.nowMs = 3200;
    expect(watch.read()).toEqual({ arrivedBytes: 310_000, neededBytes: 650_000, linksForMs: 3200 });
  });

  it("says nothing while the core makes a chunk of it, answered 425, and is the link's from that chunk's head on (D506 §4)", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    const watch = waits.watch();
    const picture = ask("/c/0.m4s", "main");
    picture.head(10, 300_000);
    picture.arrive(50_000);
    clock.nowMs = 1000;
    ask("/a/0.m4s", "audio").tooEarly(1000);
    expect(watch.read()).toBeUndefined();
    clock.nowMs = 2000;
    const again = ask("/a/0.m4s", "audio");
    expect(watch.read()).toBeUndefined(); // asked again, the core not yet answered
    again.head(2500, 50_000);
    clock.nowMs = 4000;
    expect(watch.read()).toEqual({ arrivedBytes: 50_000, neededBytes: 350_000, linksForMs: 1500 });
  });

  it("is the link's again once a 425 lapses, its chunk not asked again: given up", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    const watch = waits.watch();
    clock.nowMs = 100;
    ask("/c/0.m4s", "main").tooEarly(100);
    clock.nowMs = 100 + makingLapsesAfterMs - 1;
    expect(watch.read()).toBeUndefined();
    clock.nowMs = 100 + makingLapsesAfterMs + 1000;
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 0, linksForMs: 1000 });
  });

  it("takes no 425's head for its chunk's length", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    const watch = waits.watch();
    ask("/c/0.m4s", "main").tooEarly(100);
    const again = ask("/c/0.m4s", "main");
    again.head(300, 400_000);
    again.lose();
    ask("/c/0.m4s", "main");
    clock.nowMs = 1300;
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 400_000, linksForMs: 1000 });
  });

  // hls.js destroys a chunk's loader once it came, emptying its stats; each read threw (staging,
  // 2026-11-17), and the bar showed no more.
  it("reads no more the loader of a chunk all come, whose stats hls.js empties as it destroys it", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    const watch = waits.watch();
    const first = ask("/c/0.m4s", "main");
    first.head(100, 1000);
    first.arrive(1000);
    first.succeed();
    first.destroy();
    ask("/c/1.m4s", "main").head(200, 3000);
    clock.nowMs = 3200;
    expect(watch.read()).toEqual({ arrivedBytes: 1000, neededBytes: 4000, linksForMs: 3200 });
  });

  // The bar, once shown, stays while the film waits for the next chunk (D506 §1).
  it("says every chunk of it come where all came and none is on its way", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    const watch = waits.watch();
    const only = ask("/c/0.m4s", "main");
    only.head(100, 1000);
    only.arrive(1000);
    only.succeed();
    clock.nowMs = 2000;
    expect(watch.read()).toEqual({ arrivedBytes: 1000, neededBytes: 1000, linksForMs: 2000 });
  });

  // A retry is not the server's fault: the chunk exists (the owner, 2026-11-17).
  it("waits no more on a chunk whose load was lost; the one asked again counts against the chunk's first head (D506 §1)", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    const watch = waits.watch();
    const lost = ask("/c/0.m4s", "main");
    lost.head(100, 300_000);
    lost.arrive(20);
    lost.lose();
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 0, linksForMs: 0 });
    const again = ask("/c/0.m4s", "main");
    again.head(2000, 300_000);
    clock.nowMs = 2500;
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 300_000, linksForMs: 2500 });
  });

  it("counts a chunk asked again before its head comes against its first head's length (D506 §1)", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    const watch = waits.watch();
    const givenUp = ask("/c/0.m4s", "main");
    givenUp.head(100, 300_000);
    clock.nowMs = 6000;
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 300_000, linksForMs: 6000 });
    givenUp.lose();
    ask("/c/0.m4s", "main");
    clock.nowMs = 6200;
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 300_000, linksForMs: 6200 });
  });

  it("keeps the first heads of the latest chunks alone", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    for (let index = 0; index <= headsKept; index += 1) {
      const load = ask(`/c/${String(index)}.m4s`, "main");
      load.head(10, 1000);
      load.lose();
    }
    const watch = waits.watch();
    const forgotten = ask("/c/0.m4s", "main");
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 0, linksForMs: 0 });
    forgotten.lose();
    watch.stop();
    const latest = waits.watch();
    ask(`/c/${String(headsKept)}.m4s`, "main");
    clock.nowMs = 510;
    expect(latest.read()).toEqual({ arrivedBytes: 0, neededBytes: 1000, linksForMs: 510 });
  });

  it("counts no chunk that came before the wait began, nor a load of anything but picture and sound", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    const before = ask("/c/0.m4s", "main");
    before.head(10, 100_000);
    before.arrive(100_000);
    before.succeed();
    const watch = waits.watch();
    ask("/c/1.m4s", "main").head(500, 200_000);
    ask("/s/0.vtt", "subtitle");
    ask("/c/master.m3u8");
    clock.nowMs = 4000;
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 200_000, linksForMs: 4000 });
  });

  it("counts no bytes needed of a chunk whose head said no length", () => {
    const { waits, ask } = waitsOnADrivenLink();
    const watch = waits.watch();
    ask("/c/0.m4s", "main").head(100, undefined);
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 0, linksForMs: 0 });
  });

  it("follows no chunk asked after the watch stopped", () => {
    const { waits, ask, clock } = waitsOnADrivenLink();
    const watch = waits.watch();
    ask("/c/0.m4s", "main").head(100, 100_000);
    watch.stop();
    ask("/c/1.m4s", "main");
    clock.nowMs = 3100;
    expect(watch.read()).toEqual({ arrivedBytes: 0, neededBytes: 100_000, linksForMs: 3100 });
  });
});
