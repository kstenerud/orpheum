// SPDX-License-Identifier: MPL-2.0

/**
 * Notes each time a playing film's page stalls, its main thread busy for a second or more, with
 * the player's last events before it, so the play report says what it froze on. A Fire TV's page, after
 * a skip back on its remote, sent its next report 9.7 s late while its own clock said 207 ms passed
 * from the seek to the start, and its picture then seemed to play at double speed for a while: the
 * page frozen for about ten seconds while its decoder and sound went on, then catching up (staging,
 * 2026-10-29). Nothing said so, and nothing said what it froze on (digging means missing logs).
 *
 * Where the browser reports long tasks (Chromium's, Silk's), each of mainThreadStallAtLeastMs or
 * more is noted; elsewhere a timer of mainThreadSampleMs, only while the film plays on a shown page,
 * notes its own lateness of mainThreadStallAtLeastMs or more. At most mainThreadStallNotesAtMost a
 * playback.
 */

import Hls, { type BufferFlushingData, type ErrorData } from "hls.js";

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

/** The most stalls of its page a playback notes: the first few say what happens; more fill its report. */
export const mainThreadStallNotesAtMost = 5;

/** How many of the player's last events a stall's note names: what it froze on is the last or two. */
export const playerEventsRecalled = 4;

/** What watching the page reads of the film: whether it plays, where, and its events. */
export type WatchedFilm = Pick<
  HTMLMediaElement,
  "currentTime" | "paused" | "addEventListener" | "removeEventListener"
>;

/** What watching the page hears of hls.js: its events. */
export type WatchedHls = Pick<Hls, "on" | "off">;

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

/** One of the player's events: when, on nowMs's clock, and what. */
interface PlayerEvent {
  readonly atMs: number;
  readonly text: string;
}

/**
 * Watches the page a chunked playback plays on for stalls of its main thread, noting each with the
 * player's last events (its seeks, its qualities begun, its buffers flushed and run full, its sounds
 * begun). Returns the stopping.
 */
export function watchMainThread(
  hls: WatchedHls,
  video: WatchedFilm,
  options: {
    readonly note: (note: string) => void;
    /** The page's clock, ms; performance.now's, as long tasks are timed. */
    readonly nowMs?: () => number;
    /** The browser's long tasks; undefined where it reports none, and the timer stands in. */
    readonly longTasks?: LongTasks | undefined;
    /** Whether the page is hidden, where a browser slows its timers, so lateness is no stall. */
    readonly isHidden?: () => boolean;
  },
): () => void {
  const nowMs = options.nowMs ?? (() => performance.now());
  const isHidden = options.isHidden ?? (() => globalThis.document.visibilityState === "hidden");
  const recent: PlayerEvent[] = [];
  let noted = 0;
  const remember = (text: string): void => {
    recent.push({ atMs: nowMs(), text });
    recent.splice(0, recent.length - playerEventsRecalled);
  };
  const stalled = (startMs: number, durationMs: number, how: string): void => {
    if (durationMs < mainThreadStallAtLeastMs || noted >= mainThreadStallNotesAtMost) {
      return;
    }
    noted += 1;
    const before = recent
      .filter((event) => event.atMs <= startMs + durationMs)
      .map((event) => `${event.text} ${seconds(event.atMs - startMs)}`);
    options.note(
      `main thread stalled ${(durationMs / 1000).toFixed(1)} s (${how})` +
        ` at ${video.currentTime.toFixed(1)} s; before: ${before.length === 0 ? "nothing" : before.join(", ")}`,
    );
  };
  const stopHearing = hearPlayerEvents(hls, video, remember);
  const stopWatching =
    options.longTasks === undefined
      ? sampleWhilePlaying(video, { nowMs, isHidden, stalled })
      : options.longTasks((startMs, durationMs) => {
          stalled(startMs, durationMs, "long task");
        });
  return () => {
    stopHearing();
    stopWatching();
  };
}

/** Calls remember with each of the player's events worth naming before a stall. Returns the stopping. */
function hearPlayerEvents(
  hls: WatchedHls,
  video: WatchedFilm,
  remember: (text: string) => void,
): () => void {
  const onSeeking = (): void => {
    remember(`seek to ${video.currentTime.toFixed(1)} s`);
  };
  const onLevel = (_event: unknown, data: { readonly level: number }): void => {
    remember(`level ${String(data.level)}`);
  };
  const onSound = (_event: unknown, data: { readonly id: number }): void => {
    remember(`sound ${String(data.id)}`);
  };
  const onFlushing = (_event: unknown, data: BufferFlushingData): void => {
    const end = Number.isFinite(data.endOffset) ? data.endOffset.toFixed(0) : "end";
    remember(`flush ${data.type ?? "all"} ${data.startOffset.toFixed(0)}-${end} s`);
  };
  const onError = (_event: unknown, data: ErrorData): void => {
    if (data.details === Hls.ErrorDetails.BUFFER_FULL_ERROR) {
      remember(`full ${data.sourceBufferName ?? "?"}`);
    }
  };
  video.addEventListener("seeking", onSeeking);
  hls.on(Hls.Events.LEVEL_SWITCHING, onLevel);
  hls.on(Hls.Events.AUDIO_TRACK_SWITCHING, onSound);
  hls.on(Hls.Events.BUFFER_FLUSHING, onFlushing);
  hls.on(Hls.Events.ERROR, onError);
  return () => {
    video.removeEventListener("seeking", onSeeking);
    hls.off(Hls.Events.LEVEL_SWITCHING, onLevel);
    hls.off(Hls.Events.AUDIO_TRACK_SWITCHING, onSound);
    hls.off(Hls.Events.BUFFER_FLUSHING, onFlushing);
    hls.off(Hls.Events.ERROR, onError);
  };
}

/**
 * Stands in for long tasks where the browser reports none: while video plays, a timer every
 * mainThreadSampleMs tells stalled its own lateness, when the page is shown. Returns the stopping.
 */
function sampleWhilePlaying(
  video: WatchedFilm,
  by: {
    readonly nowMs: () => number;
    readonly isHidden: () => boolean;
    readonly stalled: (startMs: number, durationMs: number, how: string) => void;
  },
): () => void {
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
      // A timer set or run while the page was hidden is late by the browser's throttling, not a stall.
      if (!wasHidden && !by.isHidden()) {
        by.stalled(dueMs, lateMs, "timer late");
      }
      next();
    }, mainThreadSampleMs);
  };
  const start = (): void => {
    if (timer === undefined) {
      next();
    }
  };
  video.addEventListener("playing", start);
  video.addEventListener("pause", stop);
  video.addEventListener("ended", stop);
  if (!video.paused) {
    start();
  }
  return () => {
    stop();
    video.removeEventListener("playing", start);
    video.removeEventListener("pause", stop);
    video.removeEventListener("ended", stop);
  };
}

/** Milliseconds as signed seconds, for a note: "+0.2 s", "-1.3 s". */
function seconds(ms: number): string {
  const text = `${(ms / 1000).toFixed(1)} s`;
  return ms < 0 ? text : `+${text}`;
}
