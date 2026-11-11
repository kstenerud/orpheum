// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { declarationsOf, sourceHere, sourcesHere } from "./test-sources.ts";

/** Each length against the whole large viewport, `100vh`, as `file: declaration`. */
function largeViewportHeights(sheets: readonly (readonly [string, string])[]): string[] {
  return sheets.flatMap(([name, sheet]) =>
    declarationsOf(sheet)
      .filter((declaration) => /(?<![\w.])100vh\b/u.test(declaration.text))
      .map((declaration) => `${name}: ${declaration.text}`),
  );
}

describe("a height against the screen", () => {
  it("is what the screen shows, a phone's toolbars' room taken (100dvh, never 100vh)", () => {
    const sheets = sourcesHere(".css");
    expect(sheets.map(([name]) => name)).toContain("popover.css");
    expect(largeViewportHeights(sheets)).toEqual([]);
  });

  it("finds a 100vh, so the check above can fail", () => {
    expect(
      largeViewportHeights([
        ["a.css", ".a { min-height: 100vh; } .b { height: 100dvh; } .c { height: 100svh; }"],
        ["b.css", ".d { max-height: calc(100vh - 1rem); }"],
      ]),
    ).toEqual(["a.css: min-height: 100vh", "b.css: max-height: calc(100vh - 1rem)"]);
  });
});

describe("a heading", () => {
  it("balances its lines, so a long title never leaves one word alone", () => {
    const headings = declarationsOf(sourceHere("base.css")).filter(
      (declaration) => declaration.selector === "h1, h2, h3",
    );
    expect(headings.map((declaration) => declaration.text)).toContain("text-wrap: balance");
  });
});

describe("a field left wrong", () => {
  const edges = declarationsOf(sourceHere("base.css"))
    .filter((declaration) => declaration.text === "border-color: var(--color-invalid)")
    .map((declaration) => declaration.selector);

  it("has its own red edge, once the person has left it (:user-invalid)", () => {
    expect(edges).toContain("input:user-invalid, select:user-invalid, textarea:user-invalid");
  });

  it("has it too where the server marked it, in a rule of its own a browser without :user-invalid keeps", () => {
    expect(edges).toContain('[aria-invalid="true"]:is(input, select, textarea)');
  });

  it("is a colour of its own, not the irreversible red's, so either can change alone", () => {
    const tokens = declarationsOf(sourceHere("tokens.css")).map((declaration) => declaration.text);
    expect(tokens.filter((text) => text.startsWith("--color-invalid:"))).toHaveLength(1);
  });
});
