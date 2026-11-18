// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  airPlayAlternativeType,
  airPlayOffSettlesMs,
  followAirPlay,
  hasAirPlayAlternative,
  isAirPlayOffered,
  offerAirPlayAlternative,
  reloadOnAirPlay,
  showAirPlayPicker,
  wirelessChangedEvent,
} from "./airplay.ts";

/** A page's element with Safari's AirPlay: its wireless flag set by goWireless, its picker counted. */
function safarisVideo(): HTMLVideoElement & {
  readonly goWireless: (isWireless: boolean) => void;
  readonly pickersShown: () => number;
} {
  const video = document.createElement("video");
  let isTargetWireless = false;
  let shown = 0;
  Object.defineProperty(video, "webkitCurrentPlaybackTargetIsWireless", {
    get: () => isTargetWireless,
  });
  Object.defineProperty(video, "webkitShowPlaybackTargetPicker", {
    value: () => {
      shown += 1;
    },
  });
  return Object.assign(video, {
    goWireless: (isWireless: boolean) => {
      isTargetWireless = isWireless;
      video.dispatchEvent(new Event(wirelessChangedEvent));
    },
    pickersShown: () => shown,
  });
}

const grantUrl = "https://core.example/api/v1/airplay/g1/master.m3u8";

/** Safari's page, which has WebKit's event for a target's availability. */
// eslint-disable-next-line @typescript-eslint/naming-convention -- WebKit's own name
const safarisPage = { WebKitPlaybackTargetAvailabilityEvent: Event };

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("AirPlay as WebKit's own handover (D523)", () => {
  it("is offered only where the page has WebKit's availability event and the element its picker", () => {
    const video = safarisVideo();
    expect(isAirPlayOffered(video, safarisPage)).toBe(true);
    expect(isAirPlayOffered(video, {})).toBe(false);
    expect(isAirPlayOffered(document.createElement("video"), safarisPage)).toBe(false);
  });

  it("offers the grant as the element's HLS alternative, its remote playback allowed, replacing an earlier one (§1)", () => {
    const video = safarisVideo();
    video.disableRemotePlayback = true; // as hls.js leaves it, attached under Managed Media Source
    const blob = document.createElement("source");
    blob.src = "blob:hls";
    video.append(blob);
    offerAirPlayAlternative(video, `${grantUrl}?first`);
    offerAirPlayAlternative(video, grantUrl);
    const sources = [...video.querySelectorAll("source")];
    expect(sources.map((source) => source.src)).toEqual(["blob:hls", grantUrl]);
    expect(sources[1]?.type).toBe(airPlayAlternativeType);
    expect(video.disableRemotePlayback).toBe(false);
    expect(video.getAttribute("x-webkit-airplay")).toBe("allow");
    expect(hasAirPlayAlternative(video)).toBe(true);
  });

  it("shows the picker within the press, and says where it could not", () => {
    const video = safarisVideo();
    expect(showAirPlayPicker(video)).toBe(true);
    expect(video.pickersShown()).toBe(1);
    expect(showAirPlayPicker(document.createElement("video"))).toBe(false);
    const throwing = document.createElement("video");
    Object.defineProperty(throwing, "webkitShowPlaybackTargetPicker", {
      value: () => {
        throw new Error("not in a press");
      },
    });
    expect(showAirPlayPicker(throwing)).toBe(false);
  });

  it("is on as the flag rises and off only once it has stayed down a second, a flicker at the switch kept on (§2)", () => {
    const video = safarisVideo();
    const told: string[] = [];
    const stop = followAirPlay(video, {
      onOn: () => told.push("on"),
      onOff: () => told.push("off"),
    });
    video.goWireless(true);
    expect(told).toEqual(["on"]);
    // WebKit's switch: the flag down for 130 ms (the lab's E3).
    video.goWireless(false);
    vi.advanceTimersByTime(130);
    video.goWireless(true);
    vi.advanceTimersByTime(airPlayOffSettlesMs * 2);
    expect(told).toEqual(["on"]);
    video.goWireless(false);
    vi.advanceTimersByTime(airPlayOffSettlesMs - 1);
    expect(told).toEqual(["on"]);
    vi.advanceTimersByTime(1);
    expect(told).toEqual(["on", "off"]);
    stop();
    video.goWireless(true);
    expect(told).toEqual(["on", "off"]);
  });

  it("tells nothing for a fall before any rise, and nothing once stopped while settling", () => {
    const video = safarisVideo();
    const told: string[] = [];
    const stop = followAirPlay(video, {
      onOn: () => told.push("on"),
      onOff: () => told.push("off"),
    });
    video.goWireless(false);
    vi.advanceTimersByTime(airPlayOffSettlesMs);
    video.goWireless(true);
    video.goWireless(false);
    stop();
    vi.advanceTimersByTime(airPlayOffSettlesMs);
    expect(told).toEqual(["on"]);
  });

  it("reloads another sound's grant where the film stands, playing on, the alternative now the new grant (§3)", () => {
    const video = safarisVideo();
    offerAirPlayAlternative(video, `${grantUrl}?sound=0`);
    video.currentTime = 42;
    const played = vi.spyOn(video, "play").mockResolvedValue(undefined);
    Object.defineProperty(video, "paused", { value: false, configurable: true });
    const loaded = vi.spyOn(video, "load").mockImplementation(() => undefined);
    reloadOnAirPlay(video, `${grantUrl}?sound=1`);
    expect(video.getAttribute("src")).toBe(`${grantUrl}?sound=1`);
    expect(loaded).toHaveBeenCalledOnce();
    expect(played).not.toHaveBeenCalled();
    video.currentTime = 0;
    video.dispatchEvent(new Event("loadedmetadata"));
    expect(video.currentTime).toBe(42);
    expect(played).toHaveBeenCalledOnce();
    expect([...video.querySelectorAll("source")].map((source) => source.src)).toEqual([
      `${grantUrl}?sound=1`,
    ]);
  });

  it("keeps a paused film paused as it reloads its sound", () => {
    const video = safarisVideo();
    video.currentTime = 7;
    const played = vi.spyOn(video, "play").mockResolvedValue(undefined);
    vi.spyOn(video, "load").mockImplementation(() => undefined);
    reloadOnAirPlay(video, grantUrl);
    video.dispatchEvent(new Event("loadedmetadata"));
    expect(video.currentTime).toBe(7);
    expect(played).not.toHaveBeenCalled();
  });
});
