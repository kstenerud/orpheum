// SPDX-License-Identifier: MPL-2.0

import { mainThreadSampleMs } from "@orpheum/sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  isFilmPlayingOn,
  pageFreezeReportEveryMs,
  reportPageFreezes,
  type FilmsPage,
} from "./page-freezes.ts";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

/** A page watched for freezes on screen, its long tasks told by the test, its reports kept. */
function watched(screen = "series-page"): {
  readonly reports: string[];
  readonly task: (startMs: number, durationMs: number) => void;
  readonly page: { isHidden: boolean; isFilmPlaying: boolean };
  readonly stop: () => void;
} {
  const reports: string[] = [];
  const page = { isHidden: false, isFilmPlaying: false };
  let told: ((startMs: number, durationMs: number) => void) | undefined;
  const stop = reportPageFreezes(
    { screen: () => screen, isFilmPlaying: () => page.isFilmPlaying },
    {
      nowMs: () => Date.now(),
      isHidden: () => page.isHidden,
      longTasks: (task) => {
        told = task;
        return () => {
          told = undefined;
        };
      },
      report: (text) => reports.push(text),
    },
  );
  return {
    reports,
    task: (startMs, durationMs) => told?.(startMs, durationMs),
    page,
    stop,
  };
}

describe("a page's freezes, told to the core's log on every screen (the Fire TV's stuck focus, 2026-10-30)", () => {
  it("tells a freeze of a second or more with the screen, whether a film played and whether the page was hidden", () => {
    const { reports, task, page, stop } = watched();
    task(0, 999);
    expect(reports).toEqual([]);
    task(0, 2300);
    expect(reports).toEqual([
      "the page froze 2.3 s (long task) on screen=series-page filmPlaying=false hidden=false",
    ]);
    vi.advanceTimersByTime(pageFreezeReportEveryMs);
    page.isHidden = true;
    page.isFilmPlaying = true;
    task(0, 1500);
    expect(reports.at(-1)).toBe(
      "the page froze 1.5 s (long task) on screen=series-page filmPlaying=true hidden=true",
    );
    stop();
    task(0, 5000);
    expect(reports).toHaveLength(2);
  });

  it("tells at most one report each pageFreezeReportEveryMs, those within it together at its end", () => {
    const { reports, task } = watched("home");
    task(0, 1200);
    vi.advanceTimersByTime(1000);
    task(0, 3400);
    task(0, 1100);
    expect(reports).toHaveLength(1);
    vi.advanceTimersByTime(pageFreezeReportEveryMs - 1000 - 1);
    expect(reports).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(reports).toEqual([
      "the page froze 1.2 s (long task) on screen=home filmPlaying=false hidden=false",
      "the page froze 3.4 s (long task) on screen=home filmPlaying=false hidden=false; 1 more since, the longest 3.4 s",
    ]);
  });

  it("where the browser reports no long tasks, tells a timer late on a shown page, never one a hidden page slowed", () => {
    const reports: string[] = [];
    const page = { isHidden: false };
    let lateMs = 0;
    const stop = reportPageFreezes(
      { screen: () => "home", isFilmPlaying: () => false },
      {
        // The page's clock runs ahead of the timers by lateMs, as a frozen page's does.
        nowMs: () => Date.now() + lateMs,
        isHidden: () => page.isHidden,
        longTasks: undefined,
        report: (text) => reports.push(text),
      },
    );
    lateMs += 2000;
    vi.advanceTimersByTime(mainThreadSampleMs);
    expect(reports).toEqual([
      "the page froze 2.0 s (timer late) on screen=home filmPlaying=false hidden=false",
    ]);
    vi.advanceTimersByTime(pageFreezeReportEveryMs);
    page.isHidden = true;
    lateMs += 30_000;
    vi.advanceTimersByTime(mainThreadSampleMs * 3);
    expect(reports).toHaveLength(1);
    stop();
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("whether a film plays on the page", () => {
  it("is whether any of its videos plays", () => {
    const video = (
      isPaused: boolean,
      hasEnded = false,
    ): Pick<HTMLMediaElement, "paused" | "ended"> => ({
      paused: isPaused,
      ended: hasEnded,
    });
    const page = (videos: Pick<HTMLMediaElement, "paused" | "ended">[]): FilmsPage => ({
      querySelectorAll: () => videos,
    });
    expect(isFilmPlayingOn(page([]))).toBe(false);
    expect(isFilmPlayingOn(page([video(true), video(false, true)]))).toBe(false);
    expect(isFilmPlayingOn(page([video(true), video(false)]))).toBe(true);
  });
});
