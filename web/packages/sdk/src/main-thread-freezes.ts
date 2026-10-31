// SPDX-License-Identifier: MPL-2.0

/**
 * Freezes of a page's main thread, its every task held up for a second or more: the one way they are
 * found, a playback's (the player's main-thread.ts, for its play report) and the whole page's (the web
 * app's page-freezes.ts, for the core's log) alike. Here, not in the player, because the web app's
 * home page watches its freezes without loading the player and hls.js with it.
 */

/**
 * The least a page stands still for to be noted, in milliseconds: a second, as a stall of the video
 * is counted (stallAfterMs). Shorter, and a weak device's every chunk appended would be one.
 */
export const mainThreadStallAtLeastMs = 1000;

/**
 * How often the timer that stands in for long tasks where the browser reports none checks its own
 * lateness, in milliseconds: a second, cheap beside a film's decoding.
 */
export const mainThreadSampleMs = 1000;

/** Each long task as the browser tells it, its start and length in ms on nowMs's clock; returns the stopping. */
export type LongTasks = (task: (startMs: number, durationMs: number) => void) => () => void;

/** A PerformanceObserver as reading long tasks needs it; supportedEntryTypes is missing in older browsers. */
export interface LongTaskObserverClass {
  readonly supportedEntryTypes?: readonly string[];
  new (
    callback: (list: {
      getEntries(): readonly { readonly startTime: number; readonly duration: number }[];
    }) => void,
  ): { observe(options: { readonly type: string }): void; disconnect(): void };
}

/** What reading long tasks needs of the browser: its PerformanceObserver, where it has one. */
export interface LongTaskScope {
  // eslint-disable-next-line @typescript-eslint/naming-convention -- the browser's own name
  readonly PerformanceObserver?: LongTaskObserverClass | undefined;
}

/** The long tasks this browser reports, through its PerformanceObserver; undefined where it reports none. */
export function longTasksOf(scope: LongTaskScope): LongTasks | undefined {
  const observer = scope.PerformanceObserver;
  if (observer?.supportedEntryTypes?.includes("longtask") !== true) {
    return undefined;
  }
  return (task) => {
    const observing = new observer((list) => {
      for (const entry of list.getEntries()) {
        task(entry.startTime, entry.duration);
      }
    });
    observing.observe({ type: "longtask" });
    return () => {
      observing.disconnect();
    };
  };
}

/** Whether the page this runs on is hidden; false where there is no page. */
export function isPageHidden(): boolean {
  return typeof document !== "undefined" && document.visibilityState === "hidden";
}

/** A freeze of the page's main thread: when it began and how long it lasted, in ms, and how it was seen. */
export interface Freeze {
  readonly startMs: number;
  readonly durationMs: number;
  /** A long task the browser reported, or a timer's lateness standing in where it reports none. */
  readonly how: "long task" | "timer late";
  /** Whether the page was hidden as it was told: a long task of a hidden page froze nothing seen. */
  readonly isHidden: boolean;
}

/** What watchFreezes reads of the page. */
export interface FreezeWatch {
  /** The page's clock, ms; performance.now's, as long tasks are timed. */
  readonly nowMs: () => number;
  /** Whether the page is hidden, where a browser slows its timers, so lateness is no freeze. */
  readonly isHidden: () => boolean;
  /** The browser's long tasks; undefined where it reports none, and the timer stands in. */
  readonly longTasks: LongTasks | undefined;
  /**
   * When the standing-in timer runs: calls start as it should run and stop as it should not, and
   * returns the end of that; omitted, the timer runs all the while the page is watched.
   */
  readonly sampleWhile?: ((start: () => void, stop: () => void) => () => void) | undefined;
}

/**
 * Tells frozen each freeze of the page's main thread of mainThreadStallAtLeastMs or more: each such
 * long task where the browser reports them, else a timer of mainThreadSampleMs late by as much on a
 * page shown both as it was set and as it ran, since a hidden page's timers are slowed by the browser,
 * not frozen. The one way the page's freezes are found, a playback's and the whole page's alike.
 * Returns the stopping.
 */
export function watchFreezes(by: FreezeWatch, frozen: (freeze: Freeze) => void): () => void {
  if (by.longTasks !== undefined) {
    return by.longTasks((startMs, durationMs) => {
      if (durationMs >= mainThreadStallAtLeastMs) {
        frozen({ startMs, durationMs, how: "long task", isHidden: by.isHidden() });
      }
    });
  }
  let timer: ReturnType<typeof setTimeout> | undefined;
  const stop = (): void => {
    clearTimeout(timer);
    timer = undefined;
  };
  const next = (): void => {
    const dueMs = by.nowMs() + mainThreadSampleMs;
    const wasHidden = by.isHidden();
    timer = setTimeout(() => {
      const lateMs = by.nowMs() - dueMs;
      // A timer set or run while the page was hidden is late by the browser's throttling, not a freeze.
      if (!wasHidden && !by.isHidden() && lateMs >= mainThreadStallAtLeastMs) {
        frozen({ startMs: dueMs, durationMs: lateMs, how: "timer late", isHidden: false });
      }
      next();
    }, mainThreadSampleMs);
  };
  const start = (): void => {
    if (timer === undefined) {
      next();
    }
  };
  if (by.sampleWhile === undefined) {
    start();
    return stop;
  }
  const stopHearing = by.sampleWhile(start, stop);
  return () => {
    stop();
    stopHearing();
  };
}
