// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { castFrameEveryMs } from "./cast-frame.tsx";
import { CasteePlay } from "./play-castee.tsx";
import { castingRemote } from "./test-castee.tsx";
import { buttonNamed, click, settleUntil, type Mounted } from "./test-dom.tsx";

/** Each image the remote's picture loads apart, by its address. */
const asked: string[] = [];

class FakeImage {
  onload: (() => void) | null = null;
  set src(url: string) {
    asked.push(url);
  }
}

/** The places of film 4 its key frames were asked at, in ms. */
const framesAsked = (): number[] =>
  asked
    .filter((url) => url.includes("/files/4/frame"))
    .map((url) => Number(new URL(url, "http://remote").searchParams.get("atMs")));

const tick = (): void => {
  act(() => {
    vi.advanceTimersByTime(castFrameEveryMs);
  });
};

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  asked.length = 0;
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("the cast remote's picture (D545, the rework plan, phase 3)", () => {
  it("asks for the key frame where the castee says it stands, not where a seek chosen on the remote aims, until the castee says it got there", async () => {
    vi.stubGlobal("Image", FakeImage);
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const drawn = await castingRemote({}, [], { remote: CasteePlay, onPlayHere: () => undefined });
    mounted = drawn.mounted;
    const server = drawn.sockets.session;
    const page = mounted.container;
    tick(); // the first asked as the remote opened, before the session said where the film stands
    expect(framesAsked().at(-1)).toBe(61_000);
    await click(buttonNamed(page, "Forward 10 s"));
    expect(page.textContent).toContain("1:11 / 10:00"); // the aim shown at once
    tick();
    expect(framesAsked().at(-1)).toBe(61_000); // no frame where nothing yet stands
    await settleUntil(() => server.heard.at(-1)?.["command"] === "seek");
    tick();
    expect(framesAsked().at(-1)).toBe(61_000);
    await server.say({
      type: "group",
      seq: 5,
      group: {
        state: "paused",
        positionMs: 71_000,
        rate: 1,
        fileId: 4,
        event: "seeked",
        seekedFromMs: 61_000,
      },
    });
    tick();
    expect(framesAsked().at(-1)).toBe(71_000);
  });
});
