// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { forgetCastDeviceVolume, tellCastDeviceVolume } from "./cast-device-volume.ts";
import type { CastDeviceVolume } from "./cast-sdk.ts";
import { CasteePlay } from "./play-castee.tsx";
import { castingRemote, heldUpright, receiverSays } from "./test-castee.tsx";
import { buttonNamed, click, settleUntil, type Mounted } from "./test-dom.tsx";

describe("the pad's volume over a Chromecast (D557 §5)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.sessionStorage.clear();
    vi.unstubAllGlobals();
    forgetCastDeviceVolume(9);
  });

  // The phone's own Cast session took no volume once its Wi-Fi had dropped and come back, every
  // other button still working through the cast's session (the owner, 2026-11-27): a Chromecast now
  // says its device's volume as its film's channel, and the pad works it there.
  it("works a Chromecast's volume through the cast's session where it says it, not the phone's Cast session, and shows the TV remote's change it says", async () => {
    vi.stubGlobal("matchMedia", heldUpright);
    const throughThePhone: string[] = [];
    const volume: CastDeviceVolume = {
      // As the receiver first says it: the same device's volume, so no change to show.
      read: () => ({ level: 0.5, isMuted: false }),
      setLevel: (to) => throughThePhone.push(`level ${String(to)}`),
      setMuted: (isMuted) => throughThePhone.push(`muted ${String(isMuted)}`),
      onChange: () => () => undefined,
    };
    tellCastDeviceVolume(9, volume);
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
        sound: { kind: "channel", channel: "film", level: 0.2, isMuted: false },
      },
    ]);
    expect(throughThePhone).toEqual([]);
  });
});
