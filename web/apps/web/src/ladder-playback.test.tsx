// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import type { ChunkedPlayback, PlaybackFailed, playChunked } from "@orpheum/player";
import { act, useRef, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { airPlayHere } from "./airplay.ts";
import { useChunkedPlayback } from "./ladder-playback.ts";
import type { LadderOffer } from "./moving-down.ts";
import { mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const played = { starts: [] as number[], destroyed: 0 };

/** The player's chunked playback, faked: it notes where each playback starts and its ending. */
const fakePlayChunked = ((_video, _url, options): ChunkedPlayback => {
  played.starts.push(options.startPositionS);
  return {
    setAudioRendition: () => undefined,
    canSwitchSoundInPlace: false,
    destroy: () => {
      played.destroyed += 1;
    },
    playing: () => undefined,
    settledAtBps: () => undefined,
    estimateKbps: () => undefined,
    beyondTheLink: () => undefined,
  };
}) satisfies typeof playChunked;

const offer: LadderOffer = {
  url: "/api/v1/bytes/media/chunks/t/master.m3u8",
  startAtBps: 1_896_000,
  qualityKbps: 1200,
  height: 480,
  isBelowTheLowest: false,
  soundCarriedGainDb: 0,
  isReady: false,
  filmPictureKbps: 9_900,
  linkSharePercent: 70,
  steadiness: 0,
};

const grant = "https://core.example/api/v1/bytes/media/g/grant/master.m3u8";

const noFailure: PlaybackFailed = () => undefined;

/** A film on its ladder from 61 s, its playback's media token given, so AirPlay is offered for it. */
function Ladder(): JSX.Element {
  const videoRef = useRef<HTMLVideoElement>(null);
  const starting = { offer, positionMs: 61_000, mediaToken: "media", play: fakePlayChunked };
  useChunkedPlayback(videoRef, starting, noFailure);
  // eslint-disable-next-line jsx-a11y-x/media-has-caption -- a stand-in for the player's element
  return <video ref={videoRef} />;
}

/** Makes video's target wireless or not, telling it as WebKit does. */
function goWireless(video: HTMLVideoElement, isWireless: boolean): void {
  Object.defineProperty(video, "webkitCurrentPlaybackTargetIsWireless", {
    value: isWireless,
    configurable: true,
  });
  video.dispatchEvent(new Event("webkitcurrentplaybacktargetiswirelesschanged"));
}

describe("a film on its ladder handed to AirPlay and back (D473)", () => {
  let mounted: Mounted | undefined;
  const pickers: HTMLMediaElement[] = [];

  beforeEach(() => {
    played.starts.length = 0;
    played.destroyed = 0;
    vi.stubGlobal("WebKitPlaybackTargetAvailabilityEvent", Event);
    Object.defineProperty(HTMLMediaElement.prototype, "webkitShowPlaybackTargetPicker", {
      value(this: HTMLMediaElement) {
        pickers.push(this);
      },
      configurable: true,
    });
  });

  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    pickers.length = 0;
    vi.unstubAllGlobals();
    Reflect.deleteProperty(HTMLMediaElement.prototype, "webkitShowPlaybackTargetPicker");
  });

  /** The ladder mounted, its video, and the requests it made. */
  async function mountLadder(): Promise<{ readonly video: HTMLVideoElement; heard: Heard[] }> {
    const heard: Heard[] = [];
    const services = servedBy({ "DELETE /airplay/grants": [200, { revoked: 1 }] }, heard, "token");
    mounted = await mount(withServices(<Ladder />, services));
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    return { video, heard };
  }

  it("lets hls.js go and gives the element the grant at its place, starting no playback while on AirPlay", async () => {
    const { video } = await mountLadder();
    expect(played.starts).toEqual([61]);
    video.currentTime = 75;
    await act(async () => {
      airPlayHere()?.start(grant);
      await Promise.resolve();
    });
    expect(played.destroyed).toBe(1);
    expect(video.getAttribute("src")).toBe(grant);
    expect(video.currentTime).toBe(75);
    expect(pickers).toEqual([video]);
    expect(airPlayHere()?.isPlaying()).toBe(true);
    expect(played.starts).toEqual([61]);
  });

  it("as its target stops being wireless, revokes the grant and hands it back to hls.js where it stood, paused as it was", async () => {
    const { video, heard } = await mountLadder();
    await act(async () => {
      airPlayHere()?.start(grant);
      await Promise.resolve();
    });
    goWireless(video, true);
    video.currentTime = 600;
    video.pause();
    const plays = vi.spyOn(video, "play");
    await act(async () => {
      goWireless(video, false);
      await Promise.resolve();
    });
    expect(heard.map((each) => each.methodAndPath)).toEqual(["DELETE /airplay/grants"]);
    expect(played.starts).toEqual([61, 600]);
    expect(video.getAttribute("src")).toBeNull();
    expect(plays).not.toHaveBeenCalled();
    expect(airPlayHere()?.isPlaying()).toBe(false);
  });

  it("at Stop, hands a film that was playing back to hls.js playing", async () => {
    const { video, heard } = await mountLadder();
    await act(async () => {
      airPlayHere()?.start(grant);
      await Promise.resolve();
    });
    goWireless(video, true);
    video.currentTime = 90;
    const plays = vi.spyOn(video, "play").mockResolvedValue(undefined);
    Object.defineProperty(video, "paused", { value: false, configurable: true });
    await act(async () => {
      airPlayHere()?.stop();
      await Promise.resolve();
    });
    expect(heard.map((each) => each.methodAndPath)).toEqual(["DELETE /airplay/grants"]);
    expect(played.starts).toEqual([61, 90]);
    expect(plays).toHaveBeenCalled();
  });
});
