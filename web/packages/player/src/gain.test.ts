// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { SoundShift } from "./audio-shift.ts";
import {
  canSetVolume,
  gainAfterVolume,
  soundOffset,
  soundRouteOf,
  type GainAudio,
} from "./gain.ts";
import { settled } from "./test-timing.ts";

/** A gain node, as the fake makes it. */
interface FakeGainNode {
  readonly gain: { value: number };
}

/**
 * Web Audio, faked: what was routed where, and each gain node's gain. Its context runs once resumed
 * while playing is allowed, as a browser's does once the page is played with.
 */
function fakeAudio(
  permission: { isGranted: boolean },
  latencies: { readonly baseLatency?: number; readonly outputLatency?: number } = {},
): {
  createAudioContext: () => GainAudio;
  contextsMade: () => number;
  routed: string[];
  gainNodes: FakeGainNode[];
} {
  let contextsMade = 0;
  const routed: string[] = [];
  const gainNodes: FakeGainNode[] = [];
  const destination = { channelCount: 2, maxChannelCount: 6 };
  const createAudioContext = (): GainAudio => {
    contextsMade++;
    const faked = {
      ...latencies,
      destination,
      state: "suspended",
      resume: () => {
        if (permission.isGranted) {
          faked.state = "running";
          return Promise.resolve();
        }
        return Promise.reject(new Error("not allowed to start"));
      },
      createMediaElementSource: () => ({
        connect: () => routed.push("element → gain"),
      }),
      createGain: () => {
        const gainNode = {
          gain: { value: 1 },
          connect: (to: typeof destination) => {
            routed.push(`gain → ${String(to.channelCount)} channels`);
          },
        };
        gainNodes.push(gainNode);
        return gainNode;
      },
    };
    return faked as unknown as GainAudio;
  };
  return { createAudioContext, contextsMade: () => contextsMade, routed, gainNodes };
}

describe("a gain after the element's volume (D226 §4)", () => {
  it("leaves a sound at the level as the browser plays it, and routes one to raise once", async () => {
    const video = document.createElement("video");
    const fake = fakeAudio({ isGranted: true });
    const gain = gainAfterVolume(video, fake.createAudioContext);
    gain.setDb(0);
    expect(fake.contextsMade()).toBe(0);
    gain.setDb(12);
    await settled();
    expect(fake.routed).toEqual(["element → gain", "gain → 6 channels"]);
    expect(fake.gainNodes[0]?.gain.value).toBeCloseTo(3.981, 3);
    gain.setDb(-6);
    expect(fake.gainNodes[0]?.gain.value).toBeCloseTo(0.501, 3);
    // The element's sound is Web Audio's for its life: a second gain for it uses the same
    // gain node.
    gainAfterVolume(video, fake.createAudioContext).setDb(0);
    expect(fake.contextsMade()).toBe(1);
    expect(fake.gainNodes[0]?.gain.value).toBe(1);
  });

  it("says whether the sound goes through Web Audio, with its context's latencies (2026-10-24)", async () => {
    const video = document.createElement("video");
    expect(soundRouteOf(video)).toEqual({ isThroughWebAudio: false });
    const fake = fakeAudio({ isGranted: true }, { baseLatency: 0.0104, outputLatency: 0.48 });
    gainAfterVolume(video, fake.createAudioContext).setDb(3);
    await settled();
    expect(soundRouteOf(video)).toEqual({
      isThroughWebAudio: true,
      baseLatencyMs: 10,
      outputLatencyMs: 480,
    });
    // A browser that says no latency is told as none.
    const quiet = document.createElement("video");
    gainAfterVolume(quiet, fakeAudio({ isGranted: true }).createAudioContext).setDb(3);
    await settled();
    expect(soundRouteOf(quiet)).toEqual({ isThroughWebAudio: true });
  });

  it("leaves the sound alone until the page may play audio, so the video is never held", async () => {
    const video = document.createElement("video");
    const permission = { isGranted: false };
    const fake = fakeAudio(permission);
    gainAfterVolume(video, fake.createAudioContext).setDb(12.7);
    await settled();
    expect(fake.routed).toEqual([]);
    permission.isGranted = true;
    video.dispatchEvent(new Event("play"));
    await settled();
    expect(fake.routed).toEqual(["element → gain", "gain → 6 channels"]);
    expect(fake.gainNodes[0]?.gain.value).toBeCloseTo(4.315, 3);
  });

  it("knows a page that cannot set the volume, as on iOS, and leaves the volume as it was", () => {
    const settable = { volume: 0.8 };
    expect(canSetVolume(settable)).toBe(true);
    expect(settable.volume).toBe(0.8);
    let fixed = 1;
    const ios = {
      get volume() {
        return fixed;
      },
      set volume(_ignored: number) {
        fixed = 1;
      },
    };
    expect(canSetVolume(ios)).toBe(false);
  });

  it("applies no gain where the page cannot set the volume, as on iOS", async () => {
    const video = document.createElement("video");
    Object.defineProperty(video, "volume", { get: () => 1, set: () => undefined });
    const fake = fakeAudio({ isGranted: true });
    gainAfterVolume(video, fake.createAudioContext).setDb(12);
    await settled();
    expect(fake.contextsMade()).toBe(0);
    expect(fake.routed).toEqual([]);
  });
});

describe("a move of the sound from its picture (the owner's TCL TV, 2026-10-24)", () => {
  /** Web Audio, faked as far as a delay needs: each node made and what each connects to. */
  function delayingAudio(latencies: { baseLatency?: number; outputLatency?: number }): {
    readonly create: () => GainAudio;
    readonly delays: { delayTime: { value: number } }[];
    readonly connections: string[];
  } {
    const delays: { delayTime: { value: number } }[] = [];
    const connections: string[] = [];
    const destination = { channelCount: 2, maxChannelCount: 2 };
    const create = (): GainAudio => {
      const faked = {
        ...latencies,
        destination,
        state: "suspended",
        resume: () => {
          faked.state = "running";
          return Promise.resolve();
        },
        createMediaElementSource: () => ({
          connect: (to: { name: string }) => connections.push(`element → ${to.name}`),
          disconnect: () => connections.push("element ×"),
        }),
        createGain: () => ({
          name: "gain",
          gain: { value: 1 },
          connect: () => connections.push("gain → speakers"),
        }),
        createDelay: () => {
          const delay = {
            name: "delay",
            delayTime: { value: 0 },
            connect: (to: { name: string }) => connections.push(`delay → ${to.name}`),
          };
          delays.push(delay);
          return delay;
        },
      };
      return faked as unknown as GainAudio;
    };
    return { create, delays, connections };
  }

  it("delays the sound by what is asked less Web Audio's own latency, never below none", async () => {
    const video = document.createElement("video");
    const audio = delayingAudio({ baseLatency: 0.096, outputLatency: 0.328 });
    const delay = soundOffset(video, audio.create);
    delay.setMs(600);
    await settled();
    video.dispatchEvent(new Event("playing"));
    expect(audio.connections).toEqual([
      "element → gain",
      "gain → speakers",
      "element ×",
      "element → delay",
      "delay → gain",
    ]);
    expect(audio.delays[0]?.delayTime.value).toBeCloseTo(0.176, 3);
    delay.setMs(300);
    expect(audio.delays[0]?.delayTime.value).toBe(0);
    expect(soundRouteOf(video).isThroughWebAudio).toBe(true);
  });

  it("moves a sound wanted earlier by its chunks, and one through Web Audio earlier by its latency too", async () => {
    const plain = document.createElement("video");
    const plainShift = new SoundShift();
    plainShift.attach({ trigger: () => true }, plain);
    soundOffset(plain, delayingAudio({}).create).setMs(-450);
    await settled();
    expect(plainShift.earlierMs).toBe(450);
    expect(soundRouteOf(plain).isThroughWebAudio).toBe(false);
    // A sound through Web Audio, 424 ms late by its latency, wanted 300 ms late: moved 124 ms earlier.
    const routed = document.createElement("video");
    const routedShift = new SoundShift();
    routedShift.attach({ trigger: () => true }, routed);
    const audio = delayingAudio({ baseLatency: 0.096, outputLatency: 0.328 });
    gainAfterVolume(routed, audio.create).setDb(3);
    await settled();
    routed.dispatchEvent(new Event("playing"));
    soundOffset(routed, audio.create).setMs(300);
    expect(routedShift.earlierMs).toBe(124);
    expect(audio.delays).toEqual([]);
  });

  it("leaves a sound not delayed as the element plays it", async () => {
    const video = document.createElement("video");
    const audio = delayingAudio({});
    soundOffset(video, audio.create).setMs(0);
    await settled();
    expect(audio.connections).toEqual([]);
    expect(soundRouteOf(video).isThroughWebAudio).toBe(false);
  });
});
