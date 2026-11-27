// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CasteePlay } from "./play-castee.tsx";
import { castingRemote, heldUpright, receiverSays } from "./test-castee.tsx";
import { settleAnswers, settleUntil, type Mounted } from "./test-dom.tsx";

/** Presses key on what has focus, or on the page where nothing has, as a keyboard does. */
function press(key: string, extra: KeyboardEventInit = {}): void {
  const target = document.activeElement ?? document.body;
  act(() => {
    target.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true, ...extra }));
    target.dispatchEvent(new KeyboardEvent("keyup", { key, bubbles: true, ...extra }));
  });
}

/** The film's level and mute the castee is sent, as a sound message says them. */
function channel(level: number, isMuted: boolean): Record<string, unknown> {
  return { v: 3, type: "sound", sound: { kind: "channel", channel: "film", level, isMuted } };
}

// The rework plan's phase 4 item 5: the Play page's keys work the castee as its buttons do.
describe("the Play page's keys over the castee, as over the video (D555 §8)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.sessionStorage.clear();
    vi.unstubAllGlobals();
  });

  async function casting(): Promise<Awaited<ReturnType<typeof castingRemote>>> {
    const drawn = await castingRemote({}, [], { remote: CasteePlay, onPlayHere: vi.fn() });
    mounted = drawn.mounted;
    return drawn;
  }

  /** What the castee was sent of the kind type, in order. */
  function sentOf(
    server: Awaited<ReturnType<typeof casting>>["sockets"]["session"],
    type: string,
  ): Readonly<Record<string, unknown>>[] {
    return server.heard.filter((each) => each["type"] === type);
  }

  it("under the bar, plays at Space, steps 10 s at Left and Right, steps the castee's level at + and − and mutes it at M, with nothing focused", async () => {
    vi.stubGlobal("matchMedia", heldUpright);
    const { sockets } = await casting();
    const server = sockets.session;
    await receiverSays(server, {
      delivery: "direct",
      sound: {
        isLevellingOn: false,
        audioTracks: [],
        channels: [{ channel: "film", level: 0.6 }],
      },
    });
    await settleAnswers();
    press(" ");
    expect(server.heard.at(-1)).toMatchObject({ type: "command", command: "play" });
    press("ArrowRight");
    await settleUntil(() => server.heard.at(-1)?.["command"] === "seek");
    expect(server.heard.at(-1)).toMatchObject({ command: "seek", positionMs: 71_000 });
    const seeks = sentOf(server, "command").filter((each) => each["command"] === "seek").length;
    press("ArrowLeft");
    await settleUntil(
      () => sentOf(server, "command").filter((each) => each["command"] === "seek").length > seeks,
    );
    expect(server.heard.at(-1)).toMatchObject({ command: "seek", positionMs: 61_000 });
    press("+");
    expect(sentOf(server, "sound").at(-1)).toEqual(channel(0.65, false));
    press("-");
    expect(sentOf(server, "sound").at(-1)).toEqual(channel(0.6, false));
    press("m");
    expect(sentOf(server, "sound").at(-1)).toEqual(channel(0.6, true));
    // M with Ctrl, Alt or Cmd is not the player's.
    const sounds = sentOf(server, "sound").length;
    press("m", { ctrlKey: true });
    press("M", { metaKey: true });
    press("m", { altKey: true });
    expect(sentOf(server, "sound")).toHaveLength(sounds);
  });

  it("plays at Space, and puts nothing full screen at F: casting is never full screen (the owner, 2026-11-26)", async () => {
    const { sockets } = await casting();
    const page = mounted!.container;
    const requestFullscreen = vi.fn(() => Promise.resolve());
    Object.defineProperty(document.documentElement, "requestFullscreen", {
      configurable: true,
      value: requestFullscreen,
    });
    try {
      press(" ");
      expect(sockets.session.heard.at(-1)).toMatchObject({ type: "command", command: "play" });
      press("f");
      expect(requestFullscreen).not.toHaveBeenCalled();
      expect(page.querySelector(".player-overlay")).toBeNull();
    } finally {
      Reflect.deleteProperty(document.documentElement, "requestFullscreen");
    }
  });

  it("sends nothing at any key while the castee is not heard (D555 §6)", async () => {
    vi.stubGlobal("matchMedia", heldUpright);
    const { sockets } = await casting();
    const server = sockets.session;
    await receiverSays(server, {
      delivery: "direct",
      sound: {
        isLevellingOn: false,
        audioTracks: [],
        channels: [{ channel: "film", level: 0.6 }],
      },
    });
    // The receiver no longer heard, its leader gone from the cast's roster.
    await server.say({ type: "roster", roster: [] });
    await settleUntil(
      () => mounted!.container.querySelector(".castee-play__centre .pill") !== null,
    );
    const before = server.heard.length;
    for (const key of [" ", "ArrowRight", "ArrowLeft", "+", "-", "m"]) {
      press(key);
    }
    await settleAnswers();
    expect(server.heard.slice(before)).toEqual([]);
  });
});
