// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Loaded, TitleView, type TitleDetail } from "./title.tsx";
import { withServices } from "./test-services.tsx";

const heat: TitleDetail = {
  id: 10,
  kind: "movie",
  name: "Metropolis",
  year: 1995,
  runtimeMs: 170 * 60_000,
  tagline: "A Los Angeles crime saga",
  overview: "A crime saga.",
  ident: "identified",
  libraryId: 1,
  art: { poster: "p1", backdrop: "b1" },
  versions: [
    {
      id: 1,
      files: [
        {
          id: 1,
          path: "/m/Metropolis (1995).mkv",
          streams: [
            { id: 1, kind: "video", codec: "h264" },
            { id: 2, kind: "audio", codec: "truehd", lang: "en", channels: 8 },
            { id: 3, kind: "attachment", codec: "ttf" },
          ],
        },
      ],
    },
  ],
  extras: [
    { id: 30, name: "Metropolis trailer", type: "trailer" },
    { id: 31, name: "Making Metropolis", type: "behind_the_scenes" },
  ],
};

describe("TitleView", () => {
  it("shows a film's art, facts, streams and extras", () => {
    const markup = renderToStaticMarkup(withServices(<TitleView title={heat} />));
    expect(markup).toContain('src="/api/v1/bytes/art/b1/landscape-l.webp"');
    expect(markup).toContain('src="/api/v1/bytes/art/p1/portrait-m.webp"');
    expect(markup).toContain("<h1>Metropolis</h1>");
    expect(markup).toContain("170 min");
    expect(markup).toContain("Audio: truehd en 8 channels");
    expect(markup).not.toContain("ttf");
    expect(markup).toContain('href="/titles/30"');
    expect(markup).toContain("Trailer");
    expect(markup).toContain("Behind the scenes");
    expect(markup).toContain("/m/Metropolis (1995).mkv");
  });

  it("places an episode in its series, with its still", () => {
    const episode: TitleDetail = {
      ...heat,
      kind: "episode",
      name: "Pilot",
      art: { still: "s1" },
      episode: { seriesId: 20, seriesName: "Brass Arena", season: 1, number: 1 },
      versions: [],
      extras: null,
    };
    const markup = renderToStaticMarkup(withServices(<TitleView title={episode} />));
    expect(markup).toContain('href="/series/20"');
    expect(markup).toContain("Brass Arena · season 1, episode 1");
    expect(markup).toContain('src="/api/v1/bytes/art/s1/landscape-m.webp"');
  });

  it("says what is loading, and why a load failed", () => {
    const render = (value: number): React.JSX.Element => <p>{value}</p>;
    expect(
      renderToStaticMarkup(
        withServices(<Loaded resource={{ state: "loading" }} render={render} />),
      ),
    ).toContain("Loading");
    expect(
      renderToStaticMarkup(
        withServices(
          <Loaded resource={{ state: "failed", message: "no such title" }} render={render} />,
        ),
      ),
    ).toContain("no such title");
  });
});
