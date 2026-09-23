// SPDX-License-Identifier: MPL-2.0

import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  test: {
    include: ["{packages,apps}/*/src/**/*.test.{ts,tsx}"],
    environment: "node",
    // A committed `.only` would silently skip the rest of the suite.
    allowOnly: false,
    passWithNoTests: false,
  },
});
