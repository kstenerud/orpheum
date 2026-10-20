// SPDX-License-Identifier: MPL-2.0

import { mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import type { FullResult, TestCase, TestResult, TestStep } from "@playwright/test/reporter";
import { afterEach, describe, expect, it } from "vitest";

import Timings, { keepNewest, keptPasses } from "./timings.ts";

const scratches: string[] = [];

afterEach(() => {
  for (const scratch of scratches.splice(0)) {
    rmSync(scratch, { recursive: true, force: true });
  }
});

function scratch(): string {
  const made = mkdtempSync(path.join(tmpdir(), "timings-"));
  scratches.push(made);
  return made;
}

/** A step as Playwright hands it over: only what the reporter reads. */
function step(title: string, category: string, duration: number): TestStep {
  return { title, category, duration, steps: [] } as unknown as TestStep;
}

/** A step with the steps inside it. */
function around(outer: TestStep, inner: TestStep[]): TestStep {
  return { ...outer, steps: inner };
}

/** A kept pass, as the reporter writes it. */
interface KeptPass {
  readonly pass: string;
  readonly tests: unknown;
}

describe("the browser tier's timings (the owner, 2026-10-20)", () => {
  it("keeps each test's time and its long steps, however deep, with the pass they ran in", () => {
    const directory = scratch();
    const reporter = new Timings({ pass: "side-by-side", directory });
    const test = {
      title: "a guest joins",
      parent: { project: () => ({ name: "firefox" }) },
      location: { file: "/home/karl/orpheum/web/e2e/together.spec.ts", line: 70 },
    } as unknown as TestCase;
    const result = {
      status: "passed",
      duration: 15_700,
      workerIndex: 1,
      steps: [
        around(step("Before Hooks", "hook", 7_500), [step('Fixture "server"', "fixture", 2_500)]),
        step("Click", "pw:api", 20),
        step('Expect "poll toBe"', "expect", 10_100),
      ],
    } as unknown as TestResult;
    reporter.onTestEnd(test, result);
    reporter.onEnd({ status: "passed", duration: 16_000 } as FullResult);

    const [written] = readdirSync(directory);
    expect(written).toMatch(/-side-by-side\.json$/u);
    const kept = JSON.parse(readFileSync(path.join(directory, written ?? ""), "utf8")) as KeptPass;
    expect(kept.pass).toBe("side-by-side");
    expect(kept.tests).toEqual([
      {
        test: "a guest joins",
        project: "firefox",
        file: "together.spec.ts",
        line: 70,
        status: "passed",
        ms: 15_700,
        worker: 1,
        // The 20 ms click is too short to keep.
        steps: [
          { title: "Before Hooks", category: "hook", ms: 7_500, depth: 0 },
          { title: 'Fixture "server"', category: "fixture", ms: 2_500, depth: 1 },
          { title: 'Expect "poll toBe"', category: "expect", ms: 10_100, depth: 0 },
        ],
      },
    ]);
  });

  it("keeps only the newest passes", () => {
    const directory = scratch();
    for (let index = 0; index < keptPasses + 3; index += 1) {
      writeFileSync(
        path.join(directory, `2026-10-20T20-${String(index).padStart(2, "0")}-side-by-side.json`),
        "{}",
      );
    }
    keepNewest(directory, keptPasses);
    const left = readdirSync(directory).sort();
    expect(left).toHaveLength(keptPasses);
    expect(left[0]).toBe("2026-10-20T20-03-side-by-side.json");
  });
});
