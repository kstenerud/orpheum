// SPDX-License-Identifier: MPL-2.0

import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  test: {
    // e2e/ holds the browser tier's own helpers, whose logic is tested like any other.
    include: ["{packages,apps}/*/src/**/*.test.{ts,tsx}", "e2e/**/*.test.ts"],
    environment: "node",
    // A committed `.only` would silently skip the rest of the suite.
    allowOnly: false,
    passWithNoTests: false,
    // make check runs the web tests beside the Go tests and every lint at once, and a test that
    // renders a whole page took 5.1 to 5.6 s there against 0.2 s alone, failing the default 5 s
    // (docs/contributors/test-weight.md §10). A test that hangs still fails, at 15 s.
    testTimeout: 15_000,
  },
});
