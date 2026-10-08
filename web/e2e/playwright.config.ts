// SPDX-License-Identifier: MPL-2.0

import { defineConfig, devices } from "@playwright/test";

import { chromiumArguments, firefoxUserPrefs } from "./browsers.ts";

/** The browser tier: real browsers against a real server (D217 §4). `make test-browser` runs it. */
export default defineConfig({
  testDir: ".",
  // M4's acceptance runs for an hour and a half, at a milestone's end (D252 §2).
  testIgnore: "acceptance.spec.ts",
  // Traces and screenshots of a failure, beside the dev loop's state and ignored by git.
  outputDir: "../../.dev/browser-results",
  globalSetup: "./server.ts",
  // One browser at a time: the timing is the point, and two prepares at once would share a node.
  workers: 1,
  fullyParallel: false,
  forbidOnly: true,
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
