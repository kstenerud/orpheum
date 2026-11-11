// SPDX-License-Identifier: MPL-2.0

/**
 * The page's freezes told to the core's log, on every page: its main thread busy for a second or
 * more, found the one way the player finds a playback's (the SDK's watchFreezes, which the player's
 * main-thread.ts uses too), with what the page was doing. A Fire TV's focus stuck on a series' season row for many presses, its live
 * connection silent meanwhile, and nothing said whether the page had frozen or on what (the owner,
 * staging, 2026-10-30; digging means missing logs).
 */

import {
  howFrozen,
  isPageHidden,
  longTasksOf,
  reportFailure,
  watchFreezes,
  type Freeze,
  type LongTasks,
} from "@orpheum/sdk";

/**
 * The least time between two reports of the page's freezes, in milliseconds: a weak device frozen
 * again and again would otherwise send a report each second, crowding out its failures (the page
 * sends failuresSentMost at most). Those within it are told together at its end.
 */
export const pageFreezeReportEveryMs = 5000;

/** What the page was doing, read as a freeze is told. */
export interface PageDoing {
  /** The screen shown: its route's name. */
  readonly screen: () => string;
  /** Whether a film plays on the page. */
  readonly isFilmPlaying: () => boolean;
}

/** How the page is watched: its clock, whether it is hidden, its long tasks, and where reports go. */
export interface PageFreezeWatch {
  readonly nowMs?: () => number;
  readonly isHidden?: () => boolean;
  /**
   * The browser's long tasks; undefined where it reports none, and a timer stands in; omitted, the
   * page's own browser's.
   */
  readonly longTasks?: LongTasks | undefined;
  /**
   * Where each report goes; omitted, the core's log through the page's failures, the console's line
   * headed "Orpheum:" alone, as a freeze is no request's failure.
   */
  readonly report?: (text: string) => void;
}

/**
 * Tells the core's log each freeze of the page of a second or more, with the screen, whether a film
 * played and whether the page was hidden (a hidden page's slowed timers are no freeze), at most one
 * report each pageFreezeReportEveryMs, those within it told together at its end. Returns the stopping.
 */
export function reportPageFreezes(doing: PageDoing, watch: PageFreezeWatch = {}): () => void {
  const nowMs = watch.nowMs ?? (() => performance.now());
  const report =
    watch.report ??
    ((text: string) => {
      reportFailure(text, "Orpheum:");
    });
  let lastReportMs: number | undefined;
  let held: Freeze[] = [];
  let flush: ReturnType<typeof setTimeout> | undefined;
  const tell = (freezes: readonly Freeze[]): void => {
    const [first, ...more] = freezes;
    if (first === undefined) {
      return;
    }
    lastReportMs = nowMs();
    const longest = Math.max(...freezes.map((freeze) => freeze.durationMs));
    report(
      `the page froze ${seconds(first.durationMs)} (${howFrozen(first)}) on screen=${doing.screen()}` +
        ` filmPlaying=${String(doing.isFilmPlaying())} hidden=${String(first.isHidden)}` +
        (more.length === 0
          ? ""
          : `; ${String(more.length)} more since, the longest ${seconds(longest)}`),
    );
  };
  const stopWatching = watchFreezes(
    {
      nowMs,
      isHidden: watch.isHidden ?? isPageHidden,
      longTasks: "longTasks" in watch ? watch.longTasks : longTasksOf(globalThis),
    },
    (freeze) => {
      const sinceMs = lastReportMs === undefined ? Infinity : nowMs() - lastReportMs;
      if (sinceMs >= pageFreezeReportEveryMs && held.length === 0) {
        tell([freeze]);
        return;
      }
      held.push(freeze);
      flush ??= setTimeout(
        () => {
          flush = undefined;
          const told = held;
          held = [];
          tell(told);
        },
        Math.max(0, pageFreezeReportEveryMs - sinceMs),
      );
    },
  );
  return () => {
    stopWatching();
    clearTimeout(flush);
  };
}

/** A page's videos, as isFilmPlayingOn reads them: a browser's document. */
export interface FilmsPage {
  querySelectorAll(selectors: "video"): ArrayLike<Pick<HTMLMediaElement, "paused" | "ended">>;
}

/** Whether a film plays on page: any of its videos playing. */
export function isFilmPlayingOn(page: FilmsPage): boolean {
  return Array.from(page.querySelectorAll("video")).some((video) => !video.paused && !video.ended);
}

/** Milliseconds as seconds, for a report: "2.3 s". */
function seconds(ms: number): string {
  return `${(ms / 1000).toFixed(1)} s`;
}
