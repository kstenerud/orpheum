// SPDX-License-Identifier: MPL-2.0

/**
 * What a row of Continue Watching shows in its poster's place, whatever its kind (the owner,
 * 2026-11-16; D497): its poster, upright; else its 16:9 picture alone, with no "No poster" card;
 * else the "No poster" card in that 16:9 shape. A 16:9 card stands centred in a poster's height
 * where the row holds a poster, and a row of 16:9 cards alone is as tall as they are (D495 §4).
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

describe("Continue Watching's art (D497)", () => {
  it("shows each kind's picture alone where it has no poster, with no No poster card", () => {
    const markup = rendered([
      row(1, "movie", { noPosterStillSha256: "f1" }),
      row(2, "episode", { noPosterStillSha256: "e2" }),
      row(3, "other", { noPosterStillSha256: "o3" }),
    ]);
    for (const sha of ["f1", "e2", "o3"]) {
      expect(markup).toContain(
        `<img class="poster-card__tile" src="/api/v1/bytes/art/${sha}/landscape-s.webp"`,
      );
    }
    expect(markup).not.toContain("No poster");
    expect(markup).not.toContain("poster--blank");
    // No poster in the row: the pictures are as tall as they are.
    expect(markup).not.toContain("poster-card__art--poster-height");
  });

  it("shows the No poster card in a 16:9 shape for a row with nothing to show", () => {
    const markup = rendered([row(4, "movie", {}), row(5, "other", {})]);
    expect(markup.match(/<span class="poster poster--blank poster--wide/g)).toHaveLength(2);
    expect(markup.match(/No poster/g)).toHaveLength(2);
  });

  it("shows a poster upright, its own or its series', and centres the pictures in its height", () => {
    const markup = rendered([
      row(6, "episode", { posterSha256: "p6" }),
      row(7, "other", { noPosterStillSha256: "o7" }),
      row(8, "movie", {}),
    ]);
    expect(markup).toContain('src="/api/v1/bytes/art/p6/portrait-m.webp"');
    expect(markup.match(/poster-card__art poster-card__art--poster-height/g)).toHaveLength(2);
    expect(markup).not.toContain("poster--with-still");
  });
});
