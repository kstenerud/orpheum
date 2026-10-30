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

/** The page's recent long tasks, heard as the browser reports them. */
export class BusyPage {
  #tasks: LongTask[] = [];
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
