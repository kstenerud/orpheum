// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { CasteePlay } from "./play-castee.tsx";
import { castingRemote, heldUpright, receiverSays } from "./test-castee.tsx";
import { buttonNamed, click, settleUntil, type Mounted } from "./test-dom.tsx";

describe("the pad's volume over a Chromecast (D557 §5, D563)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.sessionStorage.clear();
    vi.unstubAllGlobals();
  });

  // The phone's own Cast session took no volume once its Wi-Fi had dropped and come back, every
  // other button still working through the cast's session (the owner, 2026-11-27): a Chromecast now
  // says its device's volume as its film's channel, and the pad works it there.
  it("works a Chromecast's volume through the cast's session, and shows the TV remote's change it says", async () => {
    vi.stubGlobal("matchMedia", heldUpright);
    const drawn = await castingRemote({}, [], { remote: CasteePlay, onPlayHere: vi.fn() });
    mounted = drawn.mounted;
    const page = drawn.mounted.container;
    const server = drawn.sockets.session;
    const says = (level: number): Promise<void> =>
      receiverSays(server, {
        delivery: "direct",
        sound: { isLevellingOn: false, audioTracks: [], channels: [{ channel: "film", level }] },
      });
    await says(0.5);
    const number = (): string | undefined =>
      page.querySelector(".player-overlay-volume .pill")?.textContent ?? undefined;
    expect(number()).toBeUndefined();
    // The TV's own remote turns it down, the receiver saying so: the pad shows it, nothing pressed.
    await says(0.3);
    await settleUntil(() => number() === "Volume 30 %");
    await click(buttonNamed(page, "Volume down"));
    expect(server.heard.filter((each) => each["type"] === "sound")).toEqual([
      {
        v: 3,
        type: "sound",
        sound: { kind: "channel", channel: "film", level: 0.25, isMuted: false },
      },
    ]);
  });
  // A press here was shown again whenever the receiver came back to the volume it had said before
  // the press: 50 %, pressed down to 45 %, the TV's remote putting it back to 50 % showed 45 %, and
  // as no change at all (the owner, 2026-11-27: "it doesn't show up on the caster side").
  it("shows the volume the TV's remote puts back to what it was before a press here, not the press's", async () => {
    vi.stubGlobal("matchMedia", heldUpright);
    const drawn = await castingRemote({}, [], { remote: CasteePlay, onPlayHere: vi.fn() });
    mounted = drawn.mounted;
    const page = drawn.mounted.container;
    const says = (level: number): Promise<void> =>
      receiverSays(drawn.sockets.session, {
        delivery: "direct",
        sound: { isLevellingOn: false, audioTracks: [], channels: [{ channel: "film", level }] },
      });
    const number = (): string | undefined =>
      page.querySelector(".player-overlay-volume .pill")?.textContent ?? undefined;
    await says(0.5);
    await click(buttonNamed(page, "Volume down"));
    await settleUntil(() => number() === "Volume 45 %");
    await says(0.45);
    await says(0.5);
    await settleUntil(() => number() === "Volume 50 %");
    // And a press after it steps from where the receiver is.
    await click(buttonNamed(page, "Volume down"));
    expect(drawn.sockets.session.heard.filter((each) => each["type"] === "sound").at(-1)).toEqual({
      v: 3,
      type: "sound",
      sound: { kind: "channel", channel: "film", level: 0.45, isMuted: false },
    });
  });
});
