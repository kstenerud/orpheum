// SPDX-License-Identifier: MPL-2.0

/**
 * The client's stylesheets against its code. The shell's layout, which only a stylesheet decides,
 * is app-shell.browser.test.tsx's, in vitest's browser mode (the owner, 2026-10-18).
 */

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { expect } from "@playwright/test";

import { readsStylesheets, test } from "./browser-map.ts";

/** The client's source folder, beside this one. */
const clientSource = join(import.meta.dirname, "..", "apps", "web", "src");

test(
  "every class the client's stylesheets select is one its code draws",
  { tag: readsStylesheets },
  () => {
    const files = readdirSync(clientSource);
    const code = files
      .filter((name) => /\.tsx?$/u.test(name) && !/\.test\.tsx?$/u.test(name))
      .map((name) => readFileSync(join(clientSource, name), "utf8"))
      .join("\n");
    const unused: string[] = [];
    for (const sheet of files.filter((name) => name.endsWith(".css"))) {
      const selected = readFileSync(join(clientSource, sheet), "utf8").matchAll(
        /\.([a-z][a-z0-9-]*(?:__[a-z0-9-]+)?(?:--[a-z0-9-]+)?)/gu,
      );
      for (const [, name] of selected) {
        // A rule for a class nothing draws is dead: kept, it would say a look the page never has.
        if (name !== undefined && !code.includes(name)) {
          unused.push(`${sheet}: .${name}`);
        }
      }
    }
    expect(unused).toEqual([]);
  },
);
