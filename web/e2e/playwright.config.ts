// SPDX-License-Identifier: MPL-2.0

import { defineConfig, devices } from "@playwright/test";

import { chromiumArguments, firefoxUserPrefs } from "./browsers.ts";

/**
 * ORPHEUM_E2E_ONLY, a JSON list of tests as Playwright's grep sees them ("chromium shell.spec.ts the
 * logo is…", browser-map.ts's record of each): only those run. Revert-red sets it to the tests a
 * stylesheet's revert can reach (scripts/browser_map.py); unset, every test runs.
 */
function onlyTheseTests(listed: string | undefined): { grep?: RegExp[] } {
  if (listed === undefined || listed === "") {
    return {};
  }
  const tests: unknown = JSON.parse(listed);
  if (!Array.isArray(tests) || !tests.every((one): one is string => typeof one === "string")) {
    throw new Error(`ORPHEUM_E2E_ONLY is ${listed}; want a JSON list of tests`);
  }
  const escape = (text: string): string => text.replaceAll(/[$()*+.?[\\\]^{|}]/gu, String.raw`\$&`);
  return { grep: tests.map((one) => new RegExp(`^${escape(one)}$`, "u")) };
}

/** The browser tier: real browsers against a real server (D217 §4). `make test-browser` runs it. */
export default defineConfig({
  testDir: ".",
  // Browser specs alone: a *.test.ts beside them is a unit test of a helper, vitest's to run.
  testMatch: "*.spec.ts",
  // M4's acceptance runs for two and a half hours, at a milestone's end (D252 §2).
  testIgnore: "acceptance.spec.ts",
  // Traces and screenshots of a failure, beside the dev loop's state and ignored by git.
  outputDir: "../../.dev/browser-results",
  globalSetup: "./server.ts",
  // One browser at a time: the timing is the point, and two prepares at once would share a node.
  workers: 1,
  fullyParallel: false,
  forbidOnly: true,
  ...onlyTheseTests(process.env["ORPHEUM_E2E_ONLY"]),
  retries: 0,
  reporter: [["list"]],
  timeout: 120_000,
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        launchOptions: { args: chromiumArguments },
      },
    },
    {
      name: "firefox",
      use: {
        ...devices["Desktop Firefox"],
        launchOptions: { firefoxUserPrefs },
      },
      // Voice between two Firefoxes is check-pr's slowest browser test; Firefox's voice is held by
      // the home acceptance check-pr runs, whose guest is Firefox (agent-failures A24).
      grepInvert: /voice between two fake microphones/u,
    },
  ],
});
