// SPDX-License-Identifier: MPL-2.0

import { mkdirSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";

import type {
  FullResult,
  Reporter,
  TestCase,
  TestResult,
  TestStep,
} from "@playwright/test/reporter";

/**
 * A reporter that records every pass's timings, each test's and each of its steps' (a navigation, a
 * click, an expect's wait, a test.step), always: tests grow heavier over time, and when a run is
 * found slow its timings are already there to say where (the owner, 2026-10-20). Recording costs a
 * timestamp per step. `scripts/browser_timings.py` reads them.
 */

/** Where each pass's timings go: beside the dev loop's state, outside the outputDir each pass empties. */
export const timingsDirectory = new URL("../../.dev/browser-timings/", import.meta.url).pathname;
/** How many passes' timings are kept, the newest. */
export const keptPasses = 20;
/** A step shorter than this is left out: a slow test's time is in its long steps, and the file stays small. */
export const shortestStepMs = 50;

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

/** The steps of at least shortestStepMs among steps and those inside them, in the order they ran. */
export function stepTimings(steps: readonly TestStep[], depth = 0): StepTiming[] {
  return steps.flatMap((step) => [
    ...(step.duration >= shortestStepMs
      ? [{ title: step.title, category: step.category, ms: step.duration, depth }]
      : []),
    ...stepTimings(step.steps, depth + 1),
  ]);
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

export default class Timings implements Reporter {
  readonly #pass: string;
  readonly #directory: string;
  readonly #tests: TestTiming[] = [];
  readonly #started = new Date();

  constructor(options: { pass?: string; directory?: string } = {}) {
    this.#pass =
      options.pass ?? (process.env["ORPHEUM_E2E_TIMING"] === "1" ? "timed" : "side-by-side");
    this.#directory = options.directory ?? timingsDirectory;
  }

  onTestEnd(test: TestCase, result: TestResult): void {
    this.#tests.push({
      test: test.title,
      project: test.parent.project()?.name ?? "",
      file: test.location.file.split("/e2e/").pop() ?? test.location.file,
      line: test.location.line,
      status: result.status,
      ms: result.duration,
      worker: result.workerIndex,
      steps: stepTimings(result.steps),
    });
  }

  onEnd(result: FullResult): void {
    mkdirSync(this.#directory, { recursive: true });
    const stamp = this.#started.toISOString().replaceAll(":", "-");
    const record = {
      startedAt: this.#started.toISOString(),
      pass: this.#pass,
      status: result.status,
      ms: result.duration,
      tests: this.#tests,
    };
    writeFileSync(join(this.#directory, `${stamp}-${this.#pass}.json`), JSON.stringify(record));
    keepNewest(this.#directory, keptPasses);
  }

  printsToStdio(): boolean {
    return false;
  }
}
