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
  type SoundForm,
  type AudioStart,
  type NativeTarget,
} from "./hls.ts";
import { skippedByOf } from "./event-context.ts";
import { ForwardBuffer, loadsAheadAtTopS } from "./forward-buffer.ts";

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
      { linkKbps: () => controller.kbps },
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

describe("keeping the sound loaded as the quality moves between audio groups (the owner, 2026-10-28)", () => {
  /** A sound in a group, as hls.js lists it. */
  interface Sound extends SoundForm {
    readonly name: string;
    readonly lang: string;
  }

  /** Whether hls.js 1.7.3 takes switching for the sound loaded (matchesOption, audioMatchPredicate). */
  function isSameSound(loaded: Sound, switching: Sound): boolean {
    const isSameCodec =
      loaded.audioCodec === undefined ||
      loaded.audioCodec.slice(0, 4) === (switching.audioCodec ?? "").slice(0, 4);
    const isSameChannels =
      loaded.channels === undefined || loaded.channels === (switching.channels ?? "2");
    return (
      loaded.name === switching.name &&
      loaded.lang === switching.lang &&
      isSameCodec &&
      isSameChannels
    );
  }

  /**
   * A fake hls.js, its sound loaded as loaded says, which switches sound as hls.js
   * 1.7.3 does (AudioTrackController.setAudioTrack, AudioStreamController.flushAudioIfNeeded): it
   * tells the switch with a copy of the track, and throws every loaded second away when the copy
   * differs from the sound loaded in name, language, channels or codec; else it loads the new sound
   * on from the end of what is loaded.
   */
  function fakeHls(loaded: { readonly fromS: number; readonly toS: number }): AudioKeeping & {
    readonly listGroup: (sounds: Sound[]) => void;
    readonly soundLoaded: () => { readonly fromS: number; readonly toS: number } | undefined;
  } {
    const listeners = new Map<string, (event: unknown, data: never) => void>();
    let group: Sound[] = [];
    let trackId = -1;
    let loadedSound: Sound | undefined;
    let soundLoaded: { readonly fromS: number; readonly toS: number } | undefined = loaded;
    const switchTo = (id: number): void => {
      const track = group[id];
      if (track === undefined) {
        return;
      }
      trackId = id;
      const switching = { ...track, id };
      if (loadedSound !== undefined && !isSameSound(loadedSound, switching)) {
        soundLoaded = undefined;
      }
      loadedSound = switching;
      listeners.get(Hls.Events.AUDIO_TRACK_SWITCHING)?.(undefined, switching as never);
    };
    return {
      on: (event: string, listener: (event: unknown, data: never) => void) => {
        listeners.set(event, listener);
      },
      get audioTrack() {
        return trackId;
      },
      set audioTrack(id: number) {
        switchTo(id);
      },
      // hls.js lists a group's tracks with no track chosen yet, then chooses the first if none is.
      listGroup: (sounds) => {
        group = sounds;
        trackId = -1;
        listeners.get(Hls.Events.AUDIO_TRACKS_UPDATED)?.(undefined, {
          audioTracks: sounds,
        } as never);
        if (trackId === -1) {
          switchTo(0);
        }
      },
      soundLoaded: () => soundLoaded,
    };
  }

  /** Nosferatu's sound at the top, its own 5.1 copied, and at the lower qualities the light stereo track. */
  function own(): Sound[] {
    return [{ name: "English", lang: "en", channels: "6" }];
  }
  function light(): Sound[] {
    return [{ name: "English", lang: "en", channels: "2" }];
  }

  it("keeps the sound loaded when its quality steps down to the light track, the new one loaded from its end", () => {
    // The Fire TV's sound loaded as its quality stepped down (staging, 2026-10-28): 37 s ahead.
    const hls = fakeHls({ fromS: 3049.13, toS: 3088.04 });
    keepAudioRendition(hls, undefined);
    hls.listGroup(own()); // the top, the film's own sound
    const lightGroup = light();
    hls.listGroup(lightGroup); // a lower quality loading
    expect(hls.audioTrack).toBe(0);
    expect(hls.soundLoaded()).toEqual({ fromS: 3049.13, toS: 3088.04 });
    expect(lightGroup[0]?.channels).toBe("2"); // its own form given back
    hls.listGroup(own()); // and back up
    expect(hls.soundLoaded()).toEqual({ fromS: 3049.13, toS: 3088.04 });
  });

  it("plays a sound the person chooses at once, the loaded one thrown away", () => {
    const hls = fakeHls({ fromS: 10, toS: 47 });
    const choose = keepAudioRendition(hls, undefined);
    hls.listGroup([...own(), { name: "English, dialogue raised", lang: "en", channels: "2" }]);
    choose(1);
    expect(hls.soundLoaded()).toBeUndefined();
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
  /** A playback loading its top quality, 3 Mb/s, which the bytes do not bound, and a runway to tell it. */
  function atTheTop(): {
    readonly hls: { readonly config: { maxBufferLength: number; maxMaxBufferLength: number } };
    readonly video: EventTarget;
    readonly ahead: ForwardBuffer;
    readonly runway: (aheadS: number | undefined) => void;
  } {
    const hls = {
      config: { maxBufferLength: 30, maxMaxBufferLength: 30 },
      levels: [
        { bitrate: 1_000_000, averageBitrate: 700_000 },
        { bitrate: 4_500_000, averageBitrate: 3_000_000 },
      ],
      media: null,
    };
    const ahead = new ForwardBuffer(hls, {
      device: { userAgent: "Mozilla/5.0 (X11; Linux x86_64) Chrome/141.0", deviceMemoryGb: 8 },
      isTop: (level) => level >= 1,
      note: () => undefined,
    });
    ahead.loads(1);
    const video = new EventTarget();
    const runway = (aheadS: number | undefined): void => {
      video.dispatchEvent(new CustomEvent(loadingRunwayEvent, { detail: { aheadS } }));
    };
    return { hls, video, ahead, runway };
  }

  it("loads no more than the runway asked, and as far as it would once told to", () => {
    const { hls, video, ahead, runway } = atTheTop();
    const stop = keepRunwayWhenTold(video, ahead);
    runway(10);
    expect(hls.config).toEqual({ maxBufferLength: 10, maxMaxBufferLength: 10 });
    runway(undefined);
    expect(hls.config).toEqual({
      maxBufferLength: loadsAheadAtTopS,
      maxMaxBufferLength: loadsAheadAtTopS,
    });
    stop();
    runway(10);
    expect(hls.config.maxBufferLength).toBe(loadsAheadAtTopS);
  });

  it("goes back to the target in force, hls.js's own cut after a buffer run full kept, not to what it was set to at the start (forward-buffer-2026-10-29.md §1.3)", () => {
    const { hls, video, ahead, runway } = atTheTop();
    keepRunwayWhenTold(video, ahead);
    hls.config.maxMaxBufferLength = 35; // hls.js's reduceMaxBufferLength, the browser's memory full
    runway(10);
    runway(undefined);
    expect(hls.config).toEqual({ maxBufferLength: 35, maxMaxBufferLength: 35 });
  });
});
