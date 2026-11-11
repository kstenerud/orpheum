// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { declarationsOf, sourceHere, sourcesHere } from "./test-sources.ts";

/** The condition under which a stylesheet may take the focus ring away: no more contrast wanted. */
const noMoreContrast = "@media (forced-colors: none) and (prefers-contrast: no-preference)";

/** Each declaration taking the outline away, as `file: declaration`, outside noMoreContrast. */
function ringsTakenAway(sheets: readonly (readonly [string, string])[]): string[] {
  return sheets.flatMap(([name, sheet]) =>
    declarationsOf(sheet)
      .filter((declaration) => /^outline(-style)?\s*:\s*(none|0)\b/u.test(declaration.text))
      .filter((declaration) => !declaration.within.includes(noMoreContrast))
      .map((declaration) => `${name}: ${declaration.text}`),
  );
}

describe("the focus ring where the browser asks for more contrast", () => {
  it("is taken away only where no more contrast is wanted", () => {
    const sheets = sourcesHere(".css");
    const removals = sheets.flatMap(([name, sheet]) =>
      declarationsOf(sheet)
        .filter((declaration) => declaration.text === "outline: none")
        .map(() => name),
    );
    expect(removals.toSorted()).toEqual([
      "modal-dialog.css",
      "player-overlay.css",
      "volume-row.css",
    ]);
    expect(ringsTakenAway(sheets)).toEqual([]);
  });

  it("finds a ring taken away for every browser, so the check above can fail", () => {
    expect(
      ringsTakenAway([
        ["a.css", ".a:focus-visible { outline: none; }"],
        ["b.css", `${noMoreContrast} { .b:focus-visible { outline: none; } }`],
        ["c.css", "@media (width > 1px) { .c { outline-style: none } }"],
      ]),
    ).toEqual(["a.css: outline: none", "c.css: outline-style: none"]);
  });

  it("is drawn in the system's own colour for what is chosen, under forced colours", () => {
    const tokens = declarationsOf(sourceHere("tokens.css"));
    expect(
      tokens
        .filter((declaration) => declaration.within.includes("@media (forced-colors: active)"))
        .map((declaration) => declaration.text),
    ).toContain("--focus-ring-inner-color: Highlight");
  });
});
