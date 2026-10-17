// SPDX-License-Identifier: MPL-2.0

import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";

import { expect, it } from "vitest";

import { isClientKey } from "./client-keys.ts";

it("keeps every key the client's source names, so a new one is never cleared at the next start", () => {
  const folder = import.meta.dirname;
  const named = new Set<string>();
  for (const file of readdirSync(folder)) {
    if (!/\.tsx?$/u.test(file) || /\.test\.tsx?$/u.test(file)) {
      continue;
    }
    const source = readFileSync(path.join(folder, file), "utf8");
    for (const [, key] of source.matchAll(/["'`](orpheum\.[A-Za-z][A-Za-z.]*)["'`]/gu)) {
      if (key !== undefined && !key.endsWith(".")) {
        named.add(key);
      }
    }
  }
  expect(named.size).toBeGreaterThan(5);
  expect([...named].filter((key) => !isClientKey(key))).toEqual([]);
});

it("clears what an older release kept, and nothing of another's", () => {
  expect(isClientKey("orpheum.discovery.core")).toBe(false);
  expect(isClientKey("orpheum.audioOutput")).toBe(false);
  expect(isClientKey("orpheum.core.k7xq2.session")).toBe(true);
  expect(isClientKey("orpheum.loader.core")).toBe(true);
  expect(isClientKey("orpheum.together.viewer")).toBe(true);
});
