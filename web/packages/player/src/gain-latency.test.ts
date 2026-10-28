// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { playerMovedSoundEvent, type SoundMoved } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { SoundShift } from "./audio-shift.ts";
import { gainAfterVolume, soundLatencyChangeIgnoredMs, type GainAudio } from "./gain.ts";
import { settled } from "./test-timing.ts";

/**
 * Web Audio, faked as far as a gain and its latencies need: a context that runs once resumed where
 * playing is allowed, reporting the latencies set on it, in seconds, as they change.
 */
function latencyAudio(
  latencies: { baseLatency: number; outputLatency: number },
  isGranted = true,
): () => GainAudio {
  return () => {
    const faked = {
      get baseLatency() {
        return latencies.baseLatency;
      },
      get outputLatency() {
        return latencies.outputLatency;
      },
      destination: { channelCount: 2, maxChannelCount: 2 },
      state: "suspended",
      resume: () => {
        if (!isGranted) {
          return Promise.reject(new Error("not allowed to start"));
        }
        faked.state = "running";
        return Promise.resolve();
      },
      createMediaElementSource: () => ({ connect: () => undefined }),
      createGain: () => ({ gain: { value: 1 }, connect: () => undefined }),
    };
    return faked as unknown as GainAudio;
  };
}

/** A device's memory of its sound's latency, faked: what it holds, and each latency kept. */
function fakeMemory(heldMs: number | undefined): {
  readonly memory: { recall: () => number | undefined; keep: (ms: number) => void };
  readonly kept: number[];
} {
  const kept: number[] = [];
  let held = heldMs;
  return {
    memory: {
      recall: () => held,
      keep: (ms) => {
        held = ms;
        kept.push(ms);
      },
    },
    kept,
  };
}

/**
 * A chunked playback's shift attached to video, with each time it threw the sound's buffer away, and
 * each move video told of (playerMovedSoundEvent).
 */
function attachedShift(video: HTMLVideoElement): {
  readonly shift: SoundShift;
  readonly flushes: () => number;
  readonly moves: SoundMoved[];
} {
  let flushes = 0;
  const moves: SoundMoved[] = [];
  video.addEventListener(playerMovedSoundEvent, (event) => {
    moves.push((event as CustomEvent<SoundMoved>).detail);
  });
  const shift = new SoundShift();
  shift.attach(
    {
      trigger: () => {
        flushes += 1;
        return true;
      },
    },
    video,
  );
  return { shift, flushes: () => flushes, moves };
}

describe("the sound's move for Web Audio's latency, as the device remembers it (a Fire TV, 2026-10-28)", () => {
  it("moves the sound by the latency remembered before its first chunk, and its start throws nothing away", async () => {
    const video = document.createElement("video");
    const moves: SoundMoved[] = [];
    video.addEventListener(playerMovedSoundEvent, (event) => {
      moves.push((event as CustomEvent<SoundMoved>).detail);
    });
    // The context reports no output latency until its sound plays, as none has been measured.
    const latencies = { baseLatency: 0.043, outputLatency: 0 };
    const { memory, kept } = fakeMemory(331);
    gainAfterVolume(video, latencyAudio(latencies), memory).setDb(3);
    // The playback made after starts where the sound is wanted: its first chunk already moved.
    const playback = attachedShift(video);
    expect(playback.shift.earlierMs).toBe(331);
    await settled();
    expect(playback.shift.earlierMs).toBe(331);
    // Measured as it plays, a few ms off what was remembered, and again after: nothing moves.
    latencies.outputLatency = 0.288;
    video.dispatchEvent(new Event("playing"));
    latencies.outputLatency = 0.304;
    video.dispatchEvent(new Event("playing"));
    expect(playback.shift.earlierMs).toBe(331);
    expect(playback.flushes()).toBe(0);
    expect(playback.moves).toEqual([]);
    expect(kept).toEqual([]);
    expect(moves).toEqual([
      { earlierFromMs: 0, earlierMs: 331, why: "remembered", isFlushed: false, rememberedMs: 331 },
    ]);
  });

  it("reads no latency as the context starts, before the film plays: the start keeps the move remembered (the Fire TV's casts, 07:47 to 07:51)", async () => {
    const video = document.createElement("video");
    // As it starts, the context reports a latency far short of its own: 83 ms on the Fire TV.
    const latencies = { baseLatency: 0.043, outputLatency: 0.04 };
    const { memory, kept } = fakeMemory(339);
    gainAfterVolume(video, latencyAudio(latencies), memory).setDb(3);
    const playback = attachedShift(video);
    await settled();
    expect(playback.shift.earlierMs).toBe(339);
    // As the film plays, it reports its own, within the threshold of what was remembered.
    latencies.outputLatency = 0.296;
    video.dispatchEvent(new Event("playing"));
    expect(playback.shift.earlierMs).toBe(339);
    expect(playback.flushes()).toBe(0);
    expect(kept).toEqual([]);
    expect(playback.moves).toEqual([]);
  });

  it("reads the latency at once where the sound is routed as the film already plays", async () => {
    const video = document.createElement("video");
    Object.defineProperty(video, "paused", { get: () => false });
    Object.defineProperty(video, "readyState", { get: () => 4 });
    const playback = attachedShift(video);
    const { memory, kept } = fakeMemory(undefined);
    gainAfterVolume(
      video,
      latencyAudio({ baseLatency: 0.043, outputLatency: 0.288 }),
      memory,
    ).setDb(3);
    await settled();
    expect(playback.shift.earlierMs).toBe(331);
    expect(kept).toEqual([331]);
  });

  it("leaves the move as it is for a change of the latency under the threshold", async () => {
    const video = document.createElement("video");
    const playback = attachedShift(video);
    const latencies = { baseLatency: 0.043, outputLatency: 0.288 };
    const { memory, kept } = fakeMemory(undefined);
    gainAfterVolume(video, latencyAudio(latencies), memory).setDb(3);
    await settled();
    // A device's first playback learns its latency as it plays: one move, which throws its sound away.
    video.dispatchEvent(new Event("playing"));
    expect(playback.shift.earlierMs).toBe(331);
    expect(playback.flushes()).toBe(1);
    expect(kept).toEqual([331]);
    latencies.outputLatency = 0.288 + (soundLatencyChangeIgnoredMs - 1) / 1000;
    video.dispatchEvent(new Event("playing"));
    latencies.outputLatency = 0.288 - (soundLatencyChangeIgnoredMs - 1) / 1000;
    video.dispatchEvent(new Event("playing"));
    expect(playback.shift.earlierMs).toBe(331);
    expect(playback.flushes()).toBe(1);
    expect(kept).toEqual([331]);
  });

  it("moves the sound once for a larger change, keeps the latency, and tells what moved it", async () => {
    const video = document.createElement("video");
    const latencies = { baseLatency: 0.043, outputLatency: 0.377 };
    const { memory, kept } = fakeMemory(331);
    gainAfterVolume(video, latencyAudio(latencies), memory).setDb(3);
    const playback = attachedShift(video);
    await settled();
    video.dispatchEvent(new Event("playing"));
    expect(playback.shift.earlierMs).toBe(420);
    expect(playback.flushes()).toBe(1);
    expect(kept).toEqual([420]);
    expect(playback.moves).toEqual([
      {
        earlierFromMs: 331,
        earlierMs: 420,
        why: "measured",
        latencyMs: 420,
        isFlushed: true,
        fromWhy: "remembered",
        rememberedMs: 331,
      },
    ]);
    // The next playback on the device starts at what was kept.
    const next = document.createElement("video");
    gainAfterVolume(next, latencyAudio(latencies), memory).setDb(3);
    expect(attachedShift(next).shift.earlierMs).toBe(420);
  });

  it("takes the remembered move back where the sound plays with no Web Audio after all", async () => {
    const video = document.createElement("video");
    const { memory } = fakeMemory(331);
    gainAfterVolume(
      video,
      latencyAudio({ baseLatency: 0.043, outputLatency: 0.288 }, false),
      memory,
    ).setDb(3);
    const playback = attachedShift(video);
    await settled();
    expect(playback.shift.earlierMs).toBe(331);
    video.dispatchEvent(new Event("playing"));
    expect(playback.shift.earlierMs).toBe(0);
    expect(playback.moves).toEqual([
      {
        earlierFromMs: 331,
        earlierMs: 0,
        why: "direct",
        isFlushed: true,
        fromWhy: "remembered",
        rememberedMs: 331,
      },
    ]);
  });
});
