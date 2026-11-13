// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { isHandheld, pauseWhenHidden, type HiddenPage } from "./hidden.ts";

/** A page a test hides and shows, telling its listeners. */
class FakePage implements HiddenPage {
  visibilityState: DocumentVisibilityState = "visible";
  pictureInPictureElement: Element | null = null;
  readonly #listeners = new Set<() => void>();

  addEventListener(_type: "visibilitychange", listener: () => void): void {
    this.#listeners.add(listener);
  }

  removeEventListener(_type: "visibilitychange", listener: () => void): void {
    this.#listeners.delete(listener);
  }

  show(state: DocumentVisibilityState): void {
    this.visibilityState = state;
    for (const listener of this.#listeners) {
      listener();
    }
  }
}

/** A film playing, which counts its pauses. */
class FakeFilm {
  // eslint-disable-next-line @typescript-eslint/naming-convention -- HTMLMediaElement's own name
  paused = false;
  webkitPresentationMode = "inline";
  // eslint-disable-next-line @typescript-eslint/naming-convention -- WebKit's own name
  webkitCurrentPlaybackTargetIsWireless = false;
  pauses = 0;
  plays = 0;

  pause(): void {
    this.paused = true;
    this.pauses += 1;
  }

  async play(): Promise<void> {
    this.paused = false;
    this.plays += 1;
    await Promise.resolve();
  }
}

describe("a film whose page is hidden (D287)", () => {
  it("on a handheld, pauses as the page is hidden, and waits as it is shown again", () => {
    const page = new FakePage();
    const film = new FakeFilm();
    const stop = pauseWhenHidden(film, page, true);
    page.show("hidden");
    expect(film.pauses).toBe(1);
    page.show("visible");
    expect(film.paused).toBe(true);
    stop();
    film.paused = false;
    page.show("hidden");
    expect(film.pauses).toBe(1);
  });

  it("plays on while on an AirPlay receiver, and plays again as shown if the browser paused it (D473)", () => {
    const page = new FakePage();
    const film = new FakeFilm();
    film.webkitCurrentPlaybackTargetIsWireless = true;
    pauseWhenHidden(film, page, true);
    page.show("hidden");
    expect(film.pauses).toBe(0);
    film.paused = true; // iOS's own pause as Safari goes to the background
    page.show("visible");
    expect(film.plays).toBe(1);
    expect(film.paused).toBe(false);
  });

  it("does not play again as shown a film on an AirPlay receiver that was paused as it was hidden", () => {
    const page = new FakePage();
    const film = new FakeFilm();
    film.webkitCurrentPlaybackTargetIsWireless = true;
    film.paused = true;
    pauseWhenHidden(film, page, true);
    page.show("hidden");
    page.show("visible");
    expect(film.plays).toBe(0);
  });

  it("plays on in picture-in-picture, where it is still watched", () => {
    const page = new FakePage();
    const film = new FakeFilm();
    film.webkitPresentationMode = "picture-in-picture";
    pauseWhenHidden(film, page, true);
    page.show("hidden");
    expect(film.pauses).toBe(0);
  });

  it("plays on as a computer's tab is hidden, one of several open beside the film", () => {
    const page = new FakePage();
    const film = new FakeFilm();
    pauseWhenHidden(film, page, false);
    page.show("hidden");
    expect(film.pauses).toBe(0);
  });
});

describe("whether the browser is a handheld's", () => {
  const signs = (maxTouchPoints: number, isCoarse: boolean) => ({
    maxTouchPoints,
    matchMedia: (query: string) => ({ matches: query === "(pointer: coarse)" && isCoarse }),
  });

  it("is, where a finger is the main pointer and there is touch", () => {
    expect(isHandheld(signs(5, true))).toBe(true);
  });

  it("is not on a computer, with a touch screen or without", () => {
    expect(isHandheld(signs(0, false))).toBe(false);
    expect(isHandheld(signs(10, false))).toBe(false);
  });
});
