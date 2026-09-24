// SPDX-License-Identifier: MPL-2.0

import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv } from "vite";

import { devAllowedHosts } from "./src/dev-hosts";

// In `make dev` the web client runs on Vite's dev server with hot reload, and everything that
// is the server's — the API, media bytes and the Watch Together socket — is proxied to the Go
// server the dev loop started. scripts/dev.py sets ORPHEUM_DEV_SERVER; loadEnv reads it from
// the environment.
const defaultDevServer = "http://127.0.0.1:8432";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "ORPHEUM_");
  const devServer = env["ORPHEUM_DEV_SERVER"] ?? defaultDevServer;
  return {
    plugins: [react()],
    build: {
      target: "es2023",
    },
    server: {
      // `make dev DEV_LISTEN=0.0.0.0:8432` opens Vite to the LAN; a name to reach it by goes in
      // ORPHEUM_DEV_ALLOWED_HOSTS.
      allowedHosts: devAllowedHosts(env["ORPHEUM_DEV_ALLOWED_HOSTS"]),
      proxy: {
        "/api": { target: devServer, ws: true },
        "/media": { target: devServer },
        "/cast": { target: devServer },
        "/debug": { target: devServer },
      },
    },
  };
});
