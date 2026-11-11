// SPDX-License-Identifier: MPL-2.0

import { readdirSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

/** This directory, web/apps/web/src/, whose stylesheets are read. */
const here = new URL("./", import.meta.url);

/** The condition under which a stylesheet may take the focus ring away: no more contrast wanted. */
const noMoreContrast = "@media (forced-colors: none) and (prefers-contrast: no-preference)";

/** A declaration in a stylesheet, with the at-rules it stands in, outermost first. */
interface Declaration {
  readonly text: string;
  readonly within: readonly string[];
}

/** Every declaration of a stylesheet with the at-rules around it; comments are let go. */
function declarationsOf(sheet: string): Declaration[] {
  const text = sheet.replaceAll(/\/\*[\s\S]*?\*\//gu, "");
  const found: Declaration[] = [];
  const open: string[] = [];
  let pending = "";
  for (const character of text) {
    if (character === "{") {
      open.push(pending.trim());
      pending = "";
    } else if (character === "}") {
      if (pending.trim() !== "") {
        found.push({ text: pending.trim(), within: open.filter((at) => at.startsWith("@")) });
      }
      open.pop();
      pending = "";
    } else if (character === ";") {
      found.push({ text: pending.trim(), within: open.filter((at) => at.startsWith("@")) });
      pending = "";
    } else {
      pending += character;
    }
  }
  return found;
}

/** Each declaration taking the outline away, as `file: declaration`, outside noMoreContrast. */
function ringsTakenAway(sheets: readonly (readonly [string, string])[]): string[] {
  return sheets.flatMap(([name, sheet]) =>
    declarationsOf(sheet)
      .filter((declaration) => /^outline(-style)?\s*:\s*(none|0)\b/u.test(declaration.text))
      .filter((declaration) => !declaration.within.includes(noMoreContrast))
      .map((declaration) => `${name}: ${declaration.text}`),
  );
}

function stylesheets(): (readonly [string, string])[] {
  return readdirSync(here)
    .filter((name) => name.endsWith(".css"))
    .map((name) => [name, readFileSync(new URL(name, here), "utf8")] as const);
}

describe("the focus ring where the browser asks for more contrast", () => {
  it("is taken away only where no more contrast is wanted", () => {
    const sheets = stylesheets();
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
    const tokens = declarationsOf(readFileSync(new URL("tokens.css", here), "utf8"));
    expect(
      tokens
        .filter((declaration) => declaration.within.includes("@media (forced-colors: active)"))
        .map((declaration) => declaration.text),
    ).toContain("--focus-ring-inner-color: Highlight");
  });
});
