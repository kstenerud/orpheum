// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  causeOfFrame,
  longTasksOf,
  watchFreezes,
  type Freeze,
  type LongTaskEntry,
} from "./main-thread-freezes.ts";

/** A PerformanceObserver supporting types that, observing, tells entries. */
function observerOf(
  types: readonly string[],
  entries: readonly LongTaskEntry[],
  observed: string[],
) {
  return class {
    static readonly supportedEntryTypes = types;
    readonly #callback: (list: { getEntries: () => readonly LongTaskEntry[] }) => void;

    constructor(callback: (list: { getEntries: () => readonly LongTaskEntry[] }) => void) {
      this.#callback = callback;
    }

    observe(options: { type: string }): void {
      observed.push(options.type);
      this.#callback({ getEntries: () => entries });
    }

    disconnect(): void {
      observed.push("disconnected");
    }
  };
}

/** The Fire TV's 4.4 s walk of getClientRects in a key's listener, as a long animation frame. */
const theWalk: LongTaskEntry = {
  startTime: 100,
  duration: 4400,
  renderStart: 4300,
  scripts: [
    { duration: 20, invokerType: "user-callback", invoker: "FrameRequestCallback" },
    {
      duration: 4150,
      invokerType: "event-listener",
      invoker: "BODY.onkeydown",
      sourceFunctionName: "moveFocus",
      sourceURL: "https://orpheum.dev/assets/index-3f2a.js?v=1",
      sourceCharPosition: 120_456,
      forcedStyleAndLayoutDuration: 3900,
    },
  ],
};

describe("the browser's long work, read as long animation frames where it can (the web guidance, 2026-11-11)", () => {
  it("reads long animation frames over long tasks where the browser has both, each with its cause", () => {
    const observed: string[] = [];
    const longTasks = longTasksOf({
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the browser's own name
      PerformanceObserver: observerOf(["longtask", "long-animation-frame"], [theWalk], observed),
    });
    const told: unknown[][] = [];
    const stop = longTasks?.((...task) => told.push(task));
    stop?.();
    expect({ observed, told }).toEqual({
      observed: ["long-animation-frame", "disconnected"],
      told: [[100, 4400, causeOfFrame(theWalk)]],
    });
  });

  it("reads long tasks, with no cause, where the browser has no long animation frames", () => {
    const observed: string[] = [];
    const longTasks = longTasksOf({
      // eslint-disable-next-line @typescript-eslint/naming-convention -- the browser's own name
      PerformanceObserver: observerOf(["longtask"], [{ startTime: 10, duration: 1500 }], observed),
    });
    const told: unknown[][] = [];
    longTasks?.((...task) => told.push(task))();
    expect({ observed, told }).toEqual({
      observed: ["longtask", "disconnected"],
      told: [[10, 1500]],
    });
  });

  it("names a frame's longest script, its function, file and place, the layout it forced, and the rendering", () => {
    expect(causeOfFrame(theWalk)).toBe(
      "longest script event-listener BODY.onkeydown moveFocus (index-3f2a.js:120456) 4150 ms," +
        " forced layout 3900 ms; rendering 200 ms",
    );
    expect(causeOfFrame({ startTime: 0, duration: 1200, renderStart: 0, scripts: [] })).toBe(
      "no script",
    );
    expect(
      causeOfFrame({ startTime: 0, duration: 1200, scripts: [{ duration: 1100, invoker: "x" }] }),
    ).toBe("longest script x 1100 ms");
  });

  it("tells a frame's freeze as a long animation frame with its cause, a task's as a long task", () => {
    const freezes: Freeze[] = [];
    let task: ((startMs: number, durationMs: number, cause?: string) => void) | undefined;
    const stop = watchFreezes(
      {
        nowMs: () => 0,
        isHidden: () => false,
        longTasks: (report) => {
          task = report;
          return () => undefined;
        },
      },
      (freeze) => freezes.push(freeze),
    );
    task?.(0, 2000, "no script");
    task?.(0, 1500);
    stop();
    expect(freezes).toEqual([
      {
        startMs: 0,
        durationMs: 2000,
        how: "long animation frame",
        cause: "no script",
        isHidden: false,
      },
      { startMs: 0, durationMs: 1500, how: "long task", isHidden: false },
    ]);
  });
});
