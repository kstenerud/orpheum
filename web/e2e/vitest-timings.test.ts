// SPDX-License-Identifier: MPL-2.0

import { mkdtempSync, readdirSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import type { TestModule } from "vitest/node";

import { VitestTimings } from "./vitest-timings.ts";

const scratches: string[] = [];

afterEach(() => {
  for (const scratch of scratches.splice(0)) {
    rmSync(scratch, { recursive: true, force: true });
  }
});

/** A module as vitest hands it over at a run's end: only what the reporter reads. */
function moduleOf(file: string, loadMs: number, tests: { name: string; ms: number }[]): TestModule {
  const project = { name: "vitest" };
  return {
    relativeModuleId: file,
    project,
    children: {
      allTests: () =>
        tests.map((test) => ({
          fullName: test.name,
          project,
          location: { line: 12, column: 3 },
          result: () => ({ state: "passed" }),
          diagnostic: () => ({ duration: test.ms }),
        })),
    },
    diagnostic: () => ({
      environmentSetupDuration: 10,
      prepareDuration: 20,
      collectDuration: loadMs,
      setupDuration: 30,
      duration: 40,
    }),
  } as unknown as TestModule;
}

/** A kept run, as the reporter writes it. */
interface KeptRun {
  readonly suite: string;
  readonly tests: unknown;
  readonly files: unknown;
}

describe("vitest's timings (the owner, 2026-10-20)", () => {
  it("keeps each test's time and each file's whole time, its loading apart, in its suite's own directory", () => {
    const root = mkdtempSync(path.join(tmpdir(), "vitest-timings-"));
    scratches.push(root);
    new VitestTimings({ suite: "vitest", root }).onTestRunEnd(
      [moduleOf("apps/web/src/play.test.tsx", 3_100, [{ name: "play > starts", ms: 250 }])],
      [],
      "passed",
    );
    const [written] = readdirSync(path.join(root, "vitest"));
    const kept = JSON.parse(
      readFileSync(path.join(root, "vitest", written ?? ""), "utf8"),
    ) as KeptRun;
    expect(kept.suite).toBe("vitest");
    expect(kept.tests).toEqual([
      {
        test: "play > starts",
        project: "vitest",
        file: "apps/web/src/play.test.tsx",
        line: 12,
        status: "passed",
        ms: 250,
        worker: 0,
        steps: [],
      },
    ]);
    expect(kept.files).toEqual([
      { file: "apps/web/src/play.test.tsx", project: "vitest", ms: 3_200, loadMs: 3_100 },
    ]);
  });
});
