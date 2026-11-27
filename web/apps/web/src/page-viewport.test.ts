// SPDX-License-Identifier: MPL-2.0

import { readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

/** The page every part loads from (index.html). */
const page = readFileSync(path.join(import.meta.dirname, "../index.html"), "utf8");

describe("the page's viewport (D566)", () => {
  // A phone's keyboard covered a dialog's buttons, or pushed its fields off the top: Chrome on
  // Android leaves the page's height as it was under a keyboard unless the page asks otherwise,
  // and every dialog is sized by that height (the owner, 2026-11-26: the Invite dialog).
  it("has a phone's keyboard resize the page, so a dialog fits the room above it", () => {
    const viewport = /<meta\s+name="viewport"\s+content="([^"]*)"/.exec(page)?.[1] ?? "";
    expect(viewport.split(",").map((part) => part.trim())).toContain(
      "interactive-widget=resizes-content",
    );
  });
});
