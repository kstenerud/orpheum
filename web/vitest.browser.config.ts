// SPDX-License-Identifier: MPL-2.0

import react from "@vitejs/plugin-react";
import { playwright } from "@vitest/browser-playwright";
import { searchForWorkspaceRoot } from "vite";
import { defineConfig } from "vitest/config";
import type { BrowserCommand } from "vitest/node";

import { chromiumArguments, firefoxUserPrefs } from "./e2e/browsers.ts";
import { VitestTimings } from "./e2e/vitest-timings.ts";

/**
 * The browser-mode tests (*.browser.test.tsx): what only a real engine does (layout, a stylesheet,
 * WebRTC), in Chromium and Firefox on the pinned Playwright, with no server, each a component or a
 * module alone (the owner, 2026-10-18). The browser tier keeps what needs the server, the loader or
 * two clients. `make test-web-browser` runs them; vitest.config.ts runs every other test.
 */
/** Where a failure's screenshot goes, beside the browser tier's results and ignored by git. */
const attachmentsDir = "../.dev/browser-mode-results";

/**
 * What a frame kept by `keeper` (keepTheFrameFocused's source, e2e/frame-focus.ts) has after another
 * page opens: its focused button's `:focus` and `:focus-visible`, and the clicks an Enter on it makes.
 * The frame is in a page of the command's own, opened last, as Firefox takes the focus only from the
 * page opened before the new one. The scripts are strings: this file is type-checked without the DOM.
 */
const focusAfterAPageOpens: BrowserCommand<[keeper: string]> = async ({ page }, keeper) => {
  const browser = page.context().browser();
  if (browser === null) {
    throw new Error("the test page has no browser to open another page in");
  }
  const own = await browser.newContext();
  const other = await browser.newContext();
  try {
    const tab = await own.newPage();
    await tab.setContent(`<iframe name="kept" srcdoc="<button>focused</button>"></iframe>`);
    const frame = tab.frame("kept");
    if (frame === null) {
      throw new Error("the command's page has no frame");
    }
    await frame.evaluate(`(${keeper})(window)`);
    await frame.evaluate(`{
      const button = document.querySelector("button");
      button.addEventListener("click", () => {
        document.body.dataset.clicks = String(Number(document.body.dataset.clicks ?? 0) + 1);
      });
      window.focus();
      button.focus({ focusVisible: true });
    }`);
    await other.newPage();
    const isFocused = await frame.evaluate<boolean>(`document.activeElement.matches(":focus")`);
    const isFocusShown = await frame.evaluate<boolean>(
      `document.activeElement.matches(":focus-visible")`,
    );
    await tab.keyboard.press("Enter");
    const clicks = await frame.evaluate<number>(`Number(document.body.dataset.clicks ?? 0)`);
    return { isFocused, isFocusShown, clicks };
  } finally {
    await other.close();
    await own.close();
  }
};

export default defineConfig({
  plugins: [react()],
  // Vite's own deny list but for its "**/.git/**": revert-red's worktrees and check-pr's tree live
  // under .git/, where it refused every test file, so each browser-mode probe there failed before a
  // test ran and passed as red (2026-10-26). The repository's own .git keeps its objects and its
  // top-level files (config, index, HEAD) refused.
  // A failure's screenshot is written through the server too, and its folder is outside web/, the
  // workspace root Vite serves alone by default: every one was refused, on CI as here (2026-11-11).
  server: {
    fs: {
      allow: [searchForWorkspaceRoot(process.cwd()), attachmentsDir],
      deny: [".env", ".env.*", "*.{crt,pem}", "**/.git/objects/**", "**/.git/*"],
    },
  },
  test: {
    include: ["{packages,apps}/*/src/**/*.browser.test.{ts,tsx}", "e2e/*.browser.test.ts"],
    // Each test file's frame keeps its focus while other pages open (e2e/frame-focus.ts).
    setupFiles: ["./e2e/frame-focus-setup.ts"],
    allowOnly: false,
    passWithNoTests: false,
    // The repository's test lock, taken however the run was started (e2e/test-lock.ts).
    globalSetup: ["./e2e/test-lock.ts"],
    // A walk presses keys through the browser one round trip at a time: 3 to 4 s alone, past 15 s on
    // a machine other gates load (test-weight.md §26). Its polls wait as long as it may need.
    testTimeout: 30_000,
    expect: { poll: { timeout: 10_000 } },
    fsModuleCache: true,
    // The list as it runs, and every test's and file's timings kept (e2e/vitest-timings.ts).
    reporters: ["default", new VitestTimings({ suite: "vitest-browser" })],
    attachmentsDir,
    browser: {
      enabled: true,
      headless: true,
      provider: playwright(),
      commands: { focusAfterAPageOpens },
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
