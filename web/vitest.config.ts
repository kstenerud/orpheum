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
  },
});
