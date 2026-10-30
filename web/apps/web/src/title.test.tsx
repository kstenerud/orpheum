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
  kind: "movie",
  name: "Metropolis",
  year: 1995,
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

  it("shows no broken banner or still art not made yet, through the posters' own way (the owner, 2026-10-26)", async () => {
    // The title page drew its backdrop and an episode's still as plain images, so art not made
    // yet, or never made, showed as a broken image across the top of the page (staging answered
    // 404 for 12 Marrow Larks's landscape-l).
    const page = await mount(withServices(<TitleView title={heat} />));
    const backdrop = (): Element | null => page.container.querySelector("img.title__backdrop");
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
        "PUT /interest/10": [204, null],
        "DELETE /interest/10": [204, null],
        "PUT /interest/11": [204, null],
        "DELETE /interest/11": [204, null],
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
          "PUT /interest/10": [204, null],
          "DELETE /interest/10": [204, null],
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
              { ...heat, id: 11, unavailable: { reason: "node_away" as const, nodeName: "devnas" } },
            ],
            "PUT /interest/11": [204, null],
            "DELETE /interest/11": [204, null],
          },
          heard,
        ),
      ),
    );
    expect(heard.map((request) => request.methodAndPath)).toContain("PUT /interest/11");
    page.unmount();
    await settleUntil(() =>
      heard.some((request) => request.methodAndPath === "DELETE /interest/11"),
    );
    expect(heard.at(-1)?.methodAndPath).toBe("DELETE /interest/11");
  });

  it("offers Play by itself once its file is read, or can be read again (the owner, 2026-10-24)", async () => {
    // Opened while its file is still being read, or while its machine is away, the page asks again
    // on the background poller (D237) and offers Play once the server says the file plays.
    const soon = backingOffPoller({ firstMs: 50, longestMs: 50, failedMs: 50 });
    const cases = [
      { id: 13, reason: "still_reading" as const },
      { id: 14, reason: "node_away" as const },
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
          [`PUT /interest/${String(id)}`]: [204, null],
          [`DELETE /interest/${String(id)}`]: [204, null],
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
          [200, { ...title, unavailable: { reason: "still_reading" as const, nodeName: "devnas" } }],
          unanswered,
          [200, title],
        ],
        "POST /playback": [
          200,
          { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a" },
        ],
        "PUT /interest/15": [204, null],
        "DELETE /interest/15": [204, null],
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
              { providerId: "949", name: "Metropolis", year: 1995 },
              { providerId: "50", name: "Metropolis", year: 1986 },
            ],
          },
        ],
        "POST /items/12/match": [204, {}],
        "POST /playback": [
          200,
          { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a" },
        ],
        "PUT /interest/12": [204, null],
        "DELETE /interest/12": [204, null],
      },
      heard,
    );
    const forUser = await mount(withServices(<Title id={12} />, services));
    expect(forUser.container.textContent).toContain("may be another work of the same name");
    expect(buttonNamed(forUser.container, "More actions")).toBeUndefined();
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
    // One dialog, in the list's place (the owner, 2026-10-29): Rescan, Enter manually, Report this.
    expect(page.querySelectorAll("dialog")).toHaveLength(1);
    const offered = [...(page.querySelector("dialog")?.querySelectorAll("button") ?? [])].map(
      (button) => button.textContent,
    );
    // Its Close is the cross every dialog has (the owner, 2026-10-30).
    expect(offered).toEqual(["Rescan", "Enter manually", "Report this", ""]);
    expect(page.querySelector("dialog .modal-dialog__close")?.getAttribute("aria-label")).toBe(
      "Close",
    );
    await click(buttonWithText(page, "Enter manually"));
    await settleUntil(() => page.textContent.includes("1986"));
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
});
