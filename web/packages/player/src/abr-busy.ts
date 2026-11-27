// SPDX-License-Identifier: MPL-2.0

/**
 * The page's long tasks, as the chunked player's bitrate controller (abr.ts) reads them: a chunk
 * whose arrival one overlapped is no sample of the link and cannot cause a give-up (D382 §3). Bytes
 * the page was too busy to read say nothing of the link: a Fire TV's page, its decoder dropping
 * frames, read one 128 KiB piece of a chunk in two seconds on a link that carried 20 Mb/s, and the
 * chunk was given up for the lowest quality (staging, 2026-10-30). The browser's own long tasks, of
 * 50 ms and more, where it reports them (longTasksOf, main-thread.ts); where it does not, nothing is
 * excused.
 */

import type { LongTasks } from "./main-thread.ts";

/**
 * How long a long task is kept after it ends, in ms: longer than a chunk takes to arrive before it is
 * given up or asked for again (asksAgainAfterAtMostMs), so any chunk's arrival can be read against it.
 */
export const longTasksKeptForMs = 60_000;

/** A long task: its start and length, in ms on performance.now's clock, as hls.js times its loads. */
interface LongTask {
  readonly startMs: number;
  readonly durationMs: number;
}

/** A chunk on its way as its excuse is judged: when its first byte came, and what has come since. */
interface OnItsWay {
  readonly chunk: {
    readonly level: number;
    readonly stats: { readonly loading: { readonly first: number } };
  };
  readonly firstByteMs: number | undefined;
  readonly loadedBytes: number;
  readonly loadingMs: number;
}

/** The page's recent long tasks, heard as the browser reports them. */
export class BusyPage {
  #tasks: LongTask[] = [];
  /** The chunk on its way last excused by a long task of the page, noted once (excusesOnItsWay). */
  #excused: object | undefined;
  readonly #stop: () => void;

  /** Hears longTasks, the browser's; undefined where it reports none, and nothing is ever busy. */
  constructor(longTasks: LongTasks | undefined) {
    this.#stop =
      longTasks?.((startMs, durationMs) => {
        this.#heard({ startMs, durationMs });
      }) ?? (() => undefined);
  }

  #heard(task: LongTask): void {
    const keptFromMs = task.startMs + task.durationMs - longTasksKeptForMs;
    this.#tasks = [
      ...this.#tasks.filter((kept) => kept.startMs + kept.durationMs >= keptFromMs),
      task,
    ];
  }

  /** The length, in ms, of the longest long task that overlapped fromMs to toMs; undefined for none. */
  longestWithin(fromMs: number, toMs: number): number | undefined {
    const overlapping = this.#tasks
      .filter((task) => task.startMs < toMs && task.startMs + task.durationMs > fromMs)
      .map((task) => task.durationMs);
    return overlapping.length === 0 ? undefined : Math.max(...overlapping);
  }

  /**
   * Whether the chunk on its way arrived, from its first byte to nowMs, while a long task of the page
   * ran, which excuses it from being given up (D382 §3): stuck, it is still asked for again. Noted
   * once for each chunk, with the longest task's length.
   */
  excusesOnItsWay(
    judged: OnItsWay,
    nowMs: number,
    note: ((note: string) => void) | undefined,
  ): boolean {
    const { chunk } = judged;
    const longestMs =
      judged.firstByteMs === undefined
        ? undefined
        : this.longestWithin(chunk.stats.loading.first, nowMs);
    if (longestMs === undefined) {
      return false;
    }
    if (this.#excused !== chunk) {
      this.#excused = chunk;
      const came = `${String(judged.loadedBytes)} B in ${String(judged.loadingMs)} ms`;
      note?.(busyText({ level: chunk.level, how: `on its way (${came})` }, longestMs));
    }
    return true;
  }

  /** Hears no more. */
  stop(): void {
    this.#stop();
  }
}

/**
 * The note of a chunk excused, its how saying how it came: "excused a chunk of 4 on its way (131072 B in
 * 2000 ms), the page busy: a long task of 812 ms".
 */
export function busyText(
  chunk: { readonly level: number; readonly how: string },
  longestMs: number,
): string {
  return (
    `excused a chunk of ${String(chunk.level)} ${chunk.how}, the page busy:` +
    ` a long task of ${String(Math.round(longestMs))} ms`
  );
}
