// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { CasteePlay } from "./play-castee.tsx";
import { castingRemote, heldUpright, signOf } from "./test-castee.tsx";
import { buttonNamed, click, settleUntil, type Mounted } from "./test-dom.tsx";

// The rework plan's phase 4 items 1–2 over the castee: the one Play page whatever the target.
describe("the Play page's controls over the castee, the remote's pad in every layout (the owner, 2026-11-26)", () => {
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

  for (const [layout, isUpright] of [
    ["on a computer or a device on its side", false],
    ["held upright", true],
  ] as const) {
    it(`${layout}, draws the remote's pad under the castee's key frame, working the castee, with no overlay over it and no full screen (the owner, 2026-11-26)`, async () => {
      if (isUpright) {
        vi.stubGlobal("matchMedia", heldUpright);
      }
      const { sockets } = await casting();
      const page = mounted!.container;
      const pad = page.querySelector<HTMLElement>(".castee-play__page > .remote-pad");
      expect(pad).not.toBeNull();
      expect(page.querySelector(".player-overlay")).toBeNull();
      expect(page.querySelector("video")).toBeNull();
      expect(signOf(page)).toBe("On: Fire TV");
      expect(buttonNamed(page, "Full screen")).toBeUndefined();
      await settleUntil(
        () => page.querySelector(".remote-pad__time")?.textContent === "1:01 / 10:00",
      );
      await click(buttonNamed(pad!, "Play"));
      expect(sockets.session.heard.at(-1)).toMatchObject({ type: "command", command: "play" });
      await click(buttonNamed(pad!, "Forward 10 s"));
      await settleUntil(() => sockets.session.heard.at(-1)?.["command"] === "seek");
      expect(sockets.session.heard.at(-1)).toMatchObject({ command: "seek", positionMs: 71_000 });
    });
  }
});
