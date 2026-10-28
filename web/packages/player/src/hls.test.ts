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
  keepAudioRendition,
  startWithAudio,
  tellLoading,
  tellOwnMoves,
  type AudioKeeping,
  type AudioStart,
  type NativeTarget,
} from "./hls.ts";
import { skippedByOf } from "./event-context.ts";

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
      getAttribute: (name) => (name === "src" ? source : null),
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

  it("leaves a video the next film was given already as it is", () => {
    const removed: string[] = [];
    let source = "";
    const target: NativeTarget = {
      getAttribute: (name) => (name === "src" ? source : null),
      setAttribute: (name, value) => {
        source = name === "src" ? value : source;
      },
      removeAttribute: (name) => removed.push(name),
      load: () => undefined,
    };
    const playback = playNative(target, "/api/v1/bytes/media/hls/t/master.m3u8");
    source = "/api/v1/bytes/media/direct/next";
    playback.destroy();
    expect(removed).toEqual([]);
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
          // What it moved over goes with it, for the pause's or the seek's context (event-context.ts).
          over.push(skippedByOf(event));
          return true;
        },
      },
    );
    const told: string[] = [];
    const over: (string | undefined)[] = [];
    listener?.(undefined, { details: Hls.ErrorDetails.BUFFER_SEEK_OVER_HOLE });
    listener?.(undefined, { details: Hls.ErrorDetails.FRAG_LOAD_ERROR });
    listener?.(undefined, { details: Hls.ErrorDetails.BUFFER_NUDGE_ON_STALL });
    expect(told).toEqual([playerMovedItselfEvent, playerMovedItselfEvent]);
    expect(over).toEqual(["hole", "nudge"]);
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
      {
        type: playerSwitchedQualityEvent,
        detail: { qualityKbps: 800, linkEstimateKbps: 2500, fromQualityKbps: 1200 },
      },
    ]);
  });

  it("tells a quality begun with the estimate it is given, not hls.js's starting value, and its first as no switch", () => {
    const listeners = new Map<string, ((event: unknown, data: unknown) => void)[]>();
    const fire = (event: string, data: unknown): void => {
      for (const listener of listeners.get(event) ?? []) {
        listener(undefined, data);
      }
    };
    const told: unknown[] = [];
    const controller: { kbps: number | undefined } = { kbps: undefined };
    tellLoading(
      {
        levels: [{ bitrate: 2_000_000 }, { bitrate: 15_635_000 }],
        // hls.js's starting value, the first level's BANDWIDTH up to 5 Mb/s: the Fire TV's 5000.
        bandwidthEstimate: 5_000_000,
        on: ((event: string, listener: (event: unknown, data: unknown) => void) => {
          listeners.set(event, [...(listeners.get(event) ?? []), listener]);
        }) as Hls["on"],
      },
      {
        dispatchEvent: (event) => {
          told.push((event as CustomEvent).detail);
          return true;
        },
      },
      () => controller.kbps,
    );
    fire(Hls.Events.FRAG_LOADED, {
      frag: {
        type: "audio",
        level: 0,
        duration: 10,
        stats: { loading: { start: 0, end: 1 }, total: 1 },
      },
    });
    fire(Hls.Events.LEVEL_SWITCHED, { level: 1 });
    controller.kbps = 4200;
    fire(Hls.Events.LEVEL_SWITCHED, { level: 0 });
    expect(told.slice(1)).toEqual([
      { qualityKbps: 15_635, linkEstimateKbps: undefined },
      { qualityKbps: 2000, linkEstimateKbps: 4200, fromQualityKbps: 15_635 },
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

describe("keeping the sound playing as the quality moves between audio groups (D368 §3)", () => {
  /** A fake hls.js whose listeners the test calls, as hls.js would. */
  function fakeHls(): AudioKeeping & {
    readonly updated: (count: number) => void;
    readonly switched: (id: number) => void;
  } {
    const listeners = new Map<string, (event: unknown, data: never) => void>();
    const hls = {
      on: (event: string, listener: (event: unknown, data: never) => void) => {
        listeners.set(event, listener);
      },
      audioTrack: -1,
      // hls.js lists a group's tracks with no track chosen yet, then chooses its own if none is.
      updated: (count: number) => {
        hls.audioTrack = -1;
        listeners.get(Hls.Events.AUDIO_TRACKS_UPDATED)?.(undefined, {
          audioTracks: Array.from({ length: count }),
        } as never);
        if (hls.audioTrack === -1) {
          hls.audioTrack = 0;
        }
      },
      switched: (id: number) => {
        listeners.get(Hls.Events.AUDIO_TRACK_SWITCHED)?.(undefined, { id } as never);
      },
    };
    return hls;
  }

  it("plays the rendition at its place in the new group, not the one hls.js would match", () => {
    const hls = fakeHls();
    keepAudioRendition(hls, undefined);
    hls.updated(3);
    hls.switched(2); // the playlist's default, the starting track's
    hls.updated(3); // the top's group, its own sound copied in that place
    expect(hls.audioTrack).toBe(2);
  });

  it("keeps a rendition chosen, the boosted mix, at every quality", () => {
    const hls = fakeHls();
    const choose = keepAudioRendition(hls, undefined);
    hls.updated(3);
    choose(1);
    expect(hls.audioTrack).toBe(1);
    hls.updated(3);
    expect(hls.audioTrack).toBe(1);
  });

  it("starts with the rendition it is given, and leaves hls.js its own where the group lacks the place", () => {
    const hls = fakeHls();
    keepAudioRendition(hls, 4);
    hls.updated(5);
    expect(hls.audioTrack).toBe(4);
    hls.updated(2);
    expect(hls.audioTrack).toBe(0);
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
