// SPDX-License-Identifier: MPL-2.0

import { loadedAheadMsOf, readyAheadOf } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import {
  nextChunkBits,
  tellNextChunkLoad,
  type ChunkListing,
  type NextChunkLink,
} from "./next-chunk.ts";

/** Loaded ranges, as a video's buffered says them. */
function ranges(loaded: readonly (readonly [number, number])[]): TimeRanges {
  return {
    length: loaded.length,
    start: (index: number) => loaded[index]?.[0] ?? 0,
    end: (index: number) => loaded[index]?.[1] ?? 0,
  };
}

/** A quality of averageBps cut into chunks of 10.4 s from 0, and one byte range where given. */
function listing(averageBps: number, byteRange: readonly number[] = []): ChunkListing {
  const fragments = Array.from({ length: 200 }, (_unused, index) => ({
    start: index * 10.4,
    duration: 10.4,
    byteRange: index === 173 ? byteRange : [],
  }));
  return { loadLevel: 0, levels: [{ averageBitrate: averageBps, details: { fragments } }] };
}

/** A stand-in video: where it is, what it loaded, and its events. */
class FakeVideo extends EventTarget {
  currentTime = 1798.9;
  buffered = ranges([[1794.96, 1799.2]]);
}

/** At 1798.9 s, loaded to 1799.2 s, the end of chunk 172 of 10.4 s each. */
function marrowgateVideo(): FakeVideo {
  return new FakeVideo();
}

describe("how long the next chunk is expected to take to load (the owner, 2026-10-28)", () => {
  it("is the chunk after what is loaded, its length at its quality's average rate", () => {
    // Chunk 172 is 1788.8-1799.2 s: the next is 173, 10.4 s at 7.7 Mb/s.
    expect(nextChunkBits(listing(7_700_000), marrowgateVideo())).toBeCloseTo(10.4 * 7_700_000);
  });

  it("is its byte range's size where the playlist gives one", () => {
    expect(nextChunkBits(listing(7_700_000, [5_000_000, 15_000_000]), marrowgateVideo())).toBe(
      80_000_000,
    );
  });

  it("is unknown before the quality's playlist is loaded", () => {
    expect(nextChunkBits({ loadLevel: 0, levels: [] }, marrowgateVideo())).toBeUndefined();
  });

  it("tells the SDK its load: its first byte's wait and its bits over the link its controller names", () => {
    const video = marrowgateVideo();
    const link: { now?: NextChunkLink } = {
      now: { kbps: 40_040, firstByteMs: 0, from: "remembered" },
    };
    const stop = tellNextChunkLoad(listing(7_700_000), video, () => link.now);
    // Before any chunk, the remembered link's bound: 80 Mb at 40 Mb/s.
    expect(readyAheadOf(video)).toMatchObject({
      nextChunkLoadMs: 2000,
      nextChunkLoadFrom: "remembered",
    });
    // Measured, its bound with the wait its first byte takes.
    link.now = { kbps: 80_080, firstByteMs: 300, from: "confident" };
    expect(readyAheadOf(video)).toMatchObject({
      nextChunkLoadMs: 1300,
      nextChunkLoadFrom: "confident",
    });
    stop();
    expect(readyAheadOf(video).nextChunkLoadMs).toBeUndefined();
  });

  it("says nothing with nothing remembered and nothing measured, and the player waits refillAheadMs", () => {
    const video = marrowgateVideo();
    const stop = tellNextChunkLoad(listing(7_700_000), video, () => undefined);
    expect(readyAheadOf(video).nextChunkLoadMs).toBeUndefined();
    expect(readyAheadOf(video).nextChunkLoadFrom).toBe("none");
    stop();
  });

  it("waits on the Fire TV's shape while the link it is reckoned over is under the chunk's need", () => {
    // Staging, 2026-10-28, device 60, file 5766: 1.07 s loaded past the resume point, its next
    // chunk 173 10.4 s at 1.782 Mb/s, 18.5 Mb; at the 7.9 Mb/s the controller's bound reads from a
    // remembered 9.6 and a first chunk at 28.4 (abr-trust.test.ts), it takes 2.35 s.
    const video = new FakeVideo();
    video.currentTime = 1798.13; // 1.07 s before chunk 172 ends at 1799.2 s
    const stop = tellNextChunkLoad(listing(1_782_000), video, () => ({
      kbps: 7900,
      firstByteMs: 0,
      from: "confident",
    }));
    const ready = readyAheadOf(video);
    expect(ready.nextChunkLoadMs).toBe(2346);
    expect(loadedAheadMsOf(video)).toBe(1070);
    expect(ready.aheadMs).toBeGreaterThan(1070); // the 1.07 s it had loaded does not start it
    stop();
  });
});
