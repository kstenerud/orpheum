// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { airPlayHere } from "./airplay.ts";
import { FilmPlayer, type FilmComposition } from "./film-player.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

const onLadder = {
  url: "/api/v1/bytes/media/chunks/t/master.m3u8",
  startAtBps: 1_896_000,
  qualityKbps: 1200,
  height: 480,
  isBelowTheLowest: false,
  soundCarriedGainDb: 0,
  isReady: true,
  filmPictureKbps: 4000,
  linkSharePercent: 70,
  steadiness: 0,
};

describe("a film on its ladder, offered to AirPlay from the Cast menu (D473)", () => {
  let mounted: Mounted | undefined;

  beforeEach(() => {
    // Safari's AirPlay, as the page finds it.
    vi.stubGlobal("WebKitPlaybackTargetAvailabilityEvent", Event);
    Object.defineProperty(HTMLMediaElement.prototype, "webkitShowPlaybackTargetPicker", {
      value: () => undefined,
      configurable: true,
    });
  });

  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.unstubAllGlobals();
    Reflect.deleteProperty(HTMLMediaElement.prototype, "webkitShowPlaybackTargetPicker");
  });

  async function mountFilm(hasControls: boolean): Promise<void> {
    const composition: FilmComposition = { hasControls, isAutoPlay: false, askAbout: undefined };
    mounted = await mount(
      withServices(
        <FilmPlayer
          decision={directDecision({ onLadder })}
          moved={undefined}
          ladderStartMs={0}
          composition={composition}
        />,
        servedBy({}),
      ),
    );
  }

  it("is offered for the person's own player, the one with controls, by its playback's media token", async () => {
    await mountFilm(true);
    expect(airPlayHere()?.mediaToken).toBe("t");
    mounted?.unmount();
    mounted = undefined;
    expect(airPlayHere()).toBeUndefined();
  });

  it("is not offered for a player without controls, as a film page's hidden one", async () => {
    await mountFilm(false);
    expect(airPlayHere()).toBeUndefined();
  });
});
