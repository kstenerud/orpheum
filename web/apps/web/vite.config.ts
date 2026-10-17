// SPDX-License-Identifier: MPL-2.0

import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv, type Plugin } from "vite";

import { chunkModules } from "./src/chunk-modules";
import { devAllowedHosts } from "./src/dev-hosts";

const repoRoot = fileURLToPath(new URL("../../..", import.meta.url));

/**
 * Writes which of the repository's files each built chunk holds into directory, for the browser
 * map (src/chunk-modules.ts); nothing when no directory is given, as outside check-pr's browser run.
 */
function recordChunkModules(directory: string | undefined, build: string): Plugin {
  return {
    name: "orpheum-chunk-modules",
    apply: "build",
    writeBundle(_options, bundle) {
      if (directory === undefined || directory === "") {
        return;
      }
      mkdirSync(directory, { recursive: true });
      writeFileSync(
        path.join(directory, `modules-${build}.json`),
        JSON.stringify(chunkModules(bundle, repoRoot)),
      );
    },
  };
}
import { devServerOrigin } from "./src/dev-server";

// The WebRTC path's service worker (src/sw.ts), built on its own into dist/sw.js beside the app: one
// self-contained classic script, since a module worker is not run by every browser the client
// supports (D271), and a script shared with the app would make it one.
const serviceWorkerBuild = (mapDirectory: string | undefined) => ({
  envDir: false as const,
  plugins: [recordChunkModules(mapDirectory, "sw")],
  build: {
    target: "es2023",
    emptyOutDir: false,
    lib: {
      entry: "src/sw.ts",
      formats: ["iife" as const],
      name: "orpheumServiceWorker",
      fileName: () => "sw.js",
    },
  },
});

// In `make dev` the web client runs on Vite's dev server with hot reload, and everything that
// is the server's — the API, media bytes and the Watch Together socket — is proxied to the Go
// server the dev loop started. scripts/dev.py sets ORPHEUM_DEV_SERVER in the process
// environment. Nothing is read from .env files (D204).
export default defineConfig(({ command, mode }) => {
  // envDir false: the process environment only, never .env files.
  const env = loadEnv(mode, false, "ORPHEUM_");
  const mapDirectory = env["ORPHEUM_BROWSER_MAP_DIR"];
  if (mode === "sw") {
    return serviceWorkerBuild(mapDirectory);
  }
  const target = devServerOrigin(env["ORPHEUM_DEV_SERVER"], command);
  return {
    plugins: [react(), recordChunkModules(mapDirectory, "client")],
    // No .env files: the configuration is what the process was given (D204).
    envDir: false,
    build: {
      target: "es2023",
    },
    server: {
      // `make dev DEV_LISTEN=0.0.0.0:8432` opens Vite to the LAN; a name to reach it by goes in
      // ORPHEUM_DEV_ALLOWED_HOSTS.
      allowedHosts: devAllowedHosts(env["ORPHEUM_DEV_ALLOWED_HOSTS"]),
      proxy:
        target === undefined
          ? {}
          : {
              "/api": { target, ws: true },
              "/media": { target },
              "/cast": { target },
              "/debug": { target },
            },
    },
  };
});
