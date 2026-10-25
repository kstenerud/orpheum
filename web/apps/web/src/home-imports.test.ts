// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

/** Every source file of the app, as text, tests left out, by its path. */
const sources = import.meta.glob<string>(["./**/*.{ts,tsx}", "!./**/*.test.{ts,tsx}"], {
  query: "?raw",
  import: "default",
  eager: true,
});

/** What a source imports as it loads: its static imports, never `import type` or `import()`. */
function staticImportsOf(source: string): string[] {
  const imports = source.matchAll(/^import\s+(?!type\s)(?:[^;]*?\sfrom\s+)?"([^"]+)";/gmsu);
  return [...imports].map((each) => each[1] ?? "");
}

/** The app's own path that specifier names from the file at path, or undefined for a package. */
function ownPathOf(path: string, specifier: string): string | undefined {
  if (!specifier.startsWith("./")) {
    return undefined;
  }
  const directory = path.slice(0, path.lastIndexOf("/"));
  return `${directory}/${specifier.slice(2)}`;
}

/** Each of the app's files the home page loads before it draws, by what loaded it first. */
function loadedAtStart(): Map<string, string> {
  const loadedBy = new Map<string, string>([["./client-main.tsx", ""]]);
  const toRead = ["./client-main.tsx"];
  for (let path = toRead.pop(); path !== undefined; path = toRead.pop()) {
    toRead.push(...newlyLoaded(path, loadedBy));
  }
  return loadedBy;
}

/** What path loads that nothing loaded before, noted in loadedBy; the app's own of them, to read. */
function newlyLoaded(path: string, loadedBy: Map<string, string>): string[] {
  const own: string[] = [];
  for (const specifier of staticImportsOf(sources[path] ?? "")) {
    const ownPath = ownPathOf(path, specifier);
    const key = ownPath ?? specifier;
    if (!loadedBy.has(key)) {
      loadedBy.set(key, path);
      own.push(...(ownPath === undefined ? [] : [ownPath]));
    }
  }
  return own;
}

/** How what loaded path came to load it, the client's entry first. */
function chainTo(loadedBy: ReadonlyMap<string, string>, path: string): string {
  const chain = [path];
  for (let by = loadedBy.get(path); by !== undefined && by !== ""; by = loadedBy.get(by)) {
    chain.unshift(by);
  }
  return chain.join(" → ");
}

describe("what the home page loads before it draws (the audit of 2026-10-25, W1)", () => {
  it("is no player, hls.js with it, and no device lab but whether it is offered: their screens load them", () => {
    const loadedBy = loadedAtStart();
    // The client's own reading of its files is complete: it reached the shell and Home.
    expect(loadedBy.has("./app-shell.tsx")).toBe(true);
    expect(loadedBy.has("./home.tsx")).toBe(true);
    const unwanted = [...loadedBy.keys()].filter(
      (path) =>
        path === "@orpheum/player" ||
        path === "hls.js" ||
        (/^\.\/lab[-.]/u.test(path) && path !== "./lab-offered.ts"),
    );
    expect(unwanted.map((path) => chainTo(loadedBy, path))).toEqual([]);
  });
});
