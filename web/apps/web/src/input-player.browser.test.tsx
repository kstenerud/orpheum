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

import { fillsWindowClass } from "./film-fullscreen.ts";
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
    // The subtitle and audio menus are choosers (the toolkit batch's step 6), no longer selects.
    expect(controls.some((control) => control.tagName === "SELECT")).toBe(false);
    expect(
      controls.filter((control) => control.classList.contains("route-chooser__button")).length,
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

  // D407 §9: outside full screen nothing lies over the film; the controls are under it.
  it("draws nothing over the film outside full screen, its controls under it", async () => {
    vi.spyOn(Element.prototype, "requestFullscreen").mockRejectedValue(new Error("refused"));
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmToPlay, "/play/1030"));
    const video = await theFailedVideo();
    const stage = video.closest(".film-player__stage");
    // The video, and the subtitles' layer over it where a subtitle shows (JASSUB's canvas).
    const over = [...(stage?.children ?? [])].filter(
      (child) => child !== video && child.tagName !== "CANVAS",
    );
    expect(over.map((child) => child.outerHTML.slice(0, 80))).toEqual([]);
    const controls = document.querySelector(".page-controls");
    expect(controls?.querySelector('button[aria-label="Play"]')).not.toBeNull();
    expect(stage?.contains(controls ?? null)).toBe(false);
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
      // A television's player opens with all of its overlay showing, which its first Back hides
      // (D407 §5); the next leaves the page. The overlay comes once the refused full screen has
      // fallen back to filling the window, a promise's settling later, so it is waited for.
      await expect
        .poll(() => document.querySelector(".player-overlay") !== null)
        .toBe(isTelevision);
      const overlay = document.querySelector(".player-overlay");
      if (overlay !== null) {
        await press("{Escape}");
        await expect.poll(() => overlay.getAttribute("data-shown")).toBe("hidden");
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
    const scrub = document.querySelector<HTMLInputElement>(".player-overlay .timeline__range");
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
    const scrub = document.querySelector<HTMLInputElement>(".player-overlay .timeline__range");
    scrub?.focus();
    await press("{ArrowRight>4/}"); // held: three repeats, then released
    expect(scrub?.getAttribute("aria-valuetext")).toBe("1:10");
    expect(seeks).toEqual([]);
    await expect.poll(() => seeks).toEqual([70]);
    expect(scrub?.getAttribute("aria-valuetext")).toBe("1:10"); // until the seek lands
    expect(document.activeElement).toBe(scrub);
  });

  /** The overlay's state, as it draws it. */
  function shown(): string | null | undefined {
    return document.querySelector(".player-overlay")?.getAttribute("data-shown");
  }

  /** Hides the overlay the player opens with, by Back (D407 §5), focus left on the video. */
  async function hideTheOverlay(video: HTMLVideoElement): Promise<void> {
    await expect.poll(() => document.activeElement?.closest(".player-overlay")).toBeTruthy();
    await press("{Escape}");
    await expect.poll(shown).toBe("hidden");
    expect(document.activeElement).toBe(video);
  }

  // D407 §3: Up and Down walk the rows, Up from the scrub bar to "‹ Film 3", which goes up.
  it("walks its rows by Up and Down, from the buttons to the scrub bar and up to the film's page", async () => {
    await onAFireTv();
    const play = document.querySelector<HTMLElement>('.player-overlay [aria-label="Play"]');
    const scrub = document.querySelector<HTMLElement>(".player-overlay .timeline__range");
    const upLink = document.querySelector<HTMLElement>(".player-overlay .player-overlay__up");
    expect(document.querySelector(".player-overlay .back-button")).toBeNull();
    expect(upLink?.textContent).toBe("Film 3");
    await expect.poll(() => document.activeElement).toBe(play);
    await press("{ArrowUp}");
    expect(document.activeElement).toBe(scrub);
    await press("{ArrowUp}");
    expect(document.activeElement).toBe(upLink);
    await press("{ArrowDown}{ArrowDown}");
    expect(document.activeElement).toBe(play);
    await press("{ArrowUp}{ArrowUp}{Enter}");
    await expect
      .poll(() => routeHref(walking?.services.navigation.route() ?? { screen: "home" }))
      .toBe(routeHref({ screen: "title", itemId: 103 }));
  });

  // Plan item 17 (2026-11-02): the buttons hid between presses, and Right meant for Full screen
  // sought the film instead. Their row now stays while focus walks it, and Right never seeks there.
  // The menus 💬, 🎧 and ⁝ stand between ⏩ and Full screen (D407 §1, §8).
  it("walks its buttons by Right to Full screen, the film not seeking", async () => {
    const video = await onAFireTv();
    const positionAt = standsAt(video, 30);
    const fullScreen = document.querySelector<HTMLElement>(".player-overlay .full-screen-button");
    await expect.poll(() => document.activeElement?.getAttribute("aria-label")).toBe("Play");
    const walked: (string | null | undefined)[] = [];
    for (let step = 0; step < 5; step += 1) {
      await press("{ArrowRight}");
      walked.push(document.activeElement?.getAttribute("aria-label"));
    }
    expect(walked).toEqual([
      "Forward 10 s",
      "Subtitles: Off",
      "Sound: English",
      "More",
      "Full screen",
    ]);
    expect(document.activeElement).toBe(fullScreen);
    expect(shown()).toBe("full");
    expect(positionAt()).toBe(30);
  });

  it("seeks by Left and Right while hidden, the scrub bar alone showing", async () => {
    const video = await onAFireTv();
    const positionAt = standsAt(video, 30);
    await hideTheOverlay(video);
    await press("{ArrowRight}");
    await expect.poll(positionAt).toBeCloseTo(30 + seekStepMs / 1000, 1);
    expect(shown()).toBe("scrub");
    expect(document.activeElement).toBe(video);
  });

  it("pauses by OK while hidden, showing the pause sign and nothing else", async () => {
    const video = await onAFireTv();
    Object.defineProperty(video, "paused", { configurable: true, get: () => false });
    const pause = vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => undefined);
    await hideTheOverlay(video);
    await press("{Enter}");
    expect(pause).toHaveBeenCalledTimes(1);
    await expect.poll(shown).toBe("paused");
    expect(document.activeElement).toBe(video);
  });

  // Up opened the overlay on the Fire TV with nothing focused, so OK paused nothing and no arrow
  // reached its controls (2026-11-02): play was focused at the next frame, which the stick ran
  // before React had drawn the overlay, its play then without a box. The frame runs at once here.
  it("opens all of the hidden overlay by Up with focus on play, however soon the next frame comes", async () => {
    const video = await onAFireTv();
    await hideTheOverlay(video);
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
      callback(performance.now());
      return 0;
    });
    await press("{ArrowUp}");
    const play = document.querySelector('.player-overlay [aria-label="Play"]');
    expect(play).not.toBeNull();
    await expect.poll(() => document.activeElement).toBe(play);
    expect(shown()).toBe("full");
  });

  // A cast's receiver, whose overlay this is, did nothing at Space (the owner, Chrome, 2026-10-27).
  // Space on a button presses it (D407 §5); on the scrub bar, which it cannot press, it plays.
  it("plays and pauses by Space on its scrub bar, which Space does not press", async () => {
    const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
    await onAFireTv();
    const scrub = document.querySelector<HTMLInputElement>(".player-overlay .timeline__range");
    scrub?.focus();
    const playsBefore = play.mock.calls.length;
    await press(" ");
    expect(play.mock.calls.length).toBe(playsBefore + 1);
    expect(document.activeElement).toBe(scrub);
  });

  // D407 §7, §9: a Fire TV's Back leaves the browser's full screen itself, the page never hearing
  // it; the controls are then under the film, focus on play, and the next Back goes to the film's
  // page.
  it("shows the controls under the film as full screen is left, focus on play, and goes to the film's page by the next Back", async () => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(fireTvSilk);
    let fullScreen: Element | null = null;
    vi.spyOn(Document.prototype, "fullscreenElement", "get").mockImplementation(() => fullScreen);
    vi.spyOn(Element.prototype, "requestFullscreen").mockImplementation(() => {
      fullScreen = document.querySelector(".film-player__stage");
      document.dispatchEvent(new Event("fullscreenchange"));
      return Promise.resolve();
    });
    await page.viewport(960, 480);
    const services = walkServices(filmToPlay, "/play/1030");
    walking = await mountApp(services);
    await theFailedVideo();
    await expect.poll(() => isDrawn(".player-overlay")).toBe(true);
    fullScreen = null;
    document.dispatchEvent(new Event("fullscreenchange"));
    await expect.poll(() => isDrawn(".player-overlay")).toBe(false);
    const play = document.querySelector(".page-controls button[aria-label='Play']");
    expect(play).not.toBeNull();
    await expect.poll(() => document.activeElement).toBe(play);
    expect(document.querySelector(".page-controls .player-overlay__up")).not.toBeNull();
    await press("{Escape}");
    await expect
      .poll(() => routeHref(services.navigation.route()))
      .toBe(routeHref({ screen: "title", itemId: 103 }));
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
  // (the owner, Chrome, 2026-10-27): focus on the browser's controls is focus on their video, where
  // Space plays and pauses. D407 §5 reverses Space on the player's own buttons, which once played
  // too: Space presses the focused button, as OK does, so on full screen it goes full screen, and
  // the player's controls are then the overlay.
  it("plays and pauses by Space with focus on the video, and presses the player's button focused", async () => {
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
    expect(fullScreen).not.toBeNull();
    fullScreen?.focus();
    await press(" ");
    await expect.poll(() => isDrawn(".player-overlay")).toBe(true);
    const stage = document.querySelector(".film-player__stage");
    expect(
      document.fullscreenElement === stage || stage?.classList.contains(fillsWindowClass),
    ).toBe(true);
    expect(document.querySelector(".page-controls")).toBeNull();
    expect(play.mock.calls.length).toBe(playsBefore + 1);
    if (document.fullscreenElement !== null) {
      await document.exitFullscreen();
    }
  });
});

describe("the media keys' next and previous on a Play page, in a computer's browser (D407 §10)", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  // A keyboard's ⏭ plays the next episode, as Up next does, and the browser's media controls are
  // given next track only once there is a next, previous track not at all where there is no previous:
  // Chrome shows a button for each action registered. Their next track's press is the same command
  // (input-commands.test.tsx).
  it("plays the next episode by MediaTrackNext, the browser's media controls given next alone", async () => {
    const registered = new Set<MediaSessionAction>();
    const setActionHandler = navigator.mediaSession.setActionHandler.bind(navigator.mediaSession);
    vi.spyOn(navigator.mediaSession, "setActionHandler").mockImplementation((action, handler) => {
      if (handler === null) {
        registered.delete(action);
      } else {
        registered.add(action);
      }
      setActionHandler(action, handler);
    });
    await page.viewport(1280, 800);
    const services = walkServices(
      {
        ...filmToPlay,
        "GET /files/1030/up-next": {
          upNext: {
            episode: {
              titleId: 104,
              fileId: 1040,
              name: "Bag",
              groupNumber: 1,
              numberInGroup: 3,
              isPartway: false,
            },
            ordering: "aired",
          },
        },
      },
      "/play/1030",
    );
    walking = await mountApp(services);
    const video = await theFailedVideo();
    expect(registered.has("nexttrack")).toBe(false);
    // What plays next is asked for once the film has played (up-next.tsx).
    Object.defineProperty(video, "paused", { configurable: true, get: () => false });
    video.dispatchEvent(new Event("playing"));
    await expect.poll(() => registered.has("nexttrack")).toBe(true);
    expect(registered.has("previoustrack")).toBe(false);
    // The browser keeps a media key the test's driver presses from the page (no keydown reaches it,
    // in Chromium nor Firefox), so the key comes as the page hears it from a keyboard that sends one.
    video.dispatchEvent(
      new KeyboardEvent("keydown", { key: "MediaTrackNext", bubbles: true, cancelable: true }),
    );
    await expect
      .poll(() => routeHref(services.navigation.route()))
      .toBe(routeHref({ screen: "play", fileId: 1040 }));
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
