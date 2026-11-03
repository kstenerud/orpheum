// SPDX-License-Identifier: MPL-2.0

import { defineConfig } from "vite";

export default defineConfig({
  // Served from https://media.<domain>/cast/ (plans/cut-a.md §2).
  base: "/cast/",
  build: {
    // The 3rd-generation Chromecast's Cast firmware runs Chromium 90 (browser-support-2026-10-09.md),
    // the oldest engine a receiver meets: its syntax is lowered to it, and src/old-engine.ts fills
    // the few methods it lacks that the bundle calls.
    target: "chrome90",
    // Built inside the web client's dist, which the binary embeds and serves (make build copies
    // web/apps/web/dist), so the core serves the receiver under /cast/ (internal/core/webclient).
    // The web client builds first (web/package.json's build), so its emptying leaves this be.
    outDir: "../web/dist/cast",
    emptyOutDir: true,
  },
});
