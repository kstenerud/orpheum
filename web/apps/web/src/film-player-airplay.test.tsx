// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { airPlayHere } from "./airplay.ts";
import { FilmPlayer, type FilmComposition, type MovedDown } from "./film-player.tsx";
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

  async function mountFilm(
    hasControls: boolean,
    decision: ReturnType<typeof directDecision> = directDecision({ onLadder }),
    moved?: MovedDown,
  ): Promise<void> {
    const composition: FilmComposition = { hasControls, isAutoPlay: false, askAbout: undefined };
    mounted = await mount(
      withServices(
        <FilmPlayer
          decision={decision}
          moved={moved}
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

  it("says the audio track the film plays, by its place, which its grant asks AirPlay to play (D521 §2)", async () => {
    const twoTracks = directDecision({ onLadder });
    await mountFilm(true, {
      ...twoTracks,
      audioTracks: [
        { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
        { streamId: 81, audioTrackIndex: 1, language: "ja", kind: "plain" },
      ],
      startingAudioStreamId: 81,
    });
    expect(airPlayHere()?.audioTrackIndex?.()).toBe(1);
  });

  /** The original, its second audio track its start, played as it is: no ladder of its own. */
  function playedAsItIsInJapanese(): ReturnType<typeof directDecision> {
    return {
      ...directDecision(),
      audioTracks: [
        { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
        { streamId: 81, audioTrackIndex: 1, language: "ja", kind: "plain" },
      ],
      startingAudioStreamId: 81,
    };
  }

  it("says the starting audio track of a film a stalled player moved to a lower quality of, whose ladder names no sounds (D521 §2)", async () => {
    await mountFilm(true, playedAsItIsInJapanese(), {
      mediaToken: "t",
      offer: onLadder,
      positionMs: 0,
    });
    expect(airPlayHere()?.mediaToken).toBe("t");
    expect(airPlayHere()?.audioTrackIndex?.()).toBe(1);
  });

  it("says the audio track the original played as a stalled player moved, chosen of its own (D521 §2)", async () => {
    await mountFilm(true, playedAsItIsInJapanese(), {
      mediaToken: "t",
      offer: onLadder,
      positionMs: 0,
      audioTrackIndex: 0,
    });
    expect(airPlayHere()?.audioTrackIndex?.()).toBe(0);
  });

  it("is not offered for a player without controls, as a film page's hidden one", async () => {
    await mountFilm(false);
    expect(airPlayHere()).toBeUndefined();
  });
});
