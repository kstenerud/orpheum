// SPDX-License-Identifier: MPL-2.0

import { defineConfig } from "vite";

export default defineConfig({
  // Served from https://media.<domain>/cast/ (plans/cut-a.md §2).
  base: "/cast/",
  build: {
    target: "es2023",
  },
});
