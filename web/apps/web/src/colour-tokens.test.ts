// SPDX-License-Identifier: MPL-2.0

import { readdirSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

/** This directory, web/apps/web/src/, whose stylesheets are read. */
const here = new URL("./", import.meta.url);

/**
 * The stylesheets that may write a colour out: the tokens, where every colour is named, and the
 * gallery's made-up posters, which stand in for a film's own art.
 */
const writesColours = new Set(["tokens.css", "ui-gallery.css"]);

/** Each colour a stylesheet writes out rather than naming a token, as `file: colour`. */
function rawColours(sheets: readonly (readonly [string, string])[]): string[] {
  return sheets.flatMap(([name, text]) =>
    [...text.matchAll(/#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?)\([^)]*\)/giu)].map(
      (match) => `${name}: ${match[0]}`,
    ),
  );
}

describe("every colour is a token (D138 §4, D369)", () => {
  it("is written out in tokens.css alone, what is drawn over a film too", () => {
    const sheets = readdirSync(here)
      .filter((name) => name.endsWith(".css") && !writesColours.has(name))
      .map((name) => [name, readFileSync(new URL(name, here), "utf8")] as const);
    expect(sheets.map(([name]) => name)).toContain("player-overlay.css");
    expect(rawColours(sheets)).toEqual([]);
  });

  it("finds a colour written out, so the check above can fail", () => {
    expect(
      rawColours([
        ["a.css", ".a { color: #fff; background: rgb(0 0 0 / 85%); border-color: var(--b); }"],
      ]),
    ).toEqual(["a.css: #fff", "a.css: rgb(0 0 0 / 85%)"]);
  });
});
