// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { onlyTheseTests } from "./only-these-tests.ts";

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
