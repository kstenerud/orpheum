// SPDX-License-Identifier: MPL-2.0

import react from "@vitejs/plugin-react";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig } from "vitest/config";

import { chromiumArguments, firefoxUserPrefs } from "./e2e/browsers.ts";
import { VitestTimings } from "./e2e/vitest-timings.ts";

/**
 * The browser-mode tests (*.browser.test.tsx): what only a real engine does (layout, a stylesheet,
 * WebRTC), in Chromium and Firefox on the pinned Playwright, with no server, each a component or a
 * module alone (the owner, 2026-10-18). The browser tier keeps what needs the server, the loader or
 * two clients. `make test-web-browser` runs them; vitest.config.ts runs every other test.
 */
export default defineConfig({
  plugins: [react()],
  test: {
    include: ["{packages,apps}/*/src/**/*.browser.test.{ts,tsx}"],
    allowOnly: false,
    passWithNoTests: false,
    // A walk presses keys through the browser one round trip at a time: 3 to 4 s alone, past 15 s on
    // a machine other gates load (test-weight.md §26). Its polls wait as long as it may need.
    testTimeout: 30_000,
    expect: { poll: { timeout: 10_000 } },
    fsModuleCache: true,
    // The list as it runs, and every test's and file's timings kept (e2e/vitest-timings.ts).
    reporters: ["default", new VitestTimings({ suite: "vitest-browser" })],
    // A failure's screenshot, beside the browser tier's results and ignored by git.
    attachmentsDir: "../.dev/browser-mode-results",
    browser: {
      enabled: true,
      headless: true,
      provider: playwright(),
      // The browser tier's own launch options, so a page meets what it meets there.
      instances: [
        {
          browser: "chromium",
          provider: playwright({ launchOptions: { args: chromiumArguments } }),
        },
        { browser: "firefox", provider: playwright({ launchOptions: { firefoxUserPrefs } }) },
      ],
    },
  },
});
