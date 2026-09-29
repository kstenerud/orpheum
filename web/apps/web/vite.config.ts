// SPDX-License-Identifier: MPL-2.0

import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

import { devAllowedHosts } from "./src/dev-hosts";
import { devServerOrigin } from "./src/dev-server";

// In `make dev` the web client runs on Vite's dev server with hot reload, and everything that
// is the server's — the API, media bytes and the Watch Together socket — is proxied to the Go
// server the dev loop started. scripts/dev.py sets ORPHEUM_DEV_SERVER in the process
// environment. Nothing is read from .env files (D204).
export default defineConfig(({ command, mode }) => {
  // envDir false: the process environment only, never .env files.
  const env = loadEnv(mode, false, "ORPHEUM_");
  const target = devServerOrigin(env["ORPHEUM_DEV_SERVER"], command);
  return {
    plugins: [react()],
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
