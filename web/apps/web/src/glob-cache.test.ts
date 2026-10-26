// SPDX-License-Identifier: MPL-2.0

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

/** Every source under directory, by its path. */
function sourcesUnder(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name.startsWith(".") ? [] : sourcesUnder(path);
    }
    return /\.tsx?$/u.test(entry.name) ? [path] : [];
  });
}

/** web/, from this file at web/apps/web/src/. */
const web = fileURLToPath(new URL("../../../", import.meta.url));

/** A glob given its type at the call, spelt in parts so this file does not hold it. */
const typedGlob = ["import", "meta", "glob<"].join(".");

describe("a glob of files (2026-10-26)", () => {
  it("is written so vitest's kept transforms leave its file alone", () => {
    // vitest keeps no transform of a file holding "import.meta.glob(", whose matches change as files
    // come and go; a glob given its type at the call escaped that, and idempotent.test.ts read a list of
    // sources from before cast.ts, passing in the main tree and failing in every new worktree.
    const typed = [
      ...sourcesUnder(join(web, "apps")),
      ...sourcesUnder(join(web, "packages")),
    ].filter((path) => readFileSync(path, "utf8").includes(typedGlob));
    expect(typed).toEqual([]);
  });
});
