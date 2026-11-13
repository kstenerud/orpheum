// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  airPlayPickWithinMs,
  isAirPlayOffered,
  playOnAirPlay,
  wirelessChangedEvent,
  type AirPlayLeft,
  type AirPlayVideo,
} from "./airplay.ts";

/** An element with Safari's AirPlay, which notes what it is told and tells its target's changes. */
class FakeVideo implements AirPlayVideo {
  currentTime = 0;
  src = "";
  // eslint-disable-next-line @typescript-eslint/naming-convention -- HTMLMediaElement's own name
  disableRemotePlayback = true;
  // eslint-disable-next-line @typescript-eslint/naming-convention -- HTMLMediaElement's own name
  paused = true;
  // eslint-disable-next-line @typescript-eslint/naming-convention -- WebKit's own name
  webkitCurrentPlaybackTargetIsWireless = false;
  readonly said: string[] = [];
  readonly attributes = new Map<string, string>();
  readonly #listeners = new Map<string, Set<() => void>>();

  webkitShowPlaybackTargetPicker(): void {
    this.said.push("picker");
  }

  pause(): void {
    this.paused = true;
    this.said.push("pause");
  }

  async play(): Promise<void> {
    this.paused = false;
    this.said.push("play");
    await Promise.resolve();
  }

  load(): void {
    this.said.push(`load ${this.src}`);
  }

  setAttribute(name: string, value: string): void {
    this.attributes.set(name, value);
  }

  removeAttribute(name: string): void {
    if (name === "src") {
      this.src = "";
    }
    this.attributes.delete(name);
  }

  addEventListener(type: string, listener: () => void): void {
    const listeners = this.#listeners.get(type) ?? new Set();
    listeners.add(listener);
    this.#listeners.set(type, listeners);
  }

  removeEventListener(type: string, listener: () => void): void {
    this.#listeners.get(type)?.delete(listener);
  }

  /** Makes the target wireless or not, telling the listeners as WebKit does. */
  goWireless(isWireless: boolean): void {
    this.webkitCurrentPlaybackTargetIsWireless = isWireless;
    for (const listener of this.#listeners.get(wirelessChangedEvent) ?? []) {
      listener();
    }
  }

  listening(type: string): number {
    return this.#listeners.get(type)?.size ?? 0;
  }
}

/** A page with WebKit's target availability event, as Safari's has. */
const safari = Object.fromEntries([["WebKitPlaybackTargetAvailabilityEvent", Event]]);

const grant = "https://core.example/api/v1/bytes/media/g/grant/master.m3u8";

/** A film playing at 61 s, handed to AirPlay; what it was let go of and where it ended are noted. */
function handedOver(): {
  readonly video: FakeVideo;
  readonly stop: () => void;
  readonly ended: AirPlayLeft[];
} {
  const video = new FakeVideo();
  video.currentTime = 61;
  video.paused = false;
  const ended: AirPlayLeft[] = [];
  const playing = playOnAirPlay(video, grant, {
    letGoOfHls: () => {
      video.said.push(`hls.js let go at ${String(video.currentTime)}`);
      video.src = "";
      video.currentTime = 0;
    },
    onEnded: (left) => {
      ended.push(left);
    },
  });
  return { video, stop: playing.stop, ended };
}

describe("AirPlay from the page's own element (D473, Way B)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("is offered only where the page has WebKit's availability event and the element its picker", () => {
    const video = new FakeVideo();
    expect(isAirPlayOffered(video, safari)).toBe(true);
    expect(isAirPlayOffered(video, {})).toBe(false);
    expect(isAirPlayOffered({}, safari)).toBe(false);
  });

  it("lets hls.js go, gives the element the grant at the film's position, and shows the picker within the press", () => {
    const { video } = handedOver();
    expect(video.said).toEqual(["hls.js let go at 61", "picker", "play"]);
    expect(video.src).toBe(grant);
    expect(video.currentTime).toBe(61);
    expect(video.disableRemotePlayback).toBe(false);
    expect(video.attributes.get("x-webkit-airplay")).toBe("allow");
  });

  it("ends as the target stops being wireless, emptying the element, and says where it stood, paused or not", () => {
    const { video, ended } = handedOver();
    video.goWireless(true);
    video.currentTime = 600;
    video.paused = true;
    video.goWireless(false);
    expect(ended).toEqual([{ positionS: 600, wasPaused: true }]);
    expect(video.src).toBe("");
    expect(video.said.slice(-2)).toEqual(["pause", "load "]);
    expect(video.listening(wirelessChangedEvent)).toBe(0);
  });

  it("ends at Stop, once, playing or not", () => {
    const { video, stop, ended } = handedOver();
    video.goWireless(true);
    video.currentTime = 90;
    stop();
    stop();
    video.goWireless(false);
    expect(ended).toEqual([{ positionS: 90, wasPaused: false }]);
    expect(video.src).toBe("");
  });

  it("is handed back when no receiver is picked in time, and not while one plays", () => {
    const unpicked = handedOver();
    vi.advanceTimersByTime(airPlayPickWithinMs - 1);
    expect(unpicked.ended).toEqual([]);
    vi.advanceTimersByTime(1);
    expect(unpicked.ended).toHaveLength(1);
    expect(unpicked.video.src).toBe("");

    const picked = handedOver();
    picked.video.goWireless(true);
    vi.advanceTimersByTime(airPlayPickWithinMs * 2);
    expect(picked.ended).toEqual([]);
  });
});
