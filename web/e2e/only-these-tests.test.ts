// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { onlyTheseTests, testsOfThisPass } from "./only-these-tests.ts";

/** Whether the listed entries let Playwright run the test it greps as title. */
function runs(listed: readonly string[], title: string): boolean {
  return (onlyTheseTests(JSON.stringify(listed)).grep ?? []).some((pattern) => pattern.test(title));
}

describe("the browser tests a run is narrowed to", () => {
  it("is each listed test exactly, and nothing like it", () => {
    const logo = "chromium shell.spec.ts the logo is one link home";
    expect(runs([logo], logo)).toBe(true);
    expect(runs([logo], "firefox shell.spec.ts the logo is one link home")).toBe(false);
    expect(runs(["chromium shell.spec.ts the logo"], logo)).toBe(false);
  });

  it("is every test of a listed spec, under every browser, and no other spec's", () => {
    expect(runs(["shell.spec.ts"], "chromium shell.spec.ts the logo is one link home")).toBe(true);
    expect(runs(["shell.spec.ts"], "firefox shell.spec.ts a new test")).toBe(true);
    expect(runs(["shell.spec.ts"], "chromium subtitles.spec.ts shell.spec.ts named")).toBe(false);
  });

  it("is every test when unset, and refuses what is not a list of tests", () => {
    expect(onlyTheseTests(undefined)).toEqual({});
    expect(() => onlyTheseTests('{"a": 1}')).toThrow("want a JSON list of tests");
  });
});

/** Whether a pass of the browser tier, narrowed to listed (all when undefined), runs the test title. */
function passRuns(
  listed: readonly string[] | undefined,
  isTimingPass: boolean,
  title: string,
): boolean {
  const pass = testsOfThisPass(
    listed === undefined ? undefined : JSON.stringify(listed),
    isTimingPass,
  );
  const isGrepped = pass.grep === undefined || pass.grep.some((pattern) => pattern.test(title));
  return isGrepped && !(pass.grepInvert ?? []).some((pattern) => pattern.test(title));
}

describe("the browser tier's two passes", () => {
  const timed = "chromium play-timing.spec.ts plays within 8 s @timing";
  const untimed = "chromium shell.spec.ts the logo is one link home";

  it("runs the timed tests in the second pass alone, and every other in the first", () => {
    expect([passRuns(undefined, false, untimed), passRuns(undefined, false, timed)]).toEqual([
      true,
      false,
    ]);
    expect([passRuns(undefined, true, untimed), passRuns(undefined, true, timed)]).toEqual([
      false,
      true,
    ]);
    expect(passRuns(undefined, true, "chromium a.spec.ts named for @timings")).toBe(false);
  });

  it("keeps a run's narrowing in each pass", () => {
    expect(passRuns([timed], true, timed)).toBe(true);
    expect(passRuns([untimed], true, timed)).toBe(false);
    expect(passRuns(["play-timing.spec.ts"], true, timed)).toBe(true);
    expect(passRuns(["play-timing.spec.ts"], false, timed)).toBe(false);
    expect(passRuns(["shell.spec.ts"], false, untimed)).toBe(true);
    expect(passRuns(["subtitles.spec.ts"], false, untimed)).toBe(false);
  });
});
