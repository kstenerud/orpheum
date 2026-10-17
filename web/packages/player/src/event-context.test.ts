// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it, vi } from "vitest";

import {
  eventContextOf,
  mediaSessionActionLastsMs,
  skippedByOf,
  watchMediaSession,
  type ActionSession,
  type MediaSessionAction,
} from "./event-context.ts";

const video = { readyState: 4, error: null };

describe("a pause's or a move's context", () => {
  it("says the page was hidden and unfocused, by a media key, and how ready the video was", () => {
    expect(
      eventContextOf(
        video,
        { visibilityState: "hidden", hasFocus: () => false },
        { mediaSessionAction: "pause" },
      ),
    ).toEqual({ isPageHidden: true, mediaSessionAction: "pause", readyState: 4 });
  });

  it("says a shown, focused page's pause was the person's own gesture, and what hls.js skipped", () => {
    expect(
      eventContextOf(
        video,
        { visibilityState: "visible", hasFocus: () => true },
        { isUserActive: true, skippedBy: "hole" },
      ),
    ).toEqual({ hasFocus: true, isUserActive: true, skippedBy: "hole", readyState: 4 });
  });

  it("reads what hls.js moved the video over from its event, and nothing from any other", () => {
    expect(skippedByOf(new CustomEvent("moved", { detail: "nudge" }))).toBe("nudge");
    expect(skippedByOf(new CustomEvent("moved", { detail: "hole" }))).toBe("hole");
    expect(skippedByOf(new CustomEvent("moved", { detail: "elsewhere" }))).toBeUndefined();
    expect(skippedByOf(new Event("moved"))).toBeUndefined();
  });
});

describe("the media session", () => {
  function sessionOf(): ActionSession & {
    readonly handlers: Map<MediaSessionAction, (() => void) | null>;
  } {
    const handlers = new Map<MediaSessionAction, (() => void) | null>();
    return {
      handlers,
      setActionHandler: (action, handler) => {
        handlers.set(action, handler);
      },
    };
  }

  it("does what the browser would, and remembers the action for a second", () => {
    const session = sessionOf();
    const media = { play: vi.fn(() => Promise.resolve()), pause: vi.fn() };
    let now = 1000;
    const watched = watchMediaSession(session, media, () => now);
    expect(watched.lastAction()).toBeUndefined();
    session.handlers.get("pause")?.();
    expect(media.pause).toHaveBeenCalledOnce();
    expect(watched.lastAction()).toBe("pause");
    session.handlers.get("play")?.();
    expect(media.play).toHaveBeenCalledOnce();
    expect(watched.lastAction()).toBe("play");
    now += mediaSessionActionLastsMs + 1;
    expect(watched.lastAction()).toBeUndefined();
  });

  it("gives the browser its own answers back once stopped, and asks nothing of a browser with none", () => {
    const session = sessionOf();
    const watched = watchMediaSession(session, { play: vi.fn(), pause: vi.fn() }, () => 0);
    watched.stop();
    expect([...session.handlers.values()]).toEqual([null, null]);
    const unsupported: ActionSession = {
      setActionHandler: () => {
        throw new TypeError("no such action");
      },
    };
    expect(() => {
      watchMediaSession(unsupported, { play: vi.fn(), pause: vi.fn() }, () => 0).stop();
    }).not.toThrow();
    expect(
      watchMediaSession(undefined, { play: vi.fn(), pause: vi.fn() }, () => 0).lastAction(),
    ).toBeUndefined();
  });
});
