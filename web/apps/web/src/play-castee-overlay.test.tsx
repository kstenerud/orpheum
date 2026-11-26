// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { CasteePlay } from "./play-castee.tsx";
import { castingRemote, heldUpright, signOf } from "./test-castee.tsx";
import { buttonNamed, click, settleUntil, type Mounted } from "./test-dom.tsx";

// The rework plan's phase 4 items 1–2 over the castee: the one Play page whatever the target.
describe("the Play page's controls over the castee, as over the video (D555 §8)", () => {
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

  it("on a computer or a device on its side, lays the video's overlay over the castee's key frame, working the castee, with no bar under it and the page's sign above", async () => {
    const { sockets } = await casting();
    const page = mounted!.container;
    const overlay = page.querySelector<HTMLElement>(".castee-play__stage .player-overlay");
    expect(overlay).not.toBeNull();
    expect(page.querySelector(".castee-play__bar")).toBeNull();
    expect(page.querySelector("video")).toBeNull();
    expect(signOf(page)).toBe("On: Fire TV");
    await settleUntil(() => overlay!.textContent.includes("1:01 / 10:00"));
    expect(buttonNamed(overlay!, "Full screen")).toBeDefined();
    await click(buttonNamed(overlay!, "Play"));
    expect(sockets.session.heard.at(-1)).toMatchObject({ type: "command", command: "play" });
    await click(buttonNamed(overlay!, "Forward 10 s"));
    await settleUntil(() => sockets.session.heard.at(-1)?.["command"] === "seek");
    expect(sockets.session.heard.at(-1)).toMatchObject({ command: "seek", positionMs: 71_000 });
  });

  it("held upright out of full screen, draws the bar under the key frame and no overlay over it", async () => {
    heldUpright();
    await casting();
    const page = mounted!.container;
    expect(page.querySelector(".castee-play__bar")).not.toBeNull();
    expect(page.querySelector(".player-overlay")).toBeNull();
  });
});
