// SPDX-License-Identifier: MPL-2.0

import { defineConfig, devices } from "@playwright/test";

import { acceptanceMinutes } from "./acceptance-server.ts";
import { chromiumArguments } from "./browsers.ts";

/**
 * M4's acceptance (D252): two browsers, the host in Chrome and the guest in Firefox, for
 * ORPHEUM_ACCEPTANCE_MINUTES under each network profile. `make test-acceptance` runs it, at a
 * milestone's end.
 */
export default defineConfig({
  testDir: ".",
  testMatch: "acceptance.spec.ts",
  outputDir: "../../.dev/acceptance-results",
  globalSetup: "./acceptance-server.ts",
  workers: 1,
  fullyParallel: false,
  forbidOnly: true,
  retries: 0,
  reporter: [["list"], ["./timings.ts", { pass: "acceptance" }]],
  timeout: (acceptanceMinutes() + 10) * 60_000,
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], launchOptions: { args: chromiumArguments } },
    },
  ],
});
