// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { healPageAudio, keepPageAudio, setPageAudio } from "./page-audio.ts";

/** Safari's audio session as the page sets it, every type it was given in order. */
let session: { type: string; given: string[] };

beforeEach(() => {
  vi.useFakeTimers();
  const given: string[] = [];
  session = {
    given,
    get type(): string {
      return given.at(-1) ?? "auto";
    },
    set type(type: string) {
      given.push(type);
    },
  };
  vi.stubGlobal("navigator", { audioSession: session });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

/** A page whose visibility a test sets, telling its listeners. */
class FakePage {
  visibilityState: DocumentVisibilityState = "visible";
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

describe("the page's audio session (WebKit bug 323104)", () => {
  it("plays as a video app's from the start, and heals as the page is shown again", () => {
    const page = new FakePage();
    const stop = keepPageAudio(page);
    expect(session.given).toEqual(["playback"]);
    page.show("hidden");
    expect(session.given).toEqual(["playback"]);
    page.show("visible");
    expect(session.given).toEqual(["playback", "ambient"]);
    vi.advanceTimersByTime(0);
    expect(session.given).toEqual(["playback", "ambient", "playback"]);
    stop();
    page.show("visible");
    expect(session.given).toHaveLength(3);
  });

  it("heals to what the page means: a call's while a microphone is open", () => {
    setPageAudio("auto");
    healPageAudio();
    vi.advanceTimersByTime(0);
    expect(session.given).toEqual(["auto", "ambient", "auto"]);
    setPageAudio("playback");
  });
});
