// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { tellNextChunkLoadMs } from "@orpheum/sdk";
import { act, createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { spinnerAfterMs } from "./film-waits.tsx";
import { commandEventName } from "./input-commands.ts";
import { PlayView } from "./play.tsx";
import { directDecision } from "./test-fixtures.ts";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/**
 * A film getting ready to play shows it (the owner, 2026-11-15, Chrome on an Android phone: "when I
 * go from the info page to playing, it should show the pause button, not the play button, because
 * it's trying to play ... We use the spinner to communicate that it's trying to play but can't
 * yet"): the Play page, opened by Play on a film's page, shows Pause and, after spinnerAfterMs, a
 * spinner, from its opening until its first frame plays, and again while a seek's hold or a stall
 * waits for data; a pause the person asked for, and a start the browser refused, show ▶ and none.
 */

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** Loaded ranges, as a video's buffered says them. */
function ranges(loaded: readonly (readonly [number, number])[]): TimeRanges {
  return {
    length: loaded.length,
    start: (index: number) => loaded[index]?.[0] ?? 0,
    end: (index: number) => loaded[index]?.[1] ?? 0,
  };
}

/** Fires event at video, and lets what it starts settle. */
async function fire(video: HTMLVideoElement, event: string): Promise<void> {
  await act(async () => {
    video.dispatchEvent(new Event(event));
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

/** Lets ms pass, and what it starts settle. */
async function waitMs(ms: number): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  });
}

/** What a test does to the Play page's video and reads of it. */
interface FakeFilm {
  readonly plays: () => number;
  readonly load: (loaded: readonly (readonly [number, number])[]) => void;
  readonly setReadyState: (readyState: number) => void;
}

/**
 * The Play page's video at its start: at 0, 2 s loaded, its next chunk expected to take 2.3 s, so
 * its start gate waits. Its plays are counted, and refused where isRefused says (NotAllowedError),
 * as a browser refuses a start without a press.
 */
function fakeFilm(video: HTMLVideoElement, isRefused = false): FakeFilm {
  let plays = 0;
  let isPaused = true;
  let readyState = 4;
  let buffered = ranges([[0, 2]]);
  Object.defineProperty(video, "readyState", { get: () => readyState });
  Object.defineProperty(video, "networkState", { value: 2 }); // loading
  Object.defineProperty(video, "currentTime", { value: 0, writable: true });
  Object.defineProperty(video, "paused", { get: () => isPaused });
  Object.defineProperty(video, "buffered", { get: () => buffered });
  Object.defineProperty(video, "play", {
    value: async (): Promise<void> => {
      plays += 1;
      if (isRefused) {
        throw new DOMException("play() needs a press", "NotAllowedError");
      }
      isPaused = false;
      await Promise.resolve();
    },
  });
  Object.defineProperty(video, "pause", {
    value: () => {
      isPaused = true;
    },
  });
  tellNextChunkLoadMs(video, () => ({ loadMs: 2300, from: "confident" }));
  return {
    plays: () => plays,
    load: (loaded) => {
      buffered = ranges(loaded);
    },
    setReadyState: (state) => {
      readyState = state;
    },
  };
}

/** The Play page as Play on a film's page opens it, playing a direct film. */
async function openedFromTheFilmsPage(token: string): Promise<HTMLVideoElement> {
  mounted = await mount(
    withServices(
      createElement(PlayView, {
        fileId: 2,
        decision: directDecision({ mediaToken: token }),
        onDecideAgain: () => undefined,
      }),
      servedBy({
        [`POST /playback/${token}/report`]: [204, undefined],
        "GET /files/2/waits": [200, {}],
        "GET /files/2/up-next": [200, {}],
        "GET /files/2/availability": [200, {}], // a failed load asks why
        [`PUT /playback/${token}/wants`]: [204, null],
        [`DELETE /playback/${token}`]: [204, null],
      }),
    ),
  );
  const video = mounted.container.querySelector("video");
  if (video === null) {
    throw new Error("no video");
  }
  return video;
}

/** Whether the spinner shows over the film. */
function isSpinnerShown(): boolean {
  const spinner = mounted?.container.querySelector(".film-waits");
  return spinner instanceof HTMLElement && spinner.hidden === false;
}

/**
 * The overlay's play button, the one play button of a phone's player in full screen, where the bar
 * under the film is not drawn.
 */
function playButton(): HTMLButtonElement {
  const button = mounted?.container.querySelector(
    '.player-overlay button:is([aria-label="Play"], [aria-label="Pause"])',
  );
  if (!(button instanceof HTMLButtonElement)) {
    throw new Error("no play button");
  }
  return button;
}

/**
 * The play button's label, "Play" or "Pause", as its person sees it: the overlay shown first, as a
 * tap shows it (hidden, it reads the film at its own events alone, and is not seen).
 */
async function shownPlayLabel(): Promise<string | null> {
  await showTheOverlay();
  return playButton().getAttribute("aria-label");
}

/** Shows the overlay, hidden as the player opens, as a mouse's move does (two moves). */
async function showTheOverlay(): Promise<void> {
  await act(async () => {
    for (const screenX of [5, 50]) {
      const move = { bubbles: true, pointerType: "mouse", screenX, screenY: 5 };
      document.body.dispatchEvent(new PointerEvent("pointermove", move));
    }
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

/** Presses the overlay's play button. */
async function pressPlay(): Promise<void> {
  await act(async () => {
    playButton().focus(); // as a tap or a click leaves it
    playButton().click();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

/** Sends a remote's OK, as the input layer does. */
async function pressOk(): Promise<void> {
  await act(async () => {
    const detail = { command: "select", source: "key" };
    document.body.dispatchEvent(
      new CustomEvent(commandEventName, { detail, bubbles: true, cancelable: true }),
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

describe("a film getting ready to play shows it (the owner, 2026-11-15)", () => {
  it("opened from its film's page, shows Pause at once and the spinner after a moment, until its first frame plays", async () => {
    // The spinner's wait passes as the test lets it, from the page's mount, not as a loaded
    // machine drags out the run's (a full gate's run showed it before the first look).
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    let video: HTMLVideoElement;
    let film: FakeFilm;
    try {
      let mountedMs = 0;
      let opened: HTMLVideoElement | undefined;
      void openedFromTheFilmsPage("start").then((shown) => (opened = shown));
      while (opened === undefined) {
        await vi.advanceTimersByTimeAsync(5);
        mountedMs += 5;
      }
      video = opened;
      film = fakeFilm(video);
      act(() => {
        video.dispatchEvent(new Event("loadstart")); // the start gate waits: 2 s loaded, its next chunk 2.3 s away
      });
      expect(mountedMs).toBeLessThan(spinnerAfterMs);
      expect(isSpinnerShown()).toBe(false); // a quick start shows none
      await act(async () => {
        await vi.advanceTimersByTimeAsync(spinnerAfterMs);
      });
      expect(isSpinnerShown()).toBe(true);
    } finally {
      vi.useRealTimers();
    }
    expect(film.plays()).toBe(0);
    expect(await shownPlayLabel()).toBe("Pause");
    expect(isSpinnerShown()).toBe(true);
    expect(await shownPlayLabel()).toBe("Pause");
    expect(mounted?.container.querySelector(".film-waits output.pill")?.textContent).toBe(
      "Getting ready to play",
    );
    film.load([[0, 12]]);
    await fire(video, "progress");
    expect(film.plays()).toBe(1);
    await fire(video, "play");
    await fire(video, "playing");
    expect(isSpinnerShown()).toBe(false);
    expect(await shownPlayLabel()).toBe("Pause");
  });

  it("paused by a press while it gets ready, stays paused once enough is loaded, showing ▶ and no spinner", async () => {
    const video = await openedFromTheFilmsPage("pressed");
    const film = fakeFilm(video);
    await fire(video, "loadstart");
    await waitMs(spinnerAfterMs + 50);
    expect(isSpinnerShown()).toBe(true);
    await pressPlay(); // it shows Pause: the press pauses it
    expect(film.plays()).toBe(0);
    expect(document.activeElement).toBe(playButton()); // a pause keeps the overlay; a play does not
    expect(isSpinnerShown()).toBe(false);
    expect(await shownPlayLabel()).toBe("Play");
    film.load([[0, 12]]);
    await fire(video, "progress");
    await waitMs(300); // the gate's own look again
    expect(film.plays()).toBe(0);
    expect(video.paused).toBe(true);
    await pressPlay(); // ▶ plays it
    expect(film.plays()).toBe(1);
    expect(document.activeElement).toBe(video);
  });

  it("paused by a remote's OK while it gets ready, shows the pause sign and stays paused", async () => {
    const video = await openedFromTheFilmsPage("ok");
    const film = fakeFilm(video);
    await fire(video, "loadstart");
    await waitMs(spinnerAfterMs + 50);
    expect(isSpinnerShown()).toBe(true);
    await pressOk(); // the overlay, hidden, takes OK as play or pause: it pauses
    const sign = mounted?.container.querySelector(".player-overlay-paused");
    expect(sign?.hasAttribute("hidden")).toBe(false);
    expect(isSpinnerShown()).toBe(false);
    film.load([[0, 12]]);
    await fire(video, "progress");
    await waitMs(300);
    expect(film.plays()).toBe(0);
  });

  it("paused by its person while playing, shows ▶ and no spinner", async () => {
    const video = await openedFromTheFilmsPage("paused");
    const film = fakeFilm(video);
    film.load([[0, 12]]);
    await fire(video, "loadstart");
    expect(film.plays()).toBe(1);
    await fire(video, "play");
    await fire(video, "playing");
    await pressPlay();
    await fire(video, "pause");
    await waitMs(spinnerAfterMs + 50);
    expect(video.paused).toBe(true);
    expect(isSpinnerShown()).toBe(false);
    expect(await shownPlayLabel()).toBe("Play");
  });

  it("whose load failed while it got ready, shows ▶ and no spinner: it waits for nothing", async () => {
    const video = await openedFromTheFilmsPage("failed");
    fakeFilm(video);
    await fire(video, "loadstart"); // the start gate waits, and its play stays pending
    expect(await shownPlayLabel()).toBe("Pause");
    Object.defineProperty(video, "error", { get: () => ({ code: 4 }) });
    await fire(video, "error");
    await waitMs(spinnerAfterMs + 50);
    expect(isSpinnerShown()).toBe(false);
    expect(await shownPlayLabel()).toBe("Play");
  });

  it("whose start the browser refused, shows ▶ and no spinner: it waits for its person's press", async () => {
    const video = await openedFromTheFilmsPage("refused");
    const film = fakeFilm(video, true);
    film.load([[0, 12]]);
    await fire(video, "loadstart");
    expect(film.plays()).toBe(1);
    await waitMs(spinnerAfterMs + 50);
    expect(video.paused).toBe(true);
    expect(isSpinnerShown()).toBe(false);
    expect(await shownPlayLabel()).toBe("Play");
  });

  it("held after a seek to where too little is loaded, shows Pause and the spinner until it plays on", async () => {
    const video = await openedFromTheFilmsPage("held");
    const film = fakeFilm(video);
    film.load([[0, 12]]);
    await fire(video, "loadstart");
    await fire(video, "play");
    await fire(video, "playing");
    video.currentTime = 3000; // the person's jump
    await fire(video, "seeking");
    expect(video.paused).toBe(true); // held before it can play there
    await fire(video, "pause");
    expect(await shownPlayLabel()).toBe("Pause");
    await waitMs(spinnerAfterMs + 50);
    expect(isSpinnerShown()).toBe(true);
    film.load([
      [0, 12],
      [2998, 3010],
    ]);
    await fire(video, "progress");
    expect(film.plays()).toBe(2);
    await fire(video, "play");
    await fire(video, "playing");
    expect(isSpinnerShown()).toBe(false);
  });

  it("stalled while playing, shows the spinner until it plays again", async () => {
    const video = await openedFromTheFilmsPage("stalled");
    const film = fakeFilm(video);
    film.load([[0, 12]]);
    await fire(video, "loadstart");
    await fire(video, "play");
    await fire(video, "playing");
    film.setReadyState(2); // ran dry
    await fire(video, "waiting");
    expect(isSpinnerShown()).toBe(false);
    await waitMs(spinnerAfterMs + 50);
    expect(isSpinnerShown()).toBe(true);
    expect(await shownPlayLabel()).toBe("Pause");
    film.setReadyState(4);
    await fire(video, "playing");
    expect(isSpinnerShown()).toBe(false);
  });
});
