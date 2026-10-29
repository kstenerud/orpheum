// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import Hls from "hls.js";

import {
  longTasksOf,
  mainThreadStallNotesAtMost,
  watchMainThread,
  type LongTasks,
  type WatchedHls,
} from "./main-thread.ts";

/** A stand-in for hls.js as the watch hears it. */
class FakeHls {
  readonly #listeners = new Map<string, ((event: unknown, data: unknown) => void)[]>();

  on(event: string, listener: (event: unknown, data: unknown) => void): void {
    this.#listeners.set(event, [...(this.#listeners.get(event) ?? []), listener]);
  }

  off(event: string, listener: (event: unknown, data: unknown) => void): void {
    this.#listeners.set(
      event,
      (this.#listeners.get(event) ?? []).filter((each) => each !== listener),
    );
  }

  trigger(event: string, data: unknown): void {
    for (const listener of this.#listeners.get(event) ?? []) {
      listener(event, data);
    }
  }

  listening(): number {
    return [...this.#listeners.values()].reduce((sum, each) => sum + each.length, 0);
  }
}

/** A stand-in for the film's video: where it plays, whether it does, and its events. */
class FakeVideo extends EventTarget {
  currentTime = 0;
  // eslint-disable-next-line @typescript-eslint/naming-convention -- HTMLMediaElement's own name, which this stands in for
  paused = true;

  /** Plays, as the element tells it. */
  play(): void {
    this.paused = false;
    this.dispatchEvent(new Event("playing"));
  }

  /** Seeks to atS, as the element tells it. */
  seek(atS: number): void {
    this.currentTime = atS;
    this.dispatchEvent(new Event("seeking"));
  }
}

/** A clock the test moves, as performance.now would read. */
function fakeClock(): { nowMs: () => number; advance: (ms: number) => void } {
  let atMs = 0;
  return {
    nowMs: () => atMs,
    advance: (ms) => {
      atMs += ms;
    },
  };
}

/** Watches a FakeVideo on a FakeHls with longTasks, or the timer where none. */
function watched(longTasks?: LongTasks): {
  hls: FakeHls;
  video: FakeVideo;
  notes: string[];
  clock: ReturnType<typeof fakeClock>;
  hidden: { isHidden: boolean };
  stop: () => void;
} {
  const hls = new FakeHls();
  const video = new FakeVideo();
  const notes: string[] = [];
  const clock = fakeClock();
  const hidden = { isHidden: false };
  const stop = watchMainThread(hls as unknown as WatchedHls, video, {
    note: (note) => notes.push(note),
    nowMs: clock.nowMs,
    longTasks,
    isHidden: () => hidden.isHidden,
  });
  return { hls, video, notes, clock, hidden, stop };
}

describe("a long task of a second or more is noted, with the player's last events before it", () => {
  it("notes the Fire TV's skip back: a seek, a quality and a flush, then ten seconds frozen", () => {
    let task: ((startMs: number, durationMs: number) => void) | undefined;
    const { hls, video, notes, clock, stop } = watched((report) => {
      task = report;
      return () => {
        task = undefined;
      };
    });
    clock.advance(5000);
    hls.trigger(Hls.Events.LEVEL_SWITCHING, { level: 1 });
    clock.advance(1000);
    video.seek(1234.5);
    clock.advance(100);
    hls.trigger(Hls.Events.BUFFER_FLUSHING, { startOffset: 0, endOffset: 1200, type: "video" });
    hls.trigger(Hls.Events.ERROR, {
      details: Hls.ErrorDetails.BUFFER_FULL_ERROR,
      sourceBufferName: "audio",
    });
    hls.trigger(Hls.Events.AUDIO_TRACK_SWITCHING, { id: 2 });
    task?.(6200, 999);
    expect(notes).toEqual([]);
    task?.(6300, 9700);
    expect(notes).toEqual([
      "main thread stalled 9.7 s (long task) at 1234.5 s; before: seek to 1234.5 s -0.3 s," +
        " flush video 0-1200 s -0.2 s, full audio -0.2 s, sound 2 -0.2 s",
    ]);
    stop();
    expect(task).toBeUndefined();
    expect(hls.listening()).toBe(0);
  });

  it("notes at most mainThreadStallNotesAtMost a playback", () => {
    let task: ((startMs: number, durationMs: number) => void) | undefined;
    const { notes } = watched((report) => {
      task = report;
      return () => undefined;
    });
    for (let index = 0; index < mainThreadStallNotesAtMost + 3; index += 1) {
      task?.(index * 10_000, 2000);
    }
    expect(notes).toHaveLength(mainThreadStallNotesAtMost);
    expect(notes[0]).toBe("main thread stalled 2.0 s (long task) at 0.0 s; before: nothing");
  });
});

describe("where the browser reports no long tasks, a timer's lateness while the film plays stands in", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("notes a timer a second or more late while the film plays on a shown page, and runs no timer while it does not", () => {
    const { video, notes, clock, hidden, stop } = watched();
    expect(vi.getTimerCount()).toBe(0);
    video.play();
    expect(vi.getTimerCount()).toBe(1);
    clock.advance(1000 + 900);
    vi.advanceTimersByTime(1000);
    expect(notes).toEqual([]);
    video.seek(60);
    clock.advance(1000 + 4000);
    vi.advanceTimersByTime(1000);
    expect(notes).toEqual([
      "main thread stalled 4.0 s (timer late) at 60.0 s; before: seek to 60.0 s -1.0 s",
    ]);
    hidden.isHidden = true;
    clock.advance(1000 + 30_000);
    vi.advanceTimersByTime(1000);
    expect(notes).toHaveLength(1);
    // The first timer after the page is shown again was set while it was hidden.
    hidden.isHidden = false;
    clock.advance(1000 + 30_000);
    vi.advanceTimersByTime(1000);
    expect(notes).toHaveLength(1);
    video.paused = true;
    video.dispatchEvent(new Event("pause"));
    expect(vi.getTimerCount()).toBe(0);
    video.play();
    stop();
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("the browser's long tasks", () => {
  it("are read through a PerformanceObserver where it supports them, and not where it does not", () => {
    const observed: string[] = [];
    type Entries = (list: {
      getEntries: () => readonly { startTime: number; duration: number }[];
    }) => void;
    /** A PerformanceObserver that, observing, tells one long task of 1.5 s. */
    class FakeObserver {
      static readonly supportedEntryTypes: readonly string[] = ["longtask", "mark"];
      readonly #callback: Entries;

      constructor(callback: Entries) {
        this.#callback = callback;
      }

      observe(options: { type: string }): void {
        observed.push(options.type);
        this.#callback({ getEntries: () => [{ startTime: 10, duration: 1500 }] });
      }

      disconnect(): void {
        observed.push("disconnected");
      }
    }
    // eslint-disable-next-line @typescript-eslint/naming-convention -- the browser's own name
    const longTasks = longTasksOf({ PerformanceObserver: FakeObserver });
    const tasks: number[][] = [];
    const stop = longTasks?.((startMs, durationMs) => tasks.push([startMs, durationMs]));
    stop?.();
    expect({ observed, tasks }).toEqual({
      observed: ["longtask", "disconnected"],
      tasks: [[10, 1500]],
    });
    /** A PerformanceObserver that observes no long tasks. */
    class MarksOnly extends FakeObserver {
      static override readonly supportedEntryTypes: readonly string[] = ["mark"];
    }
    // eslint-disable-next-line @typescript-eslint/naming-convention -- the browser's own name
    expect(longTasksOf({ PerformanceObserver: MarksOnly })).toBeUndefined();
    expect(longTasksOf({})).toBeUndefined();
  });
});
