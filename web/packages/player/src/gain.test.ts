// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { canSetVolume, gainAfterVolume, type GainAudio } from "./gain.ts";
import { settled } from "./test-timing.ts";

/** A gain node, as the fake makes it. */
interface FakeGainNode {
  readonly gain: { value: number };
}

/**
 * Web Audio, faked: what was routed where, and each gain node's gain. Its context runs once resumed
 * while playing is allowed, as a browser's does once the page is played with.
 */
function fakeAudio(permission: { isGranted: boolean }): {
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
