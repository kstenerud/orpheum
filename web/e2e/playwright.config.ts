// SPDX-License-Identifier: MPL-2.0

import { defineConfig, devices } from "@playwright/test";

import { chromiumArguments, firefoxUserPrefs } from "./browsers.ts";
import { oneBrowserTag, testsOfThisPass } from "./only-these-tests.ts";

/**
 * The browser tier's second pass (`make browser-tier` runs both): only the tests tagged as timed,
 * one at a time, the machine theirs alone; the first runs every other test side by side.
 */
const isTimingPass = process.env["ORPHEUM_E2E_TIMING"] === "1";

/** This pass's tests, ORPHEUM_E2E_ONLY's narrowing kept. */
const pass = testsOfThisPass(process.env["ORPHEUM_E2E_ONLY"], isTimingPass);

/**
 * The first pass's workers, ORPHEUM_E2E_WORKERS to measure another number by. Two (2026-10-18, the
 * whole tier with its build): one took 362 s, two 305 s, three 299 s with a test failed and four 366 s
 * with three, each server making the same films' chunks beside the others.
 */
const sideBySide = Number(process.env["ORPHEUM_E2E_WORKERS"] ?? "2");

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
  // Workers side by side, each with a server of its own (browser-map.ts), a spec's tests in turn on
  // one; a timed test's pass, one, as a time measured beside other work measures the work.
  workers: isTimingPass ? 1 : sideBySide,
  fullyParallel: false,
  forbidOnly: true,
  ...pass,
  retries: 0,
  // The list as it runs, and every test's and step's timings kept (timings.ts).
  reporter: [["list"], ["./timings.ts"]],
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
      // the home acceptance check-pr runs, whose guest is Firefox (agent-failures A24). A test whose
      // claim is the app's runs in Chromium alone (oneBrowserTag). A project's grepInvert replaces
      // the config's, so the pass's goes with it.
      grepInvert: [
        ...(pass.grepInvert ?? []),
        /voice between two fake microphones/u,
        new RegExp(`(?:^|\\s)${oneBrowserTag}(?:\\s|$)`, "u"),
      ],
    },
  ],
});
