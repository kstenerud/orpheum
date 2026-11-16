// SPDX-License-Identifier: MPL-2.0

/**
 * What a row of Continue Watching shows in its poster's place, whatever its kind (the owner,
 * 2026-11-16, "Better to keep them looking consistent with the other types"; D498 §1): its poster
 * (an episode's own, else its series'); else the full-size "No poster" card with its 16:9 picture in
 * the top half, an other video's too; else that card bare.
 */

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { ContinueWatchingView, type ContinueWatchingRow } from "./continue-watching.tsx";
import { withServices } from "./test-services.tsx";

const nothing = (): void => undefined;

const row = (
  id: number,
  kind: ContinueWatchingRow["kind"],
  art: Partial<ContinueWatchingRow>,
): ContinueWatchingRow => ({
  id,
  kind,
  name: `Row ${String(id)}`,
  fileId: id,
  positionMs: 500,
  durationMs: 1000,
  ...art,
});

function rendered(rows: readonly ContinueWatchingRow[]): string {
  return renderToStaticMarkup(
    withServices(
      <ContinueWatchingView rows={rows} hiddenName={undefined} onHide={nothing} onUndo={nothing} />,
    ),
  );
}

describe("Continue Watching's art (D498 §1)", () => {
  it("shows each kind's picture in the top half of the full-size No poster card where it has no poster", () => {
    const markup = rendered([
      row(1, "movie", { noPosterStillSha256: "f1" }),
      row(2, "episode", { noPosterStillSha256: "e2" }),
      row(3, "other", { noPosterStillSha256: "o3" }),
    ]);
    for (const sha of ["f1", "e2", "o3"]) {
      expect(markup).toContain(
        `<img class="poster__still" src="/api/v1/bytes/art/${sha}/landscape-s.webp"`,
      );
    }
    expect(markup.match(/class="poster poster--blank poster--with-still"/g)).toHaveLength(3);
    expect(markup.match(/No poster/g)).toHaveLength(3);
    // No 16:9 card: a row mixing kinds keeps one shape.
    expect(markup).not.toContain("poster--wide");
    expect(markup).not.toContain("poster-card__tile");
  });

  it("shows the full-size No poster card bare for a row with nothing to show, an other video's too", () => {
    const markup = rendered([row(4, "movie", {}), row(5, "other", {})]);
    expect(markup.match(/class="poster poster--blank"/g)).toHaveLength(2);
    expect(markup.match(/No poster/g)).toHaveLength(2);
    expect(markup).not.toContain("poster--wide");
  });

  it("shows a poster upright, its own or its series', in place of any picture", () => {
    const markup = rendered([
      row(6, "episode", { posterSha256: "p6" }),
      row(7, "other", { posterSha256: "p7" }),
    ]);
    expect(markup).toContain('src="/api/v1/bytes/art/p6/portrait-m.webp"');
    expect(markup).toContain('src="/api/v1/bytes/art/p7/portrait-m.webp"');
    expect(markup).not.toContain("No poster");
  });
});
