// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import {
  pageLinkOutages,
  type ChunkedPlayback,
  type PlaybackFailed,
  type playChunked,
} from "@orpheum/player";
import { act, useEffect, useRef, useState, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { airPlayHere } from "./airplay-here.ts";
import { dataCapsKey } from "./data-cap.ts";
import { useChunkedPlayback } from "./ladder-playback.ts";
import type { LadderOffer } from "./moving-down.ts";
import { mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const played = {
  starts: [] as number[],
  outages: [] as unknown[],
  dataCaps: [] as ((() => number | undefined) | undefined)[],
  loadedPlaylists: [] as unknown[],
  alternatives: [] as (string | undefined)[],
  destroyed: 0,
};

/** The player's chunked playback, faked: it notes where each playback starts, its outages, its ending. */
const fakePlayChunked = ((_video, _url, options): ChunkedPlayback => {
  played.starts.push(options.startPositionS);
  played.outages.push(options.outages);
  played.dataCaps.push(options.dataCapKbps);
  played.loadedPlaylists.push(options.loadedPlaylists);
  played.alternatives.push(options.airPlayAlternativeUrl);
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
    isAheadFull: () => false,
    isAtLowestRung: () => false,
    watchLinkWait: () => ({ read: () => undefined, stop: () => undefined }),
  };
}) satisfies typeof playChunked;

const offer: LadderOffer = {
  url: "/api/v1/bytes/media/chunks/t/master.m3u8",
  startAtBps: 1_896_000,
  qualityKbps: 1200,
  height: 480,
  isBelowTheLowest: false,
  isStartHeldByNodeSpeed: false,
  soundCarriedGainDb: 0,
  isReady: false,
  filmPictureKbps: 9_900,
  linkSharePercent: 70,
  steadiness: 0,
  playlists: {
    master: { url: "/api/v1/bytes/media/chunks/t/master.m3u8", text: "#EXTM3U\n" },
    picture: { url: "/api/v1/bytes/media/chunks/t/v2/index.m3u8", text: "#EXTM3U\n" },
  },
};

const grant = "https://core.example/api/v1/bytes/media/g/grant/master.m3u8";
const soundsGrantPath = "/api/v1/bytes/media/g/sound-1/master.m3u8";

const noFailure: PlaybackFailed = () => undefined;

/** Chooses the audio track the mounted ladder plays, by its place, as the page's sound choice does. */
let chooseTrack: (track: number) => void = () => undefined;

/**
 * A film on its ladder from 61 s, its playback's media token given, so AirPlay is offered for it,
 * playing its first audio track till another is chosen (chooseTrack).
 */
function Ladder(): JSX.Element {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [playingTrack, setPlayingTrack] = useState(0);
  useEffect(() => {
    chooseTrack = setPlayingTrack;
  }, []);
  const starting = {
    offer,
    positionMs: 61_000,
    mediaToken: "media",
    play: fakePlayChunked,
    playingTrack,
  };
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

describe("a film on its ladder handed to AirPlay and back (D473, D523)", () => {
  let mounted: Mounted | undefined;
  const pickers: HTMLMediaElement[] = [];

  beforeEach(() => {
    played.starts.length = 0;
    played.outages.length = 0;
    played.dataCaps.length = 0;
    played.loadedPlaylists.length = 0;
    played.alternatives.length = 0;
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
    vi.useRealTimers();
    Reflect.deleteProperty(HTMLMediaElement.prototype, "webkitShowPlaybackTargetPicker");
  });

  /** The ladder mounted, its video, and the requests it made. */
  async function mountLadder(): Promise<{ readonly video: HTMLVideoElement; heard: Heard[] }> {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "DELETE /airplay/grants": [200, { revoked: 1 }],
        "POST /airplay/grants": [
          201,
          { masterPath: soundsGrantPath, expiresAt: "2026-11-19T12:00:00Z" },
        ],
      },
      heard,
      "token",
    );
    mounted = await mount(withServices(<Ladder />, services));
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    return { video, heard };
  }

  it("gives each playback the page's outages, so a film loads deeper after its link was out (D487)", async () => {
    await mountLadder();
    expect(played.outages).toEqual([pageLinkOutages]);
  });

  it("hands hls.js the playlists its start was answered with, so it need not ask for them (D502 §3)", async () => {
    await mountLadder();
    expect(played.loadedPlaylists).toEqual([offer.playlists]);
  });

  it("chooses each chunk under the device's data cap as it is then, chosen or lifted mid-film (D488)", async () => {
    await mountLadder();
    const capNow = played.dataCaps[0];
    expect(capNow?.()).toBeUndefined();
    window.localStorage.setItem(dataCapsKey, JSON.stringify({ unknown: 796 }));
    expect(capNow?.()).toBe(796);
    window.localStorage.removeItem(dataCapsKey);
    expect(capNow?.()).toBeUndefined();
  });

  /** The ladder mounted and AirPlay started from it, its target wireless, the settle's clock faked. */
  async function onAirPlay(): Promise<{ readonly video: HTMLVideoElement; heard: Heard[] }> {
    const mountedLadder = await mountLadder();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    await act(async () => {
      airPlayHere()?.start(grant);
      await Promise.resolve();
    });
    await act(async () => {
      goWireless(mountedLadder.video, true);
      await Promise.resolve();
    });
    return mountedLadder;
  }

  /** The ladder unmounted, as its player goes. */
  function leave(): void {
    mounted?.unmount();
    mounted = undefined;
  }

  /** The address of AirPlay's alternative offered in video, if one is. */
  function alternativeOf(video: HTMLVideoElement): string | undefined {
    return video.querySelector<HTMLSourceElement>("source[data-airplay-alternative]")?.src;
  }

  /** Lets ms pass, the page's renders done. */
  async function pass(ms: number): Promise<void> {
    await act(async () => {
      vi.advanceTimersByTime(ms);
      await Promise.resolve();
    });
  }

  it("offers the grant as the element's alternative and shows the picker, keeping hls.js's playback through AirPlay (D523 §1)", async () => {
    const { video } = await mountLadder();
    expect(played.starts).toEqual([61]);
    video.currentTime = 75;
    await act(async () => {
      airPlayHere()?.start(grant);
      await Promise.resolve();
    });
    expect(alternativeOf(video)).toBe(grant);
    expect(video.hasAttribute("src")).toBe(false);
    expect(pickers).toEqual([video]);
    expect(airPlayHere()?.isPlaying()).toBe(false);
    await act(async () => {
      goWireless(video, true);
      await Promise.resolve();
    });
    expect(airPlayHere()?.isPlaying()).toBe(true);
    expect(played.destroyed).toBe(0);
    expect(played.starts).toEqual([61]);
  });

  it("once its target has stayed off a second, plays it on hls.js where AirPlay left it, paused as it was, the alternative ready again (D523 §2, §4)", async () => {
    const { video, heard } = await onAirPlay();
    video.currentTime = 600;
    video.pause();
    const plays = vi.spyOn(video, "play");
    await act(async () => {
      goWireless(video, false);
      await Promise.resolve();
    });
    await pass(999);
    expect(played.starts).toEqual([61]);
    expect(airPlayHere()?.isPlaying()).toBe(true);
    await pass(1);
    expect(played.starts).toEqual([61, 600]);
    expect(played.destroyed).toBe(1);
    expect(played.alternatives).toEqual([undefined, grant]);
    expect(airPlayHere()?.isPlaying()).toBe(false);
    expect(plays).not.toHaveBeenCalled();
    expect(heard.map((each) => each.methodAndPath)).toEqual([]);
  });

  it("stays on AirPlay through the flag's flicker at WebKit's switch (D523 §2)", async () => {
    const { video } = await onAirPlay();
    await act(async () => {
      goWireless(video, false);
      await Promise.resolve();
    });
    await pass(130);
    await act(async () => {
      goWireless(video, true);
      await Promise.resolve();
    });
    await pass(5000);
    expect(played.starts).toEqual([61]);
    expect(airPlayHere()?.isPlaying()).toBe(true);
  });

  it("hands a film that was playing as AirPlay ended back to hls.js playing", async () => {
    const { video } = await onAirPlay();
    video.currentTime = 90;
    const plays = vi.spyOn(video, "play").mockResolvedValue(undefined);
    Object.defineProperty(video, "paused", { value: false, configurable: true });
    await act(async () => {
      goWireless(video, false); // the picker, in which the person chose this device (D523 §6)
      await Promise.resolve();
    });
    await pass(1000);
    expect(played.starts).toEqual([61, 90]);
    expect(plays).toHaveBeenCalled();
  });

  it("revokes its grants as the player goes once AirPlay was started, and none where it never was", async () => {
    const started = await onAirPlay();
    leave();
    await act(async () => {
      await Promise.resolve();
    });
    expect(started.heard.map((each) => each.methodAndPath)).toEqual(["DELETE /airplay/grants"]);
    vi.useRealTimers();
    const never = await mountLadder();
    leave();
    expect(never.heard).toEqual([]);
  });

  it("reloads another sound chosen during AirPlay on the receiver, from a grant of its own (D523 §3)", async () => {
    const { video, heard } = await onAirPlay();
    vi.spyOn(video, "load").mockImplementation(() => undefined);
    vi.useRealTimers();
    await act(async () => {
      chooseTrack(1);
      await Promise.resolve();
    });
    const soundsGrant = new URL(soundsGrantPath, globalThis.location.origin).href;
    await vi.waitFor(() => {
      expect(video.getAttribute("src")).toBe(soundsGrant);
    });
    const asked = heard.filter((each) => each.methodAndPath === "POST /airplay/grants");
    expect(asked.map((each) => each.body)).toEqual([{ mediaToken: "media", audioTrackIndex: 1 }]);
    expect(alternativeOf(video)).toBe(soundsGrant);
    expect(played.starts).toEqual([61]);
  });
});

describe("a film on its ladder whose start its node's speed held (start-quality-by-node-speed.md §4)", () => {
  let mounted: Mounted | undefined;
  const heldStarts: (boolean | undefined)[] = [];
  const notingPlay = ((video, url, options): ChunkedPlayback => {
    heldStarts.push(options.isStartHeldByNodeSpeed);
    return fakePlayChunked(video, url, options);
  }) satisfies typeof playChunked;

  function Held(props: { readonly isHeld: boolean }): JSX.Element {
    const videoRef = useRef<HTMLVideoElement>(null);
    const starting = {
      offer: { ...offer, isStartHeldByNodeSpeed: props.isHeld },
      positionMs: 0,
      shouldStartByRememberedLink: true,
      play: notingPlay,
    };
    useChunkedPlayback(videoRef, starting, noFailure);
    // eslint-disable-next-line jsx-a11y-x/media-has-caption -- a stand-in for the player's element
    return <video ref={videoRef} />;
  }

  beforeEach(() => {
    heldStarts.length = 0;
  });
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("tells its player so, which may then jump once to its link's quality", async () => {
    mounted = await mount(withServices(<Held isHeld />, servedBy({}, [], "token")));
    expect(heldStarts).toEqual([true]);
  });

  it("tells its player nothing of it where the start was not held", async () => {
    mounted = await mount(withServices(<Held isHeld={false} />, servedBy({}, [], "token")));
    expect(heldStarts).toEqual([undefined]);
  });
});
