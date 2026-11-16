// SPDX-License-Identifier: MPL-2.0

/**
 * An other video's own page with no poster (the owner, 2026-11-16; D496): its card's 16:9 picture
 * alone in the poster's place, never the "No poster" card a film with none shows.
 */

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { TitleView, type TitleDetail } from "./title.tsx";
import { withServices } from "./test-services.tsx";

const beachDay: TitleDetail = {
  id: 90,
  kind: "other",
  name: "Beach day",
  identificationState: "identified",
  art: { noPosterStillSha256: "5711" },
  versions: [],
  extras: null,
};

describe("an other video's page with no poster (D496)", () => {
  it("shows its card's picture alone, with no No poster card", () => {
    const markup = renderToStaticMarkup(withServices(<TitleView title={beachDay} />));
    expect(markup).toContain(
      '<img class="title__still" src="/api/v1/bytes/art/5711/landscape-m.webp"',
    );
    expect(markup).not.toContain("No poster");
    expect(markup).not.toContain("poster--blank");
  });

  it("shows no No poster card even with no picture at all", () => {
    const markup = renderToStaticMarkup(
      withServices(<TitleView title={{ ...beachDay, art: {} }} />),
    );
    expect(markup).not.toContain("No poster");
    expect(markup).not.toContain("poster--blank");
  });

  it("shows a poster of its own upright, as D495 has it, and a film with none the No poster card", () => {
    const withPoster = renderToStaticMarkup(
      withServices(<TitleView title={{ ...beachDay, art: { posterSha256: "p9" } }} />),
    );
    expect(withPoster).toContain('src="/api/v1/bytes/art/p9/portrait-m.webp"');
    const film = renderToStaticMarkup(
      withServices(<TitleView title={{ ...beachDay, kind: "film" }} />),
    );
    expect(film).toContain(
      '<img class="poster__still" src="/api/v1/bytes/art/5711/landscape-s.webp"',
    );
    expect(film).toContain("No poster");
  });
});
