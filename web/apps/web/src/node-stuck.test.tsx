// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { NodeStuckView, type Stuck } from "./node-stuck.tsx";
import { withServices } from "./test-services.tsx";

function markupOf(stuck: readonly Stuck[]): string {
  return renderToStaticMarkup(withServices(<NodeStuckView stuck={stuck} />));
}

describe("a machine's work waiting on a disk that does not answer (D233 §10)", () => {
  it("names each program and the folder it was reading, as an alert", () => {
    const shown = markupOf([
      { program: "ffmpeg", paths: ["/media/films/Wing.mkv"], since: 1_790_000_000_000 },
      { program: "node", paths: ["/media/shows"], since: 1_790_000_060_000 },
    ]);
    expect(shown).toContain("Waiting on a disk that does not answer");
    expect(shown).toContain("FFmpeg, reading /media/films/Wing.mkv, since");
    expect(shown).toContain("Listing or reading a folder, reading /media/shows, since");
    expect(shown).toContain('role="alert"');
  });

  it("says nothing when nothing waits", () => {
    expect(markupOf([])).toBe("");
  });
});
