// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { declarationsOf, sourceHere, sourcesHere } from "./test-sources.ts";

/** Each z-index a stylesheet gives as a number of its own rather than a layer token, as `file: value`. */
function rawLayers(sheets: readonly (readonly [string, string])[]): string[] {
  return sheets.flatMap(([name, text]) =>
    declarationsOf(text)
      .map((declaration) => /^z-index\s*:\s*(.+)$/u.exec(declaration.text.trim())?.[1])
      .filter((value) => value !== undefined)
      .map((value) => value.trim())
      .filter((value) => !/^var\(--layer-[a-z-]+\)$/u.test(value))
      .map((value) => `${name}: ${value}`),
  );
}

/** Each transition a stylesheet times by a number of its own rather than a duration token. */
function rawDurations(sheets: readonly (readonly [string, string])[]): string[] {
  return sheets.flatMap(([name, text]) =>
    declarationsOf(text)
      .map((declaration) => declaration.text.trim())
      .filter((text) => /^transition(?:-duration)?\s*:/u.test(text))
      .filter((text) => /(?<![\w-])\d*\.?\d+m?s\b/u.test(text))
      .map((text) => `${name}: ${text}`),
  );
}

/** Each spinner's turn (an animation of keyframes named `…-turn`) timed other than by --duration-spin. */
function rawTurns(sheets: readonly (readonly [string, string])[]): string[] {
  return sheets.flatMap(([name, text]) =>
    declarationsOf(text)
      .map((declaration) => declaration.text.trim())
      .filter((text) => /^animation\s*:.*-turn\b/u.test(text))
      .filter((text) => !text.includes("var(--duration-spin)"))
      .map((text) => `${name}: ${text}`),
  );
}

/** A declaration's value split at its top-level commas, a function's own commas kept. */
function partsOf(value: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let part = "";
  for (const character of value) {
    if (character === "(") {
      depth += 1;
    } else if (character === ")") {
      depth -= 1;
    }
    if (character === "," && depth === 0) {
      parts.push(part.trim());
      part = "";
    } else {
      part += character;
    }
  }
  return [...parts, part.trim()];
}

/**
 * Each animation run once (an arrival, never an endless pulse, whose length is its own) timed other
 * than by a duration token, as `file: animation`.
 */
function rawArrivals(sheets: readonly (readonly [string, string])[]): string[] {
  return sheets.flatMap(([name, text]) =>
    declarationsOf(text)
      .map((declaration) => /^animation\s*:\s*(.+)$/su.exec(declaration.text.trim())?.[1])
      .filter((value) => value !== undefined)
      .flatMap((value) => partsOf(value))
      .filter((part) => part !== "none" && !/\binfinite\b/u.test(part))
      .filter((part) => !part.includes("var(--duration-"))
      .map((part) => `${name}: ${part}`),
  );
}

describe("what lies over what is a layer token (interface rule 11, D538)", () => {
  it("gives every z-index outside tokens.css as a layer token, never a number", () => {
    const sheets = sourcesHere(".css").filter(([name]) => name !== "tokens.css");
    expect(sheets.map(([name]) => name)).toContain("player-overlay.css");
    expect(rawLayers(sheets)).toEqual([]);
  });

  it("finds a z-index written as a number, so the check above can fail", () => {
    expect(
      rawLayers([
        ["a.css", ".a { z-index: 50; } .b { z-index: var(--layer-dialog); } .c { z-index: -1; }"],
      ]),
    ).toEqual(["a.css: 50", "a.css: -1"]);
  });
});

describe("motion is timed by the duration tokens (interface rule 11, D538)", () => {
  it("times every transition by a token", () => {
    const sheets = sourcesHere(".css").filter(([name]) => name !== "tokens.css");
    expect(rawDurations(sheets)).toEqual([]);
  });

  it("finds a transition timed by a number, so the check above can fail", () => {
    expect(
      rawDurations([
        [
          "a.css",
          ".a { transition: opacity 150ms ease-out; } .b { transition: none; } .c { transition: opacity var(--duration-quick); }",
        ],
      ]),
    ).toEqual(["a.css: transition: opacity 150ms ease-out"]);
  });

  it("turns every spinner by --duration-spin", () => {
    const sheets = sourcesHere(".css").filter(([name]) => name !== "tokens.css");
    expect(sheets.map(([name]) => name)).toContain("submit-button.css");
    expect(rawTurns(sheets)).toEqual([]);
  });

  it("finds a spinner turned by a number, so the check above can fail", () => {
    expect(
      rawTurns([
        [
          "a.css",
          ".a { animation: a-turn 1s linear infinite; } .b { animation: b-turn var(--duration-spin) linear infinite; } .c { animation: c-fade 2s; }",
        ],
      ]),
    ).toEqual(["a.css: animation: a-turn 1s linear infinite"]);
  });

  it("zeroes every duration where less motion is asked for", () => {
    const reduced = declarationsOf(sourceHere("tokens.css")).filter(
      (declaration) =>
        declaration.selector === ":root" &&
        declaration.within.some((rule) => rule.includes("prefers-reduced-motion: reduce")),
    );
    expect(reduced.map((declaration) => declaration.text.trim())).toEqual([
      "--duration-quick: 0ms",
      "--duration-normal: 0ms",
      "--duration-slow: 0ms",
      "--duration-spin: 0ms",
    ]);
  });

  it("times every animation run once by a token, as the title's loading line fades in", () => {
    const sheets = sourcesHere(".css").filter(([name]) => name !== "tokens.css");
    expect(sheets.map(([name]) => name)).toContain("title.css");
    expect(rawArrivals(sheets)).toEqual([]);
  });

  it("finds an animation run once timed by a number, so the check above can fail", () => {
    expect(
      rawArrivals([
        [
          "a.css",
          ".a { animation: a-appear 0.2s ease-out 0.4s both, a-pulse 1.6s ease-in-out infinite; } .b { animation: b-rise var(--duration-slow) cubic-bezier(0.2, 0, 0, 1); } .c { animation: none; }",
        ],
      ]),
    ).toEqual(["a.css: a-appear 0.2s ease-out 0.4s both"]);
  });
});
