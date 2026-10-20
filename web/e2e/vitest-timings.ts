// SPDX-License-Identifier: MPL-2.0

import type { Reporter, TestModule, TestRunEndReason } from "vitest/node";

import { type FileTiming, type TestTiming, timingsRoot, writePass } from "./test-timings.ts";

/**
 * A vitest reporter that keeps every run's timings, each test's and each file's, its loading
 * included, always, as every suite keeps its own (test-timings.ts). Both vitest configs use it, each
 * as its own suite.
 */
export class VitestTimings implements Reporter {
  readonly #suite: string;
  readonly #root: string;
  readonly #started = new Date();

  /** root, the timings' root, is a test's to move. */
  constructor(options: { suite: string; root?: string }) {
    this.#suite = options.suite;
    this.#root = options.root ?? timingsRoot;
  }

  onTestRunEnd(
    testModules: readonly TestModule[],
    _unhandledErrors: readonly unknown[],
    reason: TestRunEndReason,
  ): void {
    writePass(
      {
        suite: this.#suite,
        pass: "run",
        startedAt: this.#started,
        status: reason,
        ms: Date.now() - this.#started.getTime(),
        tests: testModules.flatMap(testsOf),
        files: testModules.map(fileOf),
      },
      this.#root,
    );
  }
}

/** The module's tests, each with its own time. */
function testsOf(module: TestModule): TestTiming[] {
  return [...module.children.allTests()].map((test) => ({
    test: test.fullName,
    project: test.project.name,
    file: module.relativeModuleId,
    line: test.location?.line ?? 0,
    status: test.result().state,
    ms: test.diagnostic()?.duration ?? 0,
    worker: 0,
    steps: [],
  }));
}

/** The module's whole time: its environment, loading, setup and tests. */
function fileOf(module: TestModule): FileTiming {
  const spent = module.diagnostic();
  return {
    file: module.relativeModuleId,
    project: module.project.name,
    ms:
      spent.environmentSetupDuration +
      spent.prepareDuration +
      spent.collectDuration +
      spent.setupDuration +
      spent.duration,
    loadMs: spent.collectDuration,
  };
}
