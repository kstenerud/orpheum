// SPDX-License-Identifier: MPL-2.0

import { mkdirSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";

/**
 * Every test suite's kept timings, one record per pass, in one shape whatever the suite, which
 * `scripts/test_timings.py` reads: tests grow heavier over time, and a run found slow is explained
 * from what it already wrote (the owner, 2026-10-20). The browser tier (timings.ts) and vitest
 * (vitest-timings.ts) write them here; the Go suites through scripts/go_test_timings.py.
 */

/** Where every suite keeps its timings, a directory each: check-pr's own tree links it, so its runs are kept too. */
export const timingsRoot = new URL("../../.dev/test-timings/", import.meta.url).pathname;
/** How many passes of each suite are kept, the newest. */
export const keptPasses = 20;

export interface StepTiming {
  readonly title: string;
  /** Playwright's: "pw:api" for an action, "expect", "test.step", "hook", "fixture". */
  readonly category: string;
  readonly ms: number;
  /** 0 for a test's own step, 1 for one inside it, and on. */
  readonly depth: number;
}

export interface TestTiming {
  readonly test: string;
  readonly project: string;
  readonly file: string;
  readonly line: number;
  readonly status: string;
  readonly ms: number;
  readonly worker: number;
  readonly steps: readonly StepTiming[];
}

/** A file's own time: its loading and setup as much as its tests', which a test's time leaves out. */
export interface FileTiming {
  readonly file: string;
  readonly project: string;
  readonly ms: number;
  /** Of ms, loading the file and what it imports, where a suite says. */
  readonly loadMs?: number;
}

export interface PassTiming {
  readonly suite: string;
  readonly pass: string;
  readonly startedAt: Date;
  readonly status: string;
  readonly ms: number;
  readonly tests: readonly TestTiming[];
  readonly files?: readonly FileTiming[];
}

/** Writes one pass's timings into its suite's directory under root, and keeps only the newest keptPasses. */
export function writePass(timing: PassTiming, root = timingsRoot): void {
  const directory = join(root, timing.suite);
  mkdirSync(directory, { recursive: true });
  const stamp = timing.startedAt.toISOString().replaceAll(":", "-");
  const record = { ...timing, startedAt: timing.startedAt.toISOString() };
  writeFileSync(join(directory, `${stamp}-${timing.pass}.json`), JSON.stringify(record));
  keepNewest(directory, keptPasses);
}

/** Removes all but the newest `kept` of the directory's timing files, whose names sort by when they were written. */
export function keepNewest(directory: string, kept: number): void {
  const files = readdirSync(directory)
    .filter((name) => name.endsWith(".json"))
    .sort();
  for (const old of files.slice(0, Math.max(0, files.length - kept))) {
    rmSync(join(directory, old));
  }
}
