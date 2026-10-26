// SPDX-License-Identifier: MPL-2.0

import { globSync, readFileSync } from "node:fs";

import react from "@vitejs/plugin-react";
import { configDefaults, defineConfig } from "vitest/config";

import { VitestTimings } from "./e2e/vitest-timings.ts";

// A spawned worker's own startup (its process and the environment it sets up) cost about 334 ms a
// file, 218 of them: with everything run in isolation that is seconds no test asked for
// (test-architecture-2026-10-26.md, finding 4). Vitest's own `isolate: false` reuses a worker
// across the files it runs, at the cost of a worker's globals, its module registry and whatever a
// file leaves mounted or pending surviving into the next file that shares it. Measured
// (2026-10-26): turning it off for the *whole* suite inflated a `console.error` spy's call count
// from 1 to 6 in a file nothing about it touches — a happy-dom file's test had left components
// mounted, whose effects kept firing, and logging, into the next file's worker. A `.ts` file this
// project runs never renders into a DOM (plain `.test.ts`, no `@vitest-environment happy-dom`
// pragma: the one other environment this suite uses, and the only thing that can leave state like
// that behind); each such file, 97 of them, passed with isolation off, repeatedly, shuffled, and
// run one after another in a single worker (the most any file could share). `domTestFiles` is read
// from the pragma itself, not kept as a list, so a file that starts using it moves itself back to
// the isolated project without anyone having to notice and update one.
const HAPPY_DOM_PRAGMA = /@vitest-environment\s+happy-dom/;
const PLAIN_TS_TESTS = "{packages,apps}/*/src/**/*.test.ts";
const plainTsTestFiles = globSync(PLAIN_TS_TESTS, { cwd: import.meta.dirname }).filter(
  (path) => !path.endsWith(".browser.test.ts"),
);
const domTestFiles = plainTsTestFiles.filter((path) =>
  HAPPY_DOM_PRAGMA.test(readFileSync(path, "utf8")),
);
const fastTestFiles = plainTsTestFiles.filter((path) => !domTestFiles.includes(path));

// Vite's own deny list but for its "**/.git/**": revert-red's worktrees and check-pr's tree live
// under .git/, where vitest's module runner could load no test ("Cannot find module"), so every
// unit-test probe failed before a test ran and passed as red (A45, 2026-10-26); the browser-mode
// config has the same. The repository's own .git keeps its objects and top-level files refused.
const FS_DENY = {
  fs: { deny: [".env", ".env.*", "*.{crt,pem}", "**/.git/objects/**", "**/.git/*"] },
};

// A test of what only a real engine does runs in vitest.browser.config.ts's browsers instead.
const EXCLUDE = [...configDefaults.exclude, "**/*.browser.test.{ts,tsx}"];

// Settings every project shares, so the two below differ only in which files and isolation: a
// committed `.only` would silently skip the rest of the suite; make check runs the web tests
// beside the Go tests and every lint at once, and a test that renders a whole page took 5.1 to
// 5.6 s there against 0.2 s alone, failing the default 5 s (test-weight.md §10; a test that hangs
// still fails, at 15 s); transforms are kept on disk between runs, keyed by each file's content:
// every run transformed the whole module graph afresh, and revert-red's 54 runs of the related
// tests took 38 s each, most of it that (check-pr's timings, 2026-10-13); the list as it runs, and
// every test's and file's timings kept (e2e/vitest-timings.ts).
const SHARED = {
  environment: "node" as const,
  allowOnly: false,
  passWithNoTests: false,
  testTimeout: 15_000,
  fsModuleCache: true,
  reporters: ["default", new VitestTimings({ suite: "vitest" })],
};

export default defineConfig({
  plugins: [react()],
  server: FS_DENY,
  test: {
    projects: [
      {
        extends: true,
        test: {
          name: "default",
          // e2e/ holds the browser tier's own helpers, whose logic is tested like any other; a
          // plain .ts file that renders into a DOM (domTestFiles) stays isolated here too.
          include: ["{packages,apps}/*/src/**/*.test.tsx", "e2e/**/*.test.ts", ...domTestFiles],
          exclude: EXCLUDE,
          isolate: true,
          ...SHARED,
        },
      },
      {
        extends: true,
        test: {
          name: "fast",
          include: fastTestFiles,
          exclude: EXCLUDE,
          isolate: false,
          ...SHARED,
        },
      },
    ],
  },
});
