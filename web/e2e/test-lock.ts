// SPDX-License-Identifier: MPL-2.0

import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

/** Marks a process whose run holds the repository's test lock, so its own sub-runs pass through. */
const LOCK_HELD = "ORPHEUM_GATE_LOCK_HELD";

/**
 * Vitest's and Playwright's global setup: one test run at a time on the repository (the owner,
 * 2026-11-04), however it was started (the owner, 2026-11-07: npx, an editor, a script). A run the
 * gates or the shell hook started holds the lock already; any other takes it here, waiting behind
 * whatever holds it, for as long as this process runs (`scripts/gate_lock.py --for-pid`). The
 * teardown it returns lets the lock go at once, as a watch mode's run would otherwise keep it.
 */
export default function holdTheTestLock(): (() => void) | undefined {
  if (process.env[LOCK_HELD] === "1") {
    return undefined;
  }
  const script = fileURLToPath(new URL("../../scripts/gate_lock.py", import.meta.url));
  const taken = spawnSync("python3", [script, "--for-pid", String(process.pid)], {
    stdio: ["ignore", "pipe", "inherit"],
    encoding: "utf8",
  });
  if (taken.status !== 0) {
    throw new Error(
      `could not take the repository's test lock (${script}): ${String(taken.error ?? "")}`,
    );
  }
  process.env[LOCK_HELD] = "1";
  const keeper = Number(taken.stdout.trim());
  return () => {
    if (keeper > 0) {
      try {
        process.kill(keeper, "SIGTERM");
      } catch {
        // Already gone: the hold ended with its lease, and the lock with it.
      }
    }
  };
}
