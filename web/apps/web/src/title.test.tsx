// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { BackgroundChangesContext, backingOffPoller } from "./polling.ts";
import { ApiProblem } from "./problem.ts";
import {
  buttonNamed,
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  unanswered,
  type Heard,
} from "./test-dom.tsx";
import { IsAdminContext } from "./warning-sign.tsx";
import { Loaded, Title, TitleView, type TitleDetail } from "./title.tsx";
import { createPathMoves } from "./services.tsx";
import { withServices } from "./test-services.tsx";

const heat: TitleDetail = {
  id: 10,
  kind: "film",
  name: "Metropolis",
  year: 1927,
  runtimeMs: 170 * 60_000,
  tagline: "A Los Angeles crime saga",
  overview: "A crime saga.",
  identificationState: "identified",
  art: { posterSha256: "p1", backdropSha256: "b1" },
  versions: [
    {
      id: 1,
      files: [
        {
          id: 1,
          path: "/m/Metropolis (1927).mkv",
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
    expect(markup).toContain("Sound: truehd English 8 channels");
    expect(markup).not.toContain("ttf");
    expect(markup).toContain('href="/titles/30"');
    expect(markup).toContain("Trailer");
    expect(markup).toContain("Behind the scenes");
    expect(markup).toContain("/m/Metropolis (1927).mkv");
  });

  it("names its sound's channels as the player does: mono, not 1 channels (the demo library, 2026-11-08)", () => {
    // The title's facts said "AAC · 1 channels" for a mono file: it counted channels itself,
    // rather than through the player's sound line, which names mono, stereo, 5.1 and 7.1.
    const withSound = (channelCount: number): TitleDetail => ({
      ...heat,
      versions: [
        {
          id: 1,
          files: [
            {
              id: 1,
              path: "/m/a.mkv",
              streams: [{ id: 2, kind: "audio", codec: "aac", channelCount }],
            },
          ],
        },
      ],
    });
    expect(renderToStaticMarkup(withServices(<TitleView title={withSound(1)} />))).toContain(
      "<span>AAC · mono</span>",
    );
    expect(renderToStaticMarkup(withServices(<TitleView title={withSound(8)} />))).toContain(
      "<span>AAC · 7.1</span>",
    );
  });

  it("shows no broken banner or still art not made yet, through the posters' own way (the owner, 2026-10-26)", async () => {
    // The title page drew its backdrop and an episode's still as plain images, so art not made
    // yet, or never made, showed as a broken image across the top of the page (staging answered
    // 404 for 12 Patient Sailors's landscape-l).
    const page = await mount(withServices(<TitleView title={heat} />));
    const backdrop = (): Element | null => page.container.querySelector("img.title-head__backdrop");
    expect(backdrop()?.getAttribute("src")).toBe("/api/v1/bytes/art/b1/landscape-l.webp");
    act(() => {
      backdrop()?.dispatchEvent(new Event("error"));
    });
    expect(backdrop()).toBeNull();
    // The rest of the page still draws: it is the backdrop alone that goes, not the whole page.
    expect(page.container.textContent).toContain("Metropolis");
    page.unmount();

    const episode: TitleDetail = {
      ...heat,
      kind: "episode",
      name: "Pilot",
      art: { stillSha256: "s1" },
      episode: { seriesId: 20, seriesName: "Brass Arena", seasonNumber: 1, episodeNumber: 1 },
      versions: [],
      extras: null,
    };
    const episodePage = await mount(withServices(<TitleView title={episode} />));
    const still = (): Element | null => episodePage.container.querySelector("img.title__still");
    expect(still()?.getAttribute("src")).toBe("/api/v1/bytes/art/s1/landscape-m.webp");
    act(() => {
      still()?.dispatchEvent(new Event("error"));
    });
    expect(still()).toBeNull();
    expect(episodePage.container.textContent).toContain("Pilot");
    episodePage.unmount();
  });

  it("offers no cast of its own: a cast is a session started from the status tray (D364 §1, §12)", () => {
    const markup = renderToStaticMarkup(withServices(<TitleView title={heat} />));
    expect(markup).not.toContain("cast-to");
    expect(markup).not.toContain("Cast to another device");
  });

  it("keeps a file's path and streams in a closed File details, with Play outside it, first in the actions row", () => {
    const markup = renderToStaticMarkup(withServices(<TitleView title={heat} />));
    const details =
      /<details class="title__details"><summary>File details<\/summary>(.*?)<\/details>/u.exec(
        markup,
      )?.[1];
    expect(details).toContain("/m/Metropolis (1927).mkv");
    expect(details).toContain("Sound: truehd English 8 channels");
    expect(details).not.toContain("Play");
    // Play is the actions row's first, above the synopsis, not a file's below the cast (R8).
    expect(markup).toMatch(
      /<div class="page-actions title-head__actions"[^>]*><a [^>]*class="title__play"[^>]*>Play<\/a>/u,
    );
    expect(markup.indexOf('class="title__play"')).toBeLessThan(
      markup.indexOf('<details class="title__details">'),
    );
    expect(markup.match(/class="title__play"/gu)).toHaveLength(1);
  });

  // The owner, 2026-10-29: "Play is the page's first button, not its last", and the rarely used
  // ones under a vertical "…".
  it("has Play as its first button, then Mark as watched and an admin's …, no correction loose on the page", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <IsAdminContext value={true}>
          <TitleView
            title={heat}
            onMatchCorrected={() => undefined}
            onWatchedChanged={() => undefined}
          />
        </IsAdminContext>,
      ),
    );
    const controls = [
      ...markup.matchAll(/<(?:a [^>]*class="title__play"|button)[^>]*>([^<]*)/gu),
    ].map((control) =>
      control[0].includes('aria-label="More actions"') ? "More actions" : control[1],
    );
    expect(controls).toEqual(["Play", "Mark as watched", "More actions"]);
    expect(markup).not.toContain("Identified incorrectly");
    expect(markup).not.toContain("Report this");
  });

  it("says why a title cannot be played now, and offers no play (D241 §3)", () => {
    const playable = renderToStaticMarkup(withServices(<TitleView title={heat} />));
    const missing = renderToStaticMarkup(
      withServices(
        <TitleView
          title={{
            ...heat,
            unavailable: { reason: "rootMissing", nodeName: "devnas", path: "/media/films" },
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
    const missing = { ...heat, unavailable: { reason: "nodeAway" as const, nodeName: "devnas" } };
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

  it("places an episode in its series and its season, each a link, with its still", () => {
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
    // The series and its season each a link up (research/back-navigation-2026-10-24.md).
    expect(markup).toContain('<a href="/series/20">Brass Arena</a> · ');
    expect(markup).toContain('<a href="/series/20?season=1">season 1</a>, episode 1');
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
              cause: new ApiProblem({ code: "notFound", message: "no such title" }),
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
          { ...heat, id: 11, unavailable: { reason: "nodeAway" as const, nodeName: "devnas" } },
        ],
        "POST /playback": [
          200,
          { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a" },
        ],
        "PUT /interest/page": [204, null],
        "DELETE /interest/page": [204, null],
      },
      heard,
    );
    // The page's word that it is shown is the test below's.
    const warming = (): string[] =>
      heard
        .filter((request) => !request.methodAndPath.includes("/interest/"))
        .map((request) => request.methodAndPath);
    const playable = await mount(withServices(<Title id={10} />, services));
    playable.unmount();
    expect(warming()).toEqual(["GET /items/10", "POST /playback"]);
    expect(heard.find((request) => request.methodAndPath === "POST /playback")?.body).toMatchObject(
      {
        fileId: 1,
      },
    );
    heard.length = 0;
    const unplayable = await mount(withServices(<Title id={11} />, services));
    unplayable.unmount();
    expect(warming()).toEqual(["GET /items/11"]);
  });

  it("preloads its film in a hidden video on HTTP, and not where its media go over WebRTC (the efficiency audit, finding 1)", async () => {
    const onLadder = {
      url: "/api/v1/bytes/media/chunks/a/master.m3u8",
      startAtBps: 4_000_000,
      qualityKbps: 2400,
      height: 1080,
      isBelowTheLowest: false,
      soundCarriedGainDb: 0,
      isReady: false,
      filmPictureKbps: 2400,
    };
    // The hidden videos a film page started, the warm-up's decision answered on the film's ladder.
    const hiddenVideos = async (isOverWebrtc: boolean): Promise<number> => {
      const heard: Heard[] = [];
      const services = servedBy(
        {
          "GET /items/10": [200, heat],
          "POST /playback": [
            200,
            { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a", onLadder },
          ],
          "PUT /interest/page": [204, null],
          "DELETE /interest/page": [204, null],
        },
        heard,
      );
      const pathMoves = { ...createPathMoves(), isOverWebrtc: () => isOverWebrtc };
      const page = await mount(withServices(<Title id={10} />, { ...services, pathMoves }));
      await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /playback"));
      // The decision's answer is read, and the preload started from it, a moment after.
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 50));
      });
      page.unmount();
      const found = document.body.querySelectorAll("video[hidden]");
      for (const video of found) {
        video.remove();
      }
      return found.length;
    };
    expect(await hiddenVideos(false)).toBe(1);
    expect(await hiddenVideos(true)).toBe(0);
  });

  it("says its film's work should come first while it is shown, and that it left (D324 §1)", async () => {
    const heard: Heard[] = [];
    const page = await mount(
      withServices(
        <Title id={11} />,
        servedBy(
          {
            "GET /items/11": [
              200,
              {
                ...heat,
                id: 11,
                unavailable: { reason: "nodeAway" as const, nodeName: "devnas" },
              },
            ],
            "PUT /interest/page": [204, null],
            "DELETE /interest/page": [204, null],
          },
          heard,
        ),
      ),
    );
    expect(
      heard.map((request) => `${request.methodAndPath} ${JSON.stringify(request.body)}`),
    ).toContain('PUT /interest/page {"item":11}');
    page.unmount();
    await settleUntil(() =>
      heard.some(
        (request) =>
          `${request.methodAndPath}${request.query ?? ""}` === "DELETE /interest/page?item=11",
      ),
    );
    const last = heard.at(-1);
    expect(`${last?.methodAndPath ?? ""}${last?.query ?? ""}`).toBe(
      "DELETE /interest/page?item=11",
    );
  });

  it("offers Play by itself once its file is read, or can be read again (the owner, 2026-10-24)", async () => {
    // Opened while its file is still being read, or while its machine is away, the page asks again
    // on the background poller (D237) and offers Play once the server says the file plays.
    const soon = backingOffPoller({ firstMs: 50, longestMs: 50, failedMs: 50 });
    const cases = [
      { id: 13, reason: "stillReading" as const },
      { id: 14, reason: "nodeAway" as const },
    ];
    for (const { id, reason } of cases) {
      const title = { ...heat, id };
      const heard: Heard[] = [];
      const services = servedBy(
        {
          [`GET /items/${String(id)}`]: [
            [200, { ...title, unavailable: { reason, nodeName: "devnas" } }],
            [200, title],
          ],
          "POST /playback": [
            200,
            { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a" },
          ],
          "PUT /interest/page": [204, null],
          "DELETE /interest/page": [204, null],
        },
        heard,
      );
      const page = await mount(
        <BackgroundChangesContext value={soon}>
          {withServices(<Title id={id} />, services)}
        </BackgroundChangesContext>,
      );
      const play = (): Element | null => page.container.querySelector("a.title__play");
      await settleUntil(() => play() !== null);
      expect(play()?.getAttribute("href")).toBe("/play/1");
      // Play came from asking again: the first answer said the file could not play.
      const reads = heard.filter((each) => each.methodAndPath === `GET /items/${String(id)}`);
      expect(reads.length).toBeGreaterThan(1);
      page.unmount();
    }
  });

  it("offers Play by itself even after one of its asks never answered (the owner, 2026-10-24)", async () => {
    // On a TV over the discovery's transport one request never answered, and the page, which asks
    // again only once the last ask answered, never offered Play until a reload. The poll gives the
    // ask up, aborting it, and asks again.
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const soon = backingOffPoller({ firstMs: 50, longestMs: 50, failedMs: 50 }, 100);
    const title = { ...heat, id: 15 };
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items/15": [
          [200, { ...title, unavailable: { reason: "stillReading" as const, nodeName: "devnas" } }],
          unanswered,
          [200, title],
        ],
        "POST /playback": [
          200,
          { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a" },
        ],
        "PUT /interest/page": [204, null],
        "DELETE /interest/page": [204, null],
      },
      heard,
    );
    const page = await mount(
      <BackgroundChangesContext value={soon}>
        {withServices(<Title id={15} />, services)}
      </BackgroundChangesContext>,
    );
    const play = (): Element | null => page.container.querySelector("a.title__play");
    await settleUntil(() => play() !== null);
    expect(play()?.getAttribute("href")).toBe("/play/1");
    const reads = heard.filter((each) => each.methodAndPath === "GET /items/15");
    // The first read, the one never answered, and at least one after it, which offered Play.
    expect(reads.length).toBeGreaterThanOrEqual(3);
    // The ask given up was aborted, so its request does not stay open.
    expect(reads[1]?.signal?.aborted).toBe(true);
    page.unmount();
    vi.restoreAllMocks();
  });
});

describe("a film whose match is a guess (D301)", () => {
  it("is marked over its poster, and an admin alone corrects it from the works its search found", async () => {
    const guessed = { ...heat, id: 12, isGuessed: true };
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items/12": [
          [200, guessed],
          [200, { ...guessed, isGuessed: false }],
        ],
        "GET /items/12/candidates": [
          200,
          {
            provider: "tmdb",
            candidates: [
              { providerId: "949", name: "Metropolis", year: 1927 },
              { providerId: "50", name: "Metropolis", year: 1919 },
            ],
          },
        ],
        "POST /items/12/match": [204, {}],
        "POST /playback": [
          200,
          { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a" },
        ],
        "PUT /interest/page": [204, null],
        "DELETE /interest/page": [204, null],
      },
      heard,
    );
    const forUser = await mount(withServices(<Title id={12} />, services));
    expect(forUser.container.textContent).toContain("may be another work of the same name");
    // A correction is an admin's alone, and the "…" holds no File details (D418 §5): a person's
    // page has no "…" for a film of one version.
    expect(buttonNamed(forUser.container, "More actions")).toBeUndefined();
    expect(forUser.container.textContent).not.toContain("Identified incorrectly");
    forUser.unmount();
    const forAdmin = await mount(
      withServices(
        <IsAdminContext value={true}>
          <Title id={12} />
        </IsAdminContext>,
        services,
      ),
    );
    const page = forAdmin.container;
    await settleUntil(() => buttonNamed(page, "More actions") !== undefined);
    await click(buttonNamed(page, "More actions"));
    await click(buttonWithText(page, "Identified incorrectly"));
    // One dialog, in the list's place (the owner, 2026-10-29): Rescan and its full rescan beside it
    // (D392 §1), Enter manually, Report this.
    expect(page.querySelectorAll("dialog")).toHaveLength(1);
    const offered = [...(page.querySelector("dialog")?.querySelectorAll("button") ?? [])].map(
      (button) => button.textContent,
    );
    // Its Close is the cross every dialog has (the owner, 2026-10-30).
    expect(offered).toEqual(["Rescan", "Full rescan", "Enter manually", "Report this", ""]);
    expect(
      page.querySelector("dialog .modal-dialog__close button")?.getAttribute("aria-label"),
    ).toBe("Close");
    await click(buttonWithText(page, "Enter manually"));
    await settleUntil(() => page.textContent.includes("1919"));
    const picks = [...page.querySelectorAll("li button")];
    await click(picks[1] ?? null);
    await settleUntil(() =>
      heard.some((request) => request.methodAndPath === "POST /items/12/match"),
    );
    expect(heard.find((request) => request.methodAndPath === "POST /items/12/match")?.body).toEqual(
      {
        provider: "tmdb",
        providerId: "50",
      },
    );
    await settleUntil(() => !page.textContent.includes("This may be identified incorrectly"));
    forAdmin.unmount();
  });

  it("is confirmed by an admin as it stands, and its mark and the entry go (D301 §4)", async () => {
    const guessed = { ...heat, id: 13, isGuessed: true };
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items/13": [
          [200, guessed],
          [200, { ...guessed, isGuessed: false, identificationState: "confirmed" }],
        ],
        "POST /items/13/confirm-match": [204, null],
        "POST /playback": [
          200,
          { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a" },
        ],
        "PUT /interest/page": [204, null],
        "DELETE /interest/page": [204, null],
      },
      heard,
    );
    const mounted = await mount(
      withServices(
        <IsAdminContext value={true}>
          <Title id={13} />
        </IsAdminContext>,
        services,
      ),
    );
    const page = mounted.container;
    await settleUntil(() => buttonNamed(page, "More actions") !== undefined);
    await click(buttonNamed(page, "More actions"));
    await click(buttonWithText(page, "Identified correctly"));
    await click(buttonWithText(page, "Confirm"));
    await settleUntil(() => !page.textContent.includes("This may be identified incorrectly"));
    expect(heard.map((request) => request.methodAndPath)).toContain("POST /items/13/confirm-match");
    expect(page.querySelector("dialog")).toBeNull();
    await click(buttonNamed(page, "More actions"));
    expect(page.textContent).toContain("Identified incorrectly");
    expect(page.textContent).not.toContain("Identified correctly");
    mounted.unmount();
  });

  it("offers no Identified correctly where the match is no guess", async () => {
    const services = servedBy({
      "GET /items/14": [200, { ...heat, id: 14 }],
      "POST /playback": [
        200,
        { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a" },
      ],
      "PUT /interest/page": [204, null],
      "DELETE /interest/page": [204, null],
    });
    const mounted = await mount(
      withServices(
        <IsAdminContext value={true}>
          <Title id={14} />
        </IsAdminContext>,
        services,
      ),
    );
    const page = mounted.container;
    await settleUntil(() => buttonNamed(page, "More actions") !== undefined);
    await click(buttonNamed(page, "More actions"));
    expect(page.textContent).toContain("Identified incorrectly");
    expect(page.textContent).not.toContain("Identified correctly");
    mounted.unmount();
  });
});

describe("the title template (ui-audit-2026-11-04.md §3, R8)", () => {
  const played = (resumePositionMs: number): [number, unknown] => [
    200,
    { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a", resumePositionMs },
  ];

  it("names its Play Resume with the point its warm-up was told, and offers Start over beside it", async () => {
    const services = servedBy({
      "GET /items/10": [200, heat],
      "POST /playback": played(754_000),
      "PUT /interest/page": [204, null],
      "DELETE /interest/page": [204, null],
    });
    const mounted = await mount(withServices(<Title id={10} />, services));
    const page = mounted.container;
    const play = (): Element | null => page.querySelector("a.title__play");
    await settleUntil(() => play()?.textContent === "Resume from 12:34");
    // Resume plays where the server says it left off, as Play always has; Start over, from 0.
    expect(play()?.getAttribute("href")).toBe("/play/1");
    const startOver = [...page.querySelectorAll(".title-head__actions a")].find(
      (link) => link.textContent === "Start over",
    );
    expect(startOver?.getAttribute("href")).toBe("/play/1?at=0");
    mounted.unmount();
  });

  it("names its Play Play, and offers no Start over, where nothing was watched", async () => {
    const services = servedBy({
      "GET /items/10": [200, heat],
      "POST /playback": played(0),
      "PUT /interest/page": [204, null],
      "DELETE /interest/page": [204, null],
    });
    const mounted = await mount(withServices(<Title id={10} />, services));
    const page = mounted.container;
    await settleUntil(() => page.querySelector("a.title__play") !== null);
    expect(page.querySelector("a.title__play")?.textContent).toBe("Play");
    expect(page.textContent).not.toContain("Start over");
    mounted.unmount();
  });

  it("offers an episode's Next episode in its actions row, from what the player's Up next asks", async () => {
    const pilot: TitleDetail = {
      ...heat,
      id: 40,
      kind: "episode",
      name: "Pilot",
      art: {},
      episode: { seriesId: 20, seriesName: "Brass Arena", seasonNumber: 1, episodeNumber: 1 },
      extras: null,
    };
    const services = servedBy({
      "GET /items/40": [200, pilot],
      "POST /playback": played(0),
      "GET /files/1/up-next": [
        200,
        {
          upNext: {
            ordering: "aired",
            episode: { fileId: 7, itemId: 41, groupNumber: 1, numberInGroup: 2, isPartway: false },
          },
        },
      ],
      "PUT /interest/page": [204, null],
      "DELETE /interest/page": [204, null],
    });
    const mounted = await mount(withServices(<Title id={40} />, services));
    const page = mounted.container;
    const next = (): Element | undefined =>
      [...page.querySelectorAll(".title-head__actions a")].find(
        (link) => link.textContent === "Next episode",
      );
    await settleUntil(() => next() !== undefined);
    expect(next()?.getAttribute("href")).toBe("/play/7");
    // Its series and season sit above its name, the breadcrumb line.
    const words = page.querySelector(".title-head__words");
    expect(words?.firstElementChild?.textContent).toContain("Brass Arena");
    expect(words?.firstElementChild?.nextElementSibling?.tagName).toBe("H1");
    mounted.unmount();
  });

  it("asks a film for no next episode", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /items/10": [200, heat],
        "POST /playback": played(0),
        "PUT /interest/page": [204, null],
        "DELETE /interest/page": [204, null],
      },
      heard,
    );
    const mounted = await mount(withServices(<Title id={10} />, services));
    await settleUntil(() => mounted.container.querySelector("a.title__play") !== null);
    expect(heard.map((request) => request.methodAndPath)).not.toContain("GET /files/1/up-next");
    expect(mounted.container.textContent).not.toContain("Next episode");
    mounted.unmount();
  });

  it("says its facts on one line: year, length and the sound its Play plays", () => {
    const markup = renderToStaticMarkup(withServices(<TitleView title={heat} />));
    expect(markup).toContain(
      '<p class="title-head__facts"><span>1927</span><span>170 min</span><span>TRUEHD · 7.1</span></p>',
    );
  });

  it("plays its first version, and keeps the choice of the others under its …", async () => {
    const twoVersions: TitleDetail = {
      ...heat,
      versions: [
        ...(heat.versions ?? []),
        { id: 2, files: [{ id: 5, path: "/m/Metropolis (1927) 4K.mkv", streams: [] }] },
      ],
    };
    const mounted = await mount(withServices(<TitleView title={twoVersions} />));
    const page = mounted.container;
    expect(page.querySelectorAll("a.title__play")).toHaveLength(1);
    expect(page.querySelector("a.title__play")?.getAttribute("href")).toBe("/play/1");
    await click(buttonNamed(page, "More actions"));
    await click(buttonWithText(page, "Versions"));
    const offered = [...(page.querySelector("dialog")?.querySelectorAll("a") ?? [])].map((link) => [
      link.textContent,
      link.getAttribute("href"),
    ]);
    expect(offered).toEqual([
      ["Play version 1", "/play/1"],
      ["Play version 2", "/play/5"],
    ]);
    mounted.unmount();
  });

  it("offers no choice of version where there is one", async () => {
    const mounted = await mount(withServices(<TitleView title={heat} />));
    const page = mounted.container;
    // Nothing else is under a person's "…" there, so there is none.
    expect(buttonNamed(page, "More actions")).toBeUndefined();
    expect(buttonWithText(page, "Versions")).toBeUndefined();
    mounted.unmount();
  });

  // D418 §5: a title's technical detail lives in one place, the section below its cast.
  it("keeps its File details below its cast alone, none under its …", async () => {
    const twoVersions: TitleDetail = {
      ...heat,
      versions: [
        ...(heat.versions ?? []),
        { id: 2, files: [{ id: 5, path: "/m/Metropolis (1927) 4K.mkv", streams: [] }] },
      ],
    };
    const mounted = await mount(withServices(<TitleView title={twoVersions} />));
    const page = mounted.container;
    expect(page.querySelectorAll("section details.title__details")).toHaveLength(2);
    await click(buttonNamed(page, "More actions"));
    expect(buttonWithText(page, "Versions")).toBeDefined();
    expect(buttonWithText(page, "File details")).toBeUndefined();
    mounted.unmount();
  });
});
