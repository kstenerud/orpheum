// SPDX-License-Identifier: MPL-2.0

import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv, type Plugin } from "vite";

import { agentHelpFiles, agentHelpSource } from "./src/agent-help";
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

/**
 * Writes the setup help for a person's AI agent beside the client's files, from its one source
 * under docs/users/, and /llms.txt naming it (src/agent-help.ts): the core and the loader serve them
 * as they serve the client's other files.
 */
function agentHelp(): Plugin {
  return {
    name: "orpheum-agent-help",
    apply: "build",
    generateBundle() {
      const source = readFileSync(path.join(repoRoot, agentHelpSource), "utf8");
      for (const file of agentHelpFiles(source)) {
        this.emitFile({ type: "asset", fileName: file.fileName, source: file.source });
      }
    },
  };
}

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
    plugins: [react(), recordChunkModules(mapDirectory, "client"), agentHelp()],
    // No .env files: the configuration is what the process was given (D204).
    envDir: false,
    build: {
      target: "es2023",
      rolldownOptions: {
        output: {
          codeSplitting: {
            // What a film's page reads of the sound before it plays (the player's heard-volume.ts)
            // is a chunk of its own. Left to Rolldown it joins the chunk of the player the page
            // imports only as Play nears, and the page would wait on hls.js for it (the web
            // client's architecture audit of 2026-11-10, Playback 2).
            groups: [{ name: "heard-volume", test: /packages\/player\/src\/heard-volume\.ts$/ }],
          },
        },
      },
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
