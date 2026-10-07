// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { ApiProblem } from "./problem.ts";
import { mount, servedBy, type Heard } from "./test-dom.tsx";
import { Loaded, Title, TitleView, type TitleDetail } from "./title.tsx";
import { withServices } from "./test-services.tsx";

const heat: TitleDetail = {
  id: 10,
  kind: "movie",
  name: "Metropolis",
  year: 1995,
  runtimeMs: 170 * 60_000,
  tagline: "A Los Angeles crime saga",
  overview: "A crime saga.",
  identificationState: "identified",
  libraryId: 1,
  art: { posterSha256: "p1", backdropSha256: "b1" },
  versions: [
    {
      id: 1,
      files: [
        {
          id: 1,
          path: "/m/Metropolis (1995).mkv",
          streams: [
            { id: 1, kind: "video", codec: "h264" },
            { id: 2, kind: "audio", codec: "truehd", language: "en", channelCount: 8 },
            { id: 3, kind: "attachment", codec: "ttf" },
          ],
        },
      ],
    },
  ],
  extras: [
    { id: 30, name: "Metropolis trailer", extraType: "trailer" },
    { id: 31, name: "Making Metropolis", extraType: "behind_the_scenes" },
  ],
};

describe("TitleView", () => {
  it("shows a film's art, facts, streams and extras", () => {
    const markup = renderToStaticMarkup(withServices(<TitleView title={heat} />));
    expect(markup).toContain('src="/api/v1/bytes/art/b1/landscape-l.webp"');
    expect(markup).toContain('src="/api/v1/bytes/art/p1/portrait-m.webp"');
    expect(markup).toContain("<h1>Metropolis</h1>");
    expect(markup).toContain("170 min");
    expect(markup).toContain("Audio: truehd English 8 channels");
    expect(markup).not.toContain("ttf");
    expect(markup).toContain('href="/titles/30"');
    expect(markup).toContain("Trailer");
    expect(markup).toContain("Behind the scenes");
    expect(markup).toContain("/m/Metropolis (1995).mkv");
  });

  it("keeps a file's path and streams in a closed File details, with Play outside it", () => {
    const markup = renderToStaticMarkup(withServices(<TitleView title={heat} />));
    const details =
      /<details class="title__details"><summary>File details<\/summary>(.*?)<\/details>/u.exec(
        markup,
      )?.[1];
    expect(details).toContain("/m/Metropolis (1995).mkv");
    expect(details).toContain("Audio: truehd English 8 channels");
    expect(details).not.toContain("Play");
    expect(markup).toMatch(/class="title__play"[^>]*>Play<\/a><details class="title__details">/u);
  });

  it("says why a title cannot be played now, and offers no play (D241 §3)", () => {
    const playable = renderToStaticMarkup(withServices(<TitleView title={heat} />));
    const missing = renderToStaticMarkup(
      withServices(
        <TitleView
          title={{
            ...heat,
            unavailable: { reason: "root_missing", nodeName: "devnas", path: "/media/films" },
          }}
        />,
      ),
    );
    expect(playable).toContain('href="/play/1"');
    expect(missing).toContain(
      "This cannot be played now: the folder /media/films on devnas is missing.",
    );
    expect(missing).not.toContain('href="/play/1"');
    expect(missing).toContain("<h1>Metropolis</h1>");
  });

  it("offers an admin to retire a file that cannot be read now, and no one else (D241 §5)", () => {
    const missing = { ...heat, unavailable: { reason: "node_away" as const, nodeName: "devnas" } };
    const retire = (): void => undefined;
    const forAdmin = renderToStaticMarkup(
      withServices(<TitleView title={missing} onRetireFile={retire} />),
    );
    const readable = renderToStaticMarkup(
      withServices(<TitleView title={heat} onRetireFile={retire} />),
    );
    const forUser = renderToStaticMarkup(
      withServices(
        <TitleView
          title={{ ...missing, versions: [{ id: 1, files: [{ id: 1, streams: [] }] }] }}
          onRetireFile={retire}
        />,
      ),
    );
    expect(forAdmin).toContain("This file is gone for good…");
    expect(readable).not.toContain("This file is gone for good…");
    expect(forUser).not.toContain("This file is gone for good…");
  });

  it("places an episode in its series, with its still", () => {
    const episode: TitleDetail = {
      ...heat,
      kind: "episode",
      name: "Pilot",
      art: { stillSha256: "s1" },
      episode: { seriesId: 20, seriesName: "Brass Arena", seasonNumber: 1, episodeNumber: 1 },
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
          <Loaded
            resource={{
              state: "failed",
              cause: new ApiProblem({ code: "not_found", message: "no such title" }),
            }}
            render={render}
          />,
        ),
      ),
    ).toContain("That is no longer here");
  });
});

describe("Title", () => {
  it("warms up its first file as it opens, and nothing of a title that cannot play now (D258 §1)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items/10": [200, heat],
        "GET /items/11": [
          200,
          { ...heat, id: 11, unavailable: { reason: "node_away" as const, nodeName: "devnas" } },
        ],
        "POST /playback": [
          200,
          { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a" },
        ],
      },
      heard,
    );
    const playable = await mount(withServices(<Title id={10} />, services));
    playable.unmount();
    expect(heard.map((request) => request.methodAndPath)).toEqual([
      "GET /items/10",
      "POST /playback",
    ]);
    expect(heard[1]?.body).toMatchObject({ fileId: 1 });
    heard.length = 0;
    const unplayable = await mount(withServices(<Title id={11} />, services));
    unplayable.unmount();
    expect(heard.map((request) => request.methodAndPath)).toEqual(["GET /items/11"]);
  });
});
