// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  gainAfterVolume,
  heardVolumeOf,
  ownFileGainAfterVolume,
  isWebAudioLate,
  setHeardVolume,
  soundRouteOf,
  webAudioLateAboveMs,
  type GainAudio,
  type SoundLatencyMemory,
} from "./gain.ts";
import { soundEarlierMsOf } from "./audio-shift.ts";
import { settled } from "./test-timing.ts";

/** A device's memory of its Web Audio latency, holding ms, or none. */
function remembering(ms: number | undefined): SoundLatencyMemory {
  return { recall: () => ms, keep: () => undefined };
}

/** Web Audio, faked: how many contexts were made, each running once resumed. */
function countedAudio(): { createAudioContext: () => GainAudio; contextsMade: () => number } {
  let contextsMade = 0;
  const createAudioContext = (): GainAudio => {
    contextsMade++;
    const node = { connect: () => undefined, disconnect: () => undefined };
    const faked = {
      destination: { channelCount: 2, maxChannelCount: 2 },
      state: "suspended",
      resume: () => {
        faked.state = "running";
        return Promise.resolve();
      },
      createMediaElementSource: () => node,
      createGain: () => ({ ...node, gain: { value: 1 } }),
      createDelay: () => ({ ...node, delayTime: { value: 0 } }),
    };
    return faked as unknown as GainAudio;
  };
  return { createAudioContext, contextsMade: () => contextsMade };
}

/** The TCL TV's latency through Web Audio, kept (staging, 2026-11-06). */
const televisionMs = 426;

describe("levelling where the device's Web Audio is late (D436)", () => {
  it("counts a device late only past the bound, and one never measured as not", () => {
    expect(isWebAudioLate(remembering(undefined))).toBe(false);
    expect(isWebAudioLate(remembering(12))).toBe(false);
    expect(isWebAudioLate(remembering(webAudioLateAboveMs))).toBe(false);
    expect(isWebAudioLate(remembering(televisionMs))).toBe(true);
  });

  it("lowers by the element's own volume, taking no Web Audio, and keeps the volume set", async () => {
    const video = document.createElement("video");
    const audio = countedAudio();
    setHeardVolume(video, 0.5);
    ownFileGainAfterVolume(video, audio.createAudioContext, remembering(televisionMs)).setDb(-11.6);
    await settled();
    expect(audio.contextsMade()).toBe(0);
    expect(soundRouteOf(video)).toEqual({ isThroughWebAudio: false });
    // -11.6 dB is a factor of about 0.263, under the person's half.
    expect(video.volume).toBeCloseTo(0.5 * 0.263, 3);
    expect(heardVolumeOf(video)).toBeCloseTo(0.5, 6);
    // The person's 100 is the film at the level.
    setHeardVolume(video, 1);
    expect(video.volume).toBeCloseTo(0.263, 3);
    // Levelling turned off takes the lowering back out.
    ownFileGainAfterVolume(video, audio.createAudioContext, remembering(televisionMs)).setDb(0);
    expect(video.volume).toBe(1);
    expect(heardVolumeOf(video)).toBe(1);
  });

  it("applies no raise, which the element cannot, and leaves it to the ladder's sound", async () => {
    const video = document.createElement("video");
    const audio = countedAudio();
    ownFileGainAfterVolume(video, audio.createAudioContext, remembering(televisionMs)).setDb(6);
    await settled();
    expect(audio.contextsMade()).toBe(0);
    expect(video.volume).toBe(1);
  });

  it("levels through Web Audio where its latency is under the bound", async () => {
    const video = document.createElement("video");
    const audio = countedAudio();
    ownFileGainAfterVolume(video, audio.createAudioContext, remembering(12)).setDb(-11.6);
    await settled();
    expect(audio.contextsMade()).toBe(1);
    expect(soundRouteOf(video).isThroughWebAudio).toBe(true);
    expect(video.volume).toBe(1);
  });

  it("levels a device never measured through Web Audio, once, to learn its latency", async () => {
    const video = document.createElement("video");
    const audio = countedAudio();
    ownFileGainAfterVolume(video, audio.createAudioContext, remembering(undefined)).setDb(-11.6);
    await settled();
    expect(audio.contextsMade()).toBe(1);
    expect(soundRouteOf(video).isThroughWebAudio).toBe(true);
  });

  it("keeps Web Audio for a chunked playback, whose sound's chunks are moved earlier by the latency", async () => {
    const video = document.createElement("video");
    const audio = countedAudio();
    gainAfterVolume(video, audio.createAudioContext, remembering(televisionMs)).setDb(-11.6);
    await settled();
    expect(audio.contextsMade()).toBe(1);
    expect(soundRouteOf(video).isThroughWebAudio).toBe(true);
    expect(soundEarlierMsOf(video)).toBe(televisionMs);
    expect(video.volume).toBe(1);
  });
});
