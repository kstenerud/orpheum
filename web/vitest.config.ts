// SPDX-License-Identifier: MPL-2.0

import react from "@vitejs/plugin-react";
import { configDefaults, defineConfig } from "vitest/config";

import { VitestTimings } from "./e2e/vitest-timings.ts";

export default defineConfig({
  plugins: [react()],
  // Vite's own deny list but for its "**/.git/**": revert-red's worktrees and check-pr's tree live
  // under .git/, where vitest's module runner could load no test ("Cannot find module"), so every
  // unit-test probe failed before a test ran and passed as red (A45, 2026-10-26); the browser-mode
  // config has the same. The repository's own .git keeps its objects and top-level files refused.
  server: { fs: { deny: [".env", ".env.*", "*.{crt,pem}", "**/.git/objects/**", "**/.git/*"] } },
  test: {
    // e2e/ holds the browser tier's own helpers, whose logic is tested like any other.
    include: ["{packages,apps}/*/src/**/*.test.{ts,tsx}", "e2e/**/*.test.ts"],
    // A test of what only a real engine does runs in vitest.browser.config.ts's browsers instead.
    exclude: [...configDefaults.exclude, "**/*.browser.test.{ts,tsx}"],
    environment: "node",
    // A committed `.only` would silently skip the rest of the suite.
    allowOnly: false,
    passWithNoTests: false,
    // make check runs the web tests beside the Go tests and every lint at once, and a test that
    // renders a whole page took 5.1 to 5.6 s there against 0.2 s alone, failing the default 5 s
    // (docs/contributors/test-weight.md §10). A test that hangs still fails, at 15 s.
    testTimeout: 15_000,
    // Transforms kept on disk between runs, keyed by each file's content: every run transformed the
    // whole module graph afresh, and revert-red's 54 runs of the related tests took 38 s each, most of
    // it that (check-pr's timings, 2026-10-13).
    fsModuleCache: true,
    // The list as it runs, and every test's and file's timings kept (e2e/vitest-timings.ts).
    reporters: ["default", new VitestTimings({ suite: "vitest" })],
  },
});
