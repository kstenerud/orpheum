// SPDX-License-Identifier: MPL-2.0

/**
 * The player walked with a remote's keys in a real browser at a Fire TV's page in Silk and a
 * desktop's window (controller-input-2026-10-25.md §6.1 item 4): every control under the video
 * reached by arrows, the video seeked by Left and Right and played and paused by OK, and Back
 * leaving the page for the film's, focus where it was.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";

import { focusTargetsIn } from "./focus-engine.ts";
import { routeHref } from "./router.ts";
import { seekStepMs } from "./seek-ahead.ts";
import { directDecision } from "./test-fixtures.ts";
import {
  filmDetail,
  isDrawn,
  mountApp,
  reachedFrom,
  viewer,
  walkServices,
  type Walking,
  type WalkAnswers,
} from "./test-walk.tsx";

/** The screens the walks are made at: a Fire TV's page in Silk, and a desktop's window. */
const screens = [
  { name: "a Fire TV's page in Silk", width: 960, height: 480 },
  { name: "a desktop's window", width: 1280, height: 800 },
] as const;

/** Film 3's page and its file's playback, whose media the walk does not serve. */
const filmToPlay: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /items/103": filmDetail(103, "Film 3"),
  "POST /playback": directDecision({
    titleName: "Film 3",
    plays: { titleId: 103, kind: "movie", libraryId: 1 },
    url: "/no-such-media",
    subtitles: [
      {
        streamId: 5,
        language: "en",
        isDefault: false,
        isForced: false,
        isHearingImpaired: false,
        assUrl: "/no-such-subtitle.ass",
        vttUrl: "/no-such-subtitle.vtt",
      },
    ],
    audioTracks: [
      { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
      { streamId: 81, audioTrackIndex: 1, language: "fr", kind: "commentary" },
    ],
  }),
};

/** Presses keys as a remote does: real key events, the browser's own handling with them. */
async function press(keys: string): Promise<void> {
  await userEvent.keyboard(keys);
}

/** The page's video, once drawn. */
async function theVideo(): Promise<HTMLVideoElement> {
  await expect.poll(() => isDrawn("video")).toBe(true);
  const video = document.querySelector("video");
  if (video === null) {
    throw new Error("the player draws no video");
  }
  return video;
}

/**
 * The page's video, once its load has failed, as it does with no media served: the player then
 * says so under it and changes nothing more, so a walk from it meets the page as it stays. A walk
 * begun before, on a loaded machine, met a page the failure's words then moved (2026-10-25).
 */
async function theFailedVideo(): Promise<HTMLVideoElement> {
  const video = await theVideo();
  await expect.poll(() => video.error?.code).toBeDefined();
  return video;
}

describe.each(screens)("the player by a remote's keys, on $name", ({ width, height }) => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  it("reaches every control under the video by arrows, starting on the video", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmToPlay, "/play/1030"));
    const video = await theFailedVideo();
    await press("{ArrowDown}");
    expect(document.activeElement).toBe(video);
    const reached = await reachedFrom(video, press);
    const player = document.querySelector(".play");
    const controls = focusTargetsIn(player ?? document.body);
    // This film's: the video, Start over, the subtitle and audio menus, Find subtitles, Level
    // loudness and the bare player.
    expect(controls.map((control) => control.textContent)).toEqual(
      expect.arrayContaining(["Start over", "Find subtitles…", "Play in a bare player"]),
    );
    expect(
      controls.filter((control) => control.tagName === "SELECT").length,
    ).toBeGreaterThanOrEqual(2);
    expect(controls[0]).toBe(video);
    const unreached = controls.filter((control) => !reached.has(control));
    expect(unreached.map((control) => control.outerHTML.slice(0, 80))).toEqual([]);
  });

  it("seeks by Left and Right and plays and pauses by OK on the video", async () => {
    const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmToPlay, "/play/1030"));
    const video = await theFailedVideo();
    video.focus();
    expect(document.activeElement).toBe(video);
    // The walk serves no media, so the browser's own position comes and goes as the load fails: the
    // video stands where the player last moved it, 30 s into a ten-minute film.
    let positionS = 30;
    Object.defineProperty(video, "duration", { configurable: true, get: () => 600 });
    Object.defineProperty(video, "currentTime", {
      configurable: true,
      get: () => positionS,
      set: (toS: number) => {
        positionS = toS;
      },
    });
    // Each seek is made once the presses rest (seek-ahead.ts).
    await press("{ArrowRight}");
    await expect.poll(() => video.currentTime).toBeCloseTo(30 + seekStepMs / 1000, 1);
    await press("{ArrowLeft}{ArrowLeft}");
    await expect.poll(() => video.currentTime).toBeCloseTo(30 - seekStepMs / 1000, 1);
    expect(document.activeElement).toBe(video);
    const playsBefore = play.mock.calls.length;
    await press("{Enter}");
    expect(play.mock.calls.length).toBe(playsBefore + 1);
  });

  it("goes back to the film's page by Back, to its Play", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmToPlay, "/titles/103"));
    await expect.poll(() => isDrawn(".title__play")).toBe(true);
    const playLink = document.querySelector<HTMLElement>(".title__play");
    playLink?.focus();
    await press("{Enter}");
    await theVideo();
    await press("{Escape}");
    await expect.poll(() => document.activeElement?.classList.contains("title__play")).toBe(true);
  });
});

describe("a Play page opened with no page of this app behind it", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  // A Fire TV reloaded a playing film's page: no history behind it, no Back button on a television,
  // and its remote's Back went nowhere (the owner, 2026-10-27). Back goes up to the film's page, the
  // one it plays from ("Would it be better to go back to the non-fullscreen page?"), there as on a
  // computer.
  for (const [name, userAgent, isTelevision] of [
    [
      "a Fire TV's Silk",
      "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36",
      true,
    ],
    ["a computer's browser", navigator.userAgent, false],
  ] as const) {
    it(`goes up to the film's page by Back, on ${name}`, async () => {
      vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(userAgent);
      // Full screen refused, the player fills the window instead (film-fullscreen.ts): a browser in
      // full screen takes Escape for its own leaving, which Firefox's does before the page hears it.
      vi.spyOn(Element.prototype, "requestFullscreen").mockRejectedValue(new Error("refused"));
      await page.viewport(960, 480);
      const services = walkServices(filmToPlay, "/play/1030");
      walking = await mountApp(services);
      await theVideo();
      // A television's player opens with its overlay showing, which its first Back hides
      // (tv-controls.tsx); the next leaves the page.
      const overlay = document.querySelector(".tv-controls");
      expect(overlay !== null).toBe(isTelevision);
      if (overlay !== null) {
        await press("{Escape}");
        await expect.poll(() => overlay.getAttribute("data-shown")).toBe("none");
      }
      await press("{Escape}");
      await expect
        .poll(() => routeHref(services.navigation.route()))
        .toBe(routeHref({ screen: "title", itemId: 103 }));
    });
  }
});

describe("a television's player overlay, by a remote's keys, in a Fire TV's Silk", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  /** The Play page as a Fire TV's Silk opens it, full screen refused, and its video. */
  async function onAFireTv(): Promise<HTMLVideoElement> {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(fireTvSilk);
    vi.spyOn(Element.prototype, "requestFullscreen").mockRejectedValue(new Error("refused"));
    await page.viewport(960, 480);
    walking = await mountApp(walkServices(filmToPlay, "/play/1030"));
    return await theFailedVideo();
  }

  // Left and Right on the focused scrub bar did nothing on the owner's Fire TV (2026-10-27): the
  // range kept its arrows from the input layer, and its own keydown's preventDefault had the layer
  // leave them alone.
  it("seeks by Left and Right on the scrub bar, focus staying there", async () => {
    const video = await onAFireTv();
    const positionAt = standsAt(video, 30);
    const scrub = document.querySelector<HTMLInputElement>(".tv-controls .timeline__range");
    scrub?.focus();
    expect(document.activeElement).toBe(scrub);
    await press("{ArrowRight}");
    await expect.poll(positionAt).toBeCloseTo(30 + seekStepMs / 1000, 1);
    await press("{ArrowLeft}{ArrowLeft}");
    await expect.poll(positionAt).toBeCloseTo(30 - seekStepMs / 1000, 1);
    expect(document.activeElement).toBe(scrub);
  });

  // Each repeat of a held Right seeked the video, the marker moving only as each landed, so a
  // person guessed how long to hold (the owner, Fire TV, 2026-10-29).
  it("runs the scrub bar's time ahead while Right is held, and seeks once, there, after its release", async () => {
    const video = await onAFireTv();
    const seeks: number[] = [];
    Object.defineProperty(video, "duration", { configurable: true, get: () => 600 });
    Object.defineProperty(video, "currentTime", {
      configurable: true,
      get: () => 30,
      set: (toS: number) => {
        seeks.push(toS);
      },
    });
    const scrub = document.querySelector<HTMLInputElement>(".tv-controls .timeline__range");
    scrub?.focus();
    await press("{ArrowRight>4/}"); // held: three repeats, then released
    expect(scrub?.getAttribute("aria-valuetext")).toBe("1:10");
    expect(seeks).toEqual([]);
    await expect.poll(() => seeks).toEqual([70]);
    expect(scrub?.getAttribute("aria-valuetext")).toBe("1:10"); // until the seek lands
    expect(document.activeElement).toBe(scrub);
  });

  // D367 §6, §8: Up, "‹ Film 3", and an on-screen Back, since TV Bro's remote Back never reaches the
  // page and the app cannot tell it from Silk; each reached by the arrows from the overlay's controls.
  it("offers Up to the film's page and its own Back, each reached by the arrows", async () => {
    await onAFireTv();
    const upLink = document.querySelector<HTMLElement>(".tv-controls .tv-controls__up");
    const back = document.querySelector<HTMLElement>(".tv-controls .back-button");
    expect(upLink?.textContent).toBe("‹ Film 3");
    const timeline = document.querySelector<HTMLElement>(".tv-controls .timeline__range");
    if (timeline === null || upLink === null || back === null) {
      throw new Error("the overlay draws no timeline, Up or Back");
    }
    const reached = await reachedFrom(timeline, press);
    expect(reached.has(upLink)).toBe(true);
    expect(reached.has(back)).toBe(true);
    upLink.focus();
    await press("{Enter}");
    await expect
      .poll(() => routeHref(walking?.services.navigation.route() ?? { screen: "home" }))
      .toBe(routeHref({ screen: "title", itemId: 103 }));
  });

  it("goes back by its own Back, up to the film's page with nothing of the app behind", async () => {
    await onAFireTv();
    const back = document.querySelector<HTMLElement>(".tv-controls .back-button");
    back?.focus();
    await press("{Enter}");
    await expect
      .poll(() => routeHref(walking?.services.navigation.route() ?? { screen: "home" }))
      .toBe(routeHref({ screen: "title", itemId: 103 }));
  });

  // A cast's receiver, whose overlay this is, did nothing at Space (the owner, Chrome, 2026-10-27).
  it("plays and pauses by Space on any of its controls", async () => {
    const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
    await onAFireTv();
    const scrub = document.querySelector<HTMLInputElement>(".tv-controls .timeline__range");
    scrub?.focus();
    const playsBefore = play.mock.calls.length;
    await press(" ");
    expect(play.mock.calls.length).toBe(playsBefore + 1);
    expect(document.activeElement).toBe(scrub);
  });
});

describe("the Play page's trail, in a computer's browser (D367 §6–§8)", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  it("shows where the film is, with no Back of its own, and its Up goes back to the film's page, adding nothing", async () => {
    await page.viewport(1280, 800);
    walking = await mountApp(walkServices(filmToPlay, "/titles/103"));
    const { navigation } = walking.services;
    await expect.poll(() => isDrawn(".title__play")).toBe(true);
    document.querySelector<HTMLElement>(".title__play")?.click();
    await theVideo();
    const crumbs = [...document.querySelectorAll(".up-trail li")].map((each) => each.textContent);
    expect(crumbs).toEqual(["Library", "Film 3", "Playing"]);
    expect(document.querySelector(".back-button")).toBeNull();
    const filmCrumb = [...document.querySelectorAll<HTMLAnchorElement>(".up-trail a")].find(
      (link) => link.textContent === "Film 3",
    );
    filmCrumb?.focus();
    await press("{Enter}");
    await expect
      .poll(() => routeHref(navigation.route()))
      .toBe(routeHref({ screen: "title", itemId: 103 }));
    // Back to the film's page, not a new one over the player: Back from here leaves the app.
    expect(navigation.behind()).toEqual([]);
  });
});

describe("a series returned to by Up from an episode of another season (D367 §9)", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  /** An episode of Long Run (5): its id, and its place. */
  function episodeOf(id: number, season: number): Record<string, unknown> {
    return {
      id,
      seasonNumber: season,
      episodeNumber: 1,
      name: `Episode ${String(id)}`,
      identificationState: "identified",
      hasLiveFile: true,
    };
  }

  // A binge from season 1 into season 2 left the series' entry at season 1 (§3.7): returning to it
  // shows season 2, the episode last reached focused, on the same page, not one drawn anew.
  it("shows that episode's season, the episode focused", async () => {
    await page.viewport(1280, 800);
    walking = await mountApp(
      walkServices(
        {
          "GET /me": viewer,
          "GET /client-settings": {},
          "GET /collections/5": {
            id: 5,
            kind: "series",
            name: "Long Run",
            art: {},
            seasons: [1, 2].map((seasonNumber) => ({ seasonNumber, episodeCount: 1 })),
          },
          "GET /collections/5/seasons/1": { episodes: [episodeOf(21, 1)] },
          "GET /collections/5/seasons/2": { episodes: [episodeOf(23, 2)] },
          "GET /items/23": {
            id: 23,
            kind: "episode",
            name: "Episode 23",
            identificationState: "identified",
            art: {},
            versions: [],
            extras: [],
            episode: { seriesId: 5, seriesName: "Long Run", seasonNumber: 2, episodeNumber: 1 },
          },
        },
        "/series/5?season=1",
      ),
    );
    const { navigation } = walking.services;
    await expect.poll(() => isDrawn('a[href="/titles/21"]')).toBe(true);
    navigation.go({ screen: "title", itemId: 23 });
    const seriesCrumb = (): HTMLAnchorElement | undefined =>
      [...document.querySelectorAll<HTMLAnchorElement>(".up-trail a")].find(
        (link) => link.textContent === "Long Run",
      );
    await expect.poll(() => seriesCrumb() !== undefined).toBe(true);
    seriesCrumb()?.click();
    await expect.poll(() => document.activeElement?.getAttribute("href")).toBe("/titles/23");
    expect(routeHref(navigation.route())).toBe("/series/5?season=2");
    expect(navigation.behind()).toEqual([]);
  });
});

describe("Space on an ordinary Play page, in a computer's browser", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  // After a click on the browser's own full-screen button, focus on it, Space pressed it again
  // (the owner, Chrome, 2026-10-27): focus on the browser's controls is focus on their video. Since
  // D369 the page has the player's own controls in place of the browser's, which Space plays and
  // pauses on too, once, never pressing the one focused as well.
  it("plays and pauses by Space with focus on the video and its own controls", async () => {
    const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
    await page.viewport(1280, 800);
    walking = await mountApp(walkServices(filmToPlay, "/play/1030"));
    const video = await theFailedVideo();
    expect(video.controls).toBe(false);
    video.focus();
    const playsBefore = play.mock.calls.length;
    await press(" ");
    expect(play.mock.calls.length).toBe(playsBefore + 1);
    expect(document.activeElement).toBe(video);
    const fullScreen = document.querySelector<HTMLElement>(".page-controls .full-screen-button");
    fullScreen?.focus();
    await press(" ");
    expect(play.mock.calls.length).toBe(playsBefore + 2);
    expect(document.fullscreenElement).toBeNull();
  });
});

/** A Fire TV Stick's Silk, as the owner's says itself. */
const fireTvSilk =
  "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36";

/**
 * Has video stand atS seconds into a ten-minute film, moved only by the player's seeks, as the walk
 * serves no media; returns where it stands, in seconds.
 */
function standsAt(video: HTMLVideoElement, atS: number): () => number {
  let positionS = atS;
  Object.defineProperty(video, "duration", { configurable: true, get: () => 600 });
  Object.defineProperty(video, "currentTime", {
    configurable: true,
    get: () => positionS,
    set: (toS: number) => {
      positionS = toS;
    },
  });
  return () => positionS;
}
