// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  holdLoadingEvent,
  loadingRunwayEvent,
  playerLoadedChunkEvent,
  playerMovedItselfEvent,
  playerSwitchedQualityEvent,
} from "@orpheum/sdk";
import Hls from "hls.js";

import {
  hlsMethod,
  holdLoadingWhenTold,
  keepRunwayWhenTold,
  measuredKbps,
  playNative,
  preparedHlsConfig,
  startWithAudio,
  tellLoading,
  tellOwnMoves,
  type AudioStart,
  type NativeTarget,
} from "./hls.ts";

describe("playing a prepared rendition", () => {
  it("uses hls.js wherever there is MSE, the element's own HLS where there is only that", () => {
    expect(hlsMethod({ hasMse: true, canPlayNatively: true })).toBe("hls.js");
    expect(hlsMethod({ hasMse: false, canPlayNatively: true })).toBe("native");
    expect(hlsMethod({ hasMse: false, canPlayNatively: false })).toBe("none");
  });

  it("starts a rendition still being prepared at its start, not at its newest segment", () => {
    // hls.js's own default, -1, starts an open (EVENT) playlist near its live edge.
    expect(new Hls().config.startPosition).toBe(-1);
    expect(new Hls(preparedHlsConfig).config.startPosition).toBe(0);
  });

  it("hands the playlist to the element itself, and lets go of it", () => {
    const removed: string[] = [];
    let source = "";
    let loads = 0;
    const target: NativeTarget = {
      setAttribute: (name, value) => {
        source = name === "src" ? value : source;
      },
      removeAttribute: (name) => removed.push(name),
      load: () => {
        loads++;
      },
    };
    const playback = playNative(target, "/api/v1/bytes/media/hls/t/master.m3u8");
    expect(source).toBe("/api/v1/bytes/media/hls/t/master.m3u8");
    playback.destroy();
    expect(removed).toEqual(["src"]);
    expect(loads).toBe(1);
  });

  it("tells the video when hls.js moves it itself, and not for any other error", () => {
    let listener: ((event: unknown, data: { details: string }) => void) | undefined;
    tellOwnMoves(
      {
        on: (event, heard) => {
          expect(event).toBe(Hls.Events.ERROR);
          listener = heard as typeof listener;
        },
      },
      {
        dispatchEvent: (event) => {
          told.push(event.type);
          return true;
        },
      },
    );
    const told: string[] = [];
    listener?.(undefined, { details: Hls.ErrorDetails.BUFFER_SEEK_OVER_HOLE });
    listener?.(undefined, { details: Hls.ErrorDetails.FRAG_LOAD_ERROR });
    listener?.(undefined, { details: Hls.ErrorDetails.BUFFER_NUDGE_ON_STALL });
    expect(told).toEqual([playerMovedItselfEvent, playerMovedItselfEvent]);
  });

  it("tells the video each chunk hls.js loads, how fast, and each quality it switches to", () => {
    const listeners = new Map<string, ((event: unknown, data: unknown) => void)[]>();
    const fire = (event: string, data: unknown): void => {
      for (const listener of listeners.get(event) ?? []) {
        listener(undefined, data);
      }
    };
    tellLoading(
      {
        levels: [{ bitrate: 800_000 }, { bitrate: 1_200_000 }],
        bandwidthEstimate: 2_500_000,
        on: ((event: string, listener: (event: unknown, data: unknown) => void) => {
          listeners.set(event, [...(listeners.get(event) ?? []), listener]);
        }) as Hls["on"],
      },
      {
        dispatchEvent: (event) => {
          told.push({ type: event.type, detail: (event as CustomEvent).detail });
          return true;
        },
      },
    );
    const told: { type: string; detail: unknown }[] = [];
    const loaded = (type: string, level: number): void => {
      fire(Hls.Events.FRAG_LOADED, {
        frag: {
          type,
          level,
          duration: 10.2,
          stats: { loading: { start: 100, end: 2100 }, total: 500_000 },
        },
      });
    };
    // Before a chunk loaded, hls.js's estimate is its starting value, measuring nothing.
    fire(Hls.Events.LEVEL_SWITCHED, { level: 1 });
    loaded("main", 1);
    loaded("audio", 0);
    loaded("subtitle", 0); // not a chunk of film
    fire(Hls.Events.LEVEL_SWITCHED, { level: 0 });
    expect(told).toEqual([
      {
        type: playerSwitchedQualityEvent,
        detail: { qualityKbps: 1200, linkEstimateKbps: undefined },
      },
      {
        type: playerLoadedChunkEvent,
        detail: {
          track: "video",
          loadMs: 2000,
          loadedKbps: 2000,
          qualityKbps: 1200,
          filmMs: 10_200,
        },
      },
      {
        type: playerLoadedChunkEvent,
        detail: { track: "audio", loadMs: 2000, loadedKbps: 2000, qualityKbps: 0, filmMs: 10_200 },
      },
      { type: playerSwitchedQualityEvent, detail: { qualityKbps: 800, linkEstimateKbps: 2500 } },
    ]);
  });

  it("starts with the chosen mix once the master playlist is read, and leaves the default alone", () => {
    const listeners: (() => void)[] = [];
    const hls: AudioStart = {
      on: (event, listener) => {
        expect(event).toBe(Hls.Events.MANIFEST_PARSED);
        listeners.push(listener);
      },
      audioTrack: -1,
    };
    startWithAudio(hls, 1);
    expect(hls.audioTrack).toBe(-1);
    for (const listener of listeners) {
      listener();
    }
    expect(hls.audioTrack).toBe(1);
    const untouched: AudioStart = { on: () => expect.fail("nothing to wait for"), audioTrack: -1 };
    startWithAudio(untouched, 0);
    expect(untouched.audioTrack).toBe(-1);
  });
});

describe("the link's speed a player reports (D257 §1)", () => {
  it("is nothing until hls.js has loaded a chunk, whose starting estimate is not a measurement", () => {
    const listeners = new Map<string, () => void>();
    const hls = {
      bandwidthEstimate: 500_000,
      on: (event: string, listener: () => void) => {
        listeners.set(event, listener);
      },
    };
    const estimateKbps = measuredKbps(hls);
    expect(estimateKbps()).toBeUndefined();
    listeners.get(Hls.Events.FRAG_LOADED)?.();
    hls.bandwidthEstimate = 1_000_000;
    expect(estimateKbps()).toBe(1000);
  });
});

describe("holding its loading while its group waits (the owner's jumps, 2026-10-12)", () => {
  it("stops loading when told to hold, goes on from where the video is when told, and stops listening", () => {
    const calls: string[] = [];
    const hls = {
      stopLoad: () => {
        calls.push("stop");
      },
      startLoad: (position?: number) => {
        calls.push(`start ${String(position)}`);
      },
    };
    const video = new EventTarget();
    const hold = (isHeld: boolean): void => {
      video.dispatchEvent(new CustomEvent(holdLoadingEvent, { detail: { isHeld } }));
    };
    const stop = holdLoadingWhenTold(hls, video);
    hold(true);
    hold(false);
    stop();
    hold(true);
    expect(calls).toEqual(["stop", "start -1"]);
  });
});

describe("keeping a short runway while its group has one behind (D285 §2)", () => {
  it("loads no more than the runway asked, and as far as it would once told to", () => {
    const hls = { config: { maxBufferLength: 30, maxMaxBufferLength: 30 } };
    const video = new EventTarget();
    const runway = (aheadS: number | undefined): void => {
      video.dispatchEvent(new CustomEvent(loadingRunwayEvent, { detail: { aheadS } }));
    };
    const stop = keepRunwayWhenTold(hls, video, { maxBufferLength: 30, maxMaxBufferLength: 30 });
    runway(10);
    expect(hls.config).toEqual({ maxBufferLength: 10, maxMaxBufferLength: 10 });
    runway(undefined);
    expect(hls.config).toEqual({ maxBufferLength: 30, maxMaxBufferLength: 30 });
    stop();
    runway(10);
    expect(hls.config.maxBufferLength).toBe(30);
  });
});
