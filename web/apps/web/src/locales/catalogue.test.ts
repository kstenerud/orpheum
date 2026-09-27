// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { en } from "./en.ts";

/** Every source file of the app, as text, tests left out. */
const sources = import.meta.glob<string>(["../**/*.{ts,tsx}", "!../**/*.test.{ts,tsx}"], {
  query: "?raw",
  import: "default",
  eager: true,
});

/** The keys a source file asks the translation layer for by name, as t("a.b"). */
function keysAskedFor(source: string): string[] {
  return [...source.matchAll(/\bt\(\s*"([^"]+)"/gu)].flatMap((match) =>
    match[1] === undefined ? [] : [match[1]],
  );
}

/** Whether the catalogue has text at a dotted key. */
function hasText(catalogue: unknown, key: string): boolean {
  let node = catalogue;
  for (const part of key.split(".")) {
    if (typeof node !== "object" || node === null) {
      return false;
    }
    node = new Map<string, unknown>(Object.entries(node)).get(part);
  }
  return typeof node === "string";
}

describe("the English catalogue", () => {
  it("finds the keys a file asks for", () => {
    expect(keysAskedFor('t("a.b"); x.t("c"); at("d"); t(\n  "e.f", {})')).toEqual([
      "a.b",
      "c",
      "e.f",
    ]);
  });

  it("has text for every key the app asks for by name, so none shows as its key (D51)", () => {
    const missing = Object.entries(sources).flatMap(([file, source]) =>
      keysAskedFor(source)
        .filter((key) => !hasText(en, key))
        .map((key) => `${file}: ${key}`),
    );
    expect(Object.keys(sources).length).toBeGreaterThan(20);
    expect(missing).toEqual([]);
  });
});
