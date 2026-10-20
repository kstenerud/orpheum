// SPDX-License-Identifier: MPL-2.0

import type {
  FullResult,
  Reporter,
  TestCase,
  TestResult,
  TestStep,
} from "@playwright/test/reporter";

import { type StepTiming, type TestTiming, timingsRoot, writePass } from "./test-timings.ts";

/**
 * A reporter that keeps every pass's timings, each test's and each of its steps' (a navigation, a
 * click, an expect's wait, a test.step), always, as every suite keeps its own (test-timings.ts).
 * Recording costs a timestamp per step.
 */

/** A step shorter than this is left out: a slow test's time is in its long steps, and the file stays small. */
export const shortestStepMs = 50;

/** The steps of at least shortestStepMs among steps and those inside them, in the order they ran. */
export function stepTimings(steps: readonly TestStep[], depth = 0): StepTiming[] {
  return steps.flatMap((step) => [
    ...(step.duration >= shortestStepMs
      ? [{ title: step.title, category: step.category, ms: step.duration, depth }]
      : []),
    ...stepTimings(step.steps, depth + 1),
  ]);
}

export default class Timings implements Reporter {
  readonly #suite: string;
  readonly #pass: string;
  readonly #root: string;
  readonly #tests: TestTiming[] = [];
  readonly #started = new Date();

  /** suite is "browser" unless named ("acceptance"); root, the timings' root, is a test's to move. */
  constructor(options: { suite?: string; root?: string } = {}) {
    this.#suite = options.suite ?? "browser";
    this.#pass = process.env["ORPHEUM_E2E_TIMING"] === "1" ? "timed" : "side-by-side";
    this.#root = options.root ?? timingsRoot;
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
    writePass(
      {
        suite: this.#suite,
        pass: this.#pass,
        startedAt: this.#started,
        status: result.status,
        ms: result.duration,
        tests: this.#tests,
      },
      this.#root,
    );
  }

  printsToStdio(): boolean {
    return false;
  }
}
