// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import Hls from "hls.js";
import { describe, expect, it } from "vitest";

import { offerAirPlayAlternative, wirelessChangedEvent } from "./airplay.ts";
import { letAirPlayTakeOver, stopsAsAirPlayTakesOver, type HandingOverHls } from "./airplay-hls.ts";

/** hls.js as the handover hears it: its attach and detach told by emit, its stopLoad counted. */
function fakeHls(): HandingOverHls & {
  readonly emit: (event: string) => void;
  readonly stoppedLoads: () => number;
  readonly listened: () => number;
} {
  const listeners = new Map<string, Set<() => void>>();
  let stopped = 0;
  const hls = {
    on: (event: string, listener: () => void) => {
      const each = listeners.get(event) ?? new Set();
      each.add(listener);
      listeners.set(event, each);
    },
    off: (event: string, listener: () => void) => {
      listeners.get(event)?.delete(listener);
    },
    stopLoad: () => {
      stopped += 1;
    },
    emit: (event: string) => {
      for (const listener of listeners.get(event) ?? []) {
        listener();
      }
    },
    stoppedLoads: () => stopped,
    listened: () => [...listeners.values()].reduce((count, each) => count + each.size, 0),
  };
  return hls;
}

/** A page's element whose wireless flag and current source the test sets. */
function element(): HTMLVideoElement & {
  readonly goWireless: (isWireless: boolean) => void;
  readonly playFrom: (source: string) => void;
} {
  const video = document.createElement("video");
  let isTargetWireless = false;
  let currentSource = "blob:hls";
  Object.defineProperty(video, "webkitCurrentPlaybackTargetIsWireless", {
    get: () => isTargetWireless,
  });
  Object.defineProperty(video, "currentSrc", { get: () => currentSource });
  return Object.assign(video, {
    goWireless: (isWireless: boolean) => {
      isTargetWireless = isWireless;
      video.dispatchEvent(new Event(wirelessChangedEvent));
    },
    playFrom: (source: string) => {
      currentSource = source;
    },
  });
}

const grantUrl = "https://core.example/api/v1/airplay/g1/master.m3u8";

describe("hls.js's part in WebKit's AirPlay handover (D523 §1, §4)", () => {
  it("offers the grant's alternative each time hls.js attaches, as its attach removes every source", () => {
    const hls = fakeHls();
    const video = element();
    letAirPlayTakeOver(hls, video, { alternativeUrl: grantUrl, letGo: () => undefined });
    expect(video.querySelectorAll("source")).toHaveLength(0);
    hls.emit(Hls.Events.MEDIA_ATTACHED);
    expect([...video.querySelectorAll("source")].map((source) => source.src)).toEqual([grantUrl]);
    expect(video.disableRemotePlayback).toBe(false);
  });

  it("offers none as it attaches where it was given none", () => {
    const hls = fakeHls();
    const video = element();
    letAirPlayTakeOver(hls, video, { letGo: () => undefined });
    hls.emit(Hls.Events.MEDIA_ATTACHED);
    expect(video.querySelectorAll("source")).toHaveLength(0);
  });

  it("stops hls.js loading as the target becomes wireless with an alternative offered, not without", () => {
    const hls = fakeHls();
    const video = element();
    letAirPlayTakeOver(hls, video, { letGo: () => undefined });
    video.goWireless(true);
    expect(hls.stoppedLoads()).toBe(0);
    video.goWireless(false);
    offerAirPlayAlternative(video, grantUrl);
    video.goWireless(true);
    expect(hls.stoppedLoads()).toBe(1);
  });

  it("lets go once hls.js detaches as WebKit plays the alternative, never at another detach", () => {
    const hls = fakeHls();
    const video = element();
    let letGo = 0;
    letAirPlayTakeOver(hls, video, {
      alternativeUrl: grantUrl,
      letGo: () => {
        letGo += 1;
      },
    });
    hls.emit(Hls.Events.MEDIA_ATTACHED);
    hls.emit(Hls.Events.MEDIA_DETACHED); // a recovery's, the element still on hls.js's source
    expect(letGo).toBe(0);
    video.playFrom(grantUrl);
    hls.emit(Hls.Events.MEDIA_DETACHED);
    expect(letGo).toBe(1);
  });

  it("stops listening at its end", () => {
    const hls = fakeHls();
    const video = element();
    const stop = letAirPlayTakeOver(hls, video, {
      alternativeUrl: grantUrl,
      letGo: () => undefined,
    });
    stop();
    expect(hls.listened()).toBe(0);
    offerAirPlayAlternative(video, grantUrl);
    video.goWireless(true);
    expect(hls.stoppedLoads()).toBe(0);
  });

  it("stops a playback's listeners once, as AirPlay takes the element over and again at its end", () => {
    const hls = fakeHls();
    const video = element();
    let stopped = 0;
    const stops = [
      () => {
        stopped += 1;
      },
    ];
    const stopListening = stopsAsAirPlayTakesOver(
      { hls, video },
      { airPlayAlternativeUrl: grantUrl },
      stops,
    );
    hls.emit(Hls.Events.MEDIA_ATTACHED);
    video.playFrom(grantUrl);
    hls.emit(Hls.Events.MEDIA_DETACHED);
    expect(stopped).toBe(1);
    expect(hls.listened()).toBe(0);
    stopListening();
    expect(stopped).toBe(1);
  });
});
