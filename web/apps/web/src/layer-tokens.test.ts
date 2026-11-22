// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { declarationsOf, sourceHere, sourcesHere } from "./test-sources.ts";

/** Each z-index a stylesheet gives as a number of its own rather than a layer token, as `file: value`. */
function rawLayers(sheets: readonly (readonly [string, string])[]): string[] {
  return sheets.flatMap(([name, text]) =>
    declarationsOf(text)
      .map((declaration) => /^z-index\s*:\s*(?<value>.+)$/u.exec(declaration.text.trim()))
      .filter((match) => match !== null)
      .map((match) => match.groups?.value.trim() ?? "")
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
    ]);
  });
});
