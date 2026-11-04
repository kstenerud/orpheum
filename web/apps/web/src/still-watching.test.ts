// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  forgetAutoplays,
  isStillWatchingDue,
  noteAutoplayed,
  stillWatchingAfterAutoplays,
  stillWatchingOnATvAfterMs,
} from "./still-watching.tsx";

/** A Fire TV Stick 4K's Silk, which names itself by its model code. */
const fireTv =
  "Mozilla/5.0 (Linux; Android 9; AFTMM Build/PS7633) AppleWebKit/537.36 (KHTML, like Gecko) Silk/112.3.1 like Chrome/112.0.5615.213 Safari/537.36";

/** A phone's Chrome. */
const phone =
  "Mozilla/5.0 (Linux; Android 14; SM-S911B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36";

/** An episode of 11 minutes, as a children's series runs. */
const shortEpisodeMs = 11 * 60_000;

// starts-chunks-remote-2026-10-29.md §4 (S1): Netflix's rule, 3 episodes and 90 minutes on a TV.
describe("when Still watching? asks", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["performance"] });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  /** Autoplays count episodes, each shortEpisodeMs long, with no press between. */
  function autoplay(count: number): void {
    for (let each = 0; each < count; each += 1) {
      vi.advanceTimersByTime(shortEpisodeMs);
      noteAutoplayed();
    }
  }

  it("on a TV, asks nothing after three short autoplays, and asks once 90 minutes have passed with no press", () => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue(fireTv);
    forgetAutoplays();
    autoplay(stillWatchingAfterAutoplays);
    expect(isStillWatchingDue()).toBe(false);
    vi.advanceTimersByTime(stillWatchingOnATvAfterMs - 3 * shortEpisodeMs - 1);
    expect(isStillWatchingDue()).toBe(false);
    vi.advanceTimersByTime(1);
    expect(isStillWatchingDue()).toBe(true);
  });

  it("on a TV, asks nothing after 90 minutes with fewer than three autoplays", () => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue(fireTv);
    forgetAutoplays();
    autoplay(stillWatchingAfterAutoplays - 1);
    vi.advanceTimersByTime(stillWatchingOnATvAfterMs);
    expect(isStillWatchingDue()).toBe(false);
  });

  it("on a phone, asks at the third autoplay however short", () => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue(phone);
    forgetAutoplays();
    autoplay(stillWatchingAfterAutoplays - 1);
    expect(isStillWatchingDue()).toBe(false);
    autoplay(1);
    expect(isStillWatchingDue()).toBe(true);
  });

  it("on a TV, a press starts the 90 minutes again", () => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue(fireTv);
    forgetAutoplays();
    vi.advanceTimersByTime(stillWatchingOnATvAfterMs);
    forgetAutoplays();
    autoplay(stillWatchingAfterAutoplays);
    expect(isStillWatchingDue()).toBe(false);
  });
});
