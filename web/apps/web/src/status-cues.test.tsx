// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { statusCues, StatusNotes, StatusTray } from "./status-cues.tsx";
import { withServices } from "./test-services.tsx";

describe("the status cues", () => {
  it("are one list, the warning of one that cannot play always first (the owner, 2026-10-15)", () => {
    expect(statusCues.map((cue) => cue.name)).toEqual([
      "cannotPlay",
      "uncertainMatch",
      "stillReading",
    ]);
  });

  it("show in the list's order over a poster and on a page, and nothing for a subject with none", () => {
    const both = {
      unavailable: { reason: "node_away" as const, nodeName: "devnas" },
      isGuessed: true,
    };
    for (const markup of [
      renderToStaticMarkup(withServices(<StatusTray subject={both} />)),
      renderToStaticMarkup(withServices(<StatusNotes subject={both} />)),
    ]) {
      const warning = markup.indexOf("This cannot be played now");
      expect(warning).toBeGreaterThanOrEqual(0);
      expect(markup.indexOf("This may be identified incorrectly")).toBeGreaterThan(warning);
    }
    expect(renderToStaticMarkup(withServices(<StatusTray subject={{}} />))).toBe("");
  });

  it("mark a title still being read with its own cue, not the cannot-play warning (D304 §4)", () => {
    const reading = { unavailable: { reason: "still_reading" as const, nodeName: "devnas" } };
    for (const markup of [
      renderToStaticMarkup(withServices(<StatusTray subject={reading} />)),
      renderToStaticMarkup(withServices(<StatusNotes subject={reading} />)),
    ]) {
      expect(markup).toContain("Still reading");
      expect(markup).not.toContain("This cannot be played now");
    }
  });
});
