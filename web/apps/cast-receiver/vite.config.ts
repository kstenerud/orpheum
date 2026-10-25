// SPDX-License-Identifier: MPL-2.0

import { defineConfig } from "vite";

export default defineConfig({
  // Served from https://media.<domain>/cast/ (plans/cut-a.md §2).
  base: "/cast/",
  build: {
    target: "es2023",
    // Built inside the web client's dist, which the binary embeds and serves (make build copies
    // web/apps/web/dist), so the core serves the receiver under /cast/ (internal/core/webclient).
    // The web client builds first (web/package.json's build), so its emptying leaves this be.
    outDir: "../web/dist/cast",
    emptyOutDir: true,
  },
});
