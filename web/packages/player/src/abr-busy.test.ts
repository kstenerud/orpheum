// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { BusyPage, longTasksKeptForMs } from "./abr-busy.ts";
import {
  chunkAt,
  choosingFrom,
  fireTvPlaying,
  scene,
  topChunkOnItsWay,
  useScene,
} from "./test-choosing.ts";

useScene({ beforeEach, afterEach, vi });

describe("a busy page measures nothing of the link (D382 §3)", () => {
  it("does not give up a chunk whose arrival a long task overlapped, and notes it with the task's length", () => {
    const chosen = fireTvPlaying();
    scene.longTask(scene.nowMs + 600, 812); // the page busy as the chunk's bytes came
    topChunkOnItsWay(chosen, { pieceAtMs: 1500, afterBytes: 0 });
    expect(chosen.givenUp).toEqual([]);
    expect(scene.linkNotes.at(-1)).toBe(
      "excused a chunk of 4 on its way (131072 B in 2000 ms), the page busy: a long task of 812 ms",
    );
    // Noted once for the chunk, however long it stays excused.
    vi.advanceTimersByTime(2000);
    expect(scene.linkNotes.filter((note) => note.startsWith("excused"))).toHaveLength(1);
    expect(scene.steps).toEqual([]); // nothing lowered
  });

  it("gives up a chunk as before where no long task overlapped its arrival", () => {
    const chosen = fireTvPlaying();
    scene.longTask(scene.nowMs - 5000, 812); // before the chunk was asked for
    const chunk = topChunkOnItsWay(chosen, { pieceAtMs: 500, afterBytes: 375_000 });
    expect(chosen.givenUp).toEqual([chunk]);
    expect(scene.linkNotes.filter((note) => note.startsWith("excused"))).toEqual([]);
  });

  it("takes no sample of a loaded chunk whose arrival a long task overlapped", () => {
    const { choosing } = choosingFrom(40_000_000);
    choosing.firstLevel();
    const chunk = chunkAt(3);
    choosing.loading(chunk);
    chunk.stats.loaded = 1_000_000;
    chunk.stats.loading = { start: 0, first: 50, end: 1050 };
    scene.longTask(700, 120);
    choosing.loaded(chunk);
    expect(choosing.measures(chunk)).toBe(false);
    expect(scene.samplesNoted).toEqual([]);
    expect(scene.linkNotes.at(-1)).toBe(
      "excused a chunk of 3 loaded (1000000 B in 1000 ms), the page busy: a long task of 120 ms",
    );
    // The next, the page idle as it came, measures the link.
    const next = chunkAt(3);
    choosing.loading(next);
    next.stats.loaded = 1_000_000;
    next.stats.loading = { start: 2000, first: 2050, end: 3050 };
    choosing.loaded(next);
    expect(choosing.measures(next)).toBe(true);
  });
});

describe("the page's long tasks (BusyPage)", () => {
  it("says the longest that overlapped a time, and none where the browser reports none", () => {
    let told: (startMs: number, durationMs: number) => void = () => undefined;
    const busy = new BusyPage((task) => {
      told = task;
      return () => undefined;
    });
    told(1000, 60);
    told(1100, 300);
    expect(busy.longestWithin(0, 1050)).toBe(60);
    expect(busy.longestWithin(1050, 2000)).toBe(300);
    expect(busy.longestWithin(1400, 2000)).toBeUndefined();
    // One long past is forgotten as the next is heard.
    told(1400 + longTasksKeptForMs + 1, 50);
    expect(busy.longestWithin(0, 2000)).toBeUndefined();
    expect(new BusyPage(undefined).longestWithin(0, Number.POSITIVE_INFINITY)).toBeUndefined();
  });
});
