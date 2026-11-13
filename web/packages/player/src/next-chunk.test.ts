// SPDX-License-Identifier: MPL-2.0

import {
  heavyChunkLoadKbps,
  loadedAheadMsOf,
  readyAheadOf,
  refillAfterStallOf,
} from "@orpheum/sdk";
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
  return {
    loadLevel: 0,
    minAutoLevel: 0,
    levels: [{ averageBitrate: averageBps, details: { fragments } }],
    config: { maxBufferLength: 30, maxMaxBufferLength: 30 },
  };
}

/** A stand-in video: where it is, what it loaded, and its events. */
class FakeVideo extends EventTarget {
  currentTime = 1798.9;
  buffered = ranges([[1794.96, 1799.2]]);
}

/** At 1798.9 s, loaded to 1799.2 s, the end of chunk 172 of 10.4 s each. */
function resumedVideo(): FakeVideo {
  return new FakeVideo();
}

describe("how long the next chunk is expected to take to load (the owner, 2026-10-28)", () => {
  it("is the chunk after what is loaded, its length at its quality's average rate", () => {
    // Chunk 172 is 1788.8-1799.2 s: the next is 173, 10.4 s at 7.7 Mb/s.
    expect(nextChunkBits(listing(7_700_000), resumedVideo())).toBeCloseTo(10.4 * 7_700_000);
  });

  it("is its byte range's size where the playlist gives one", () => {
    expect(nextChunkBits(listing(7_700_000, [5_000_000, 15_000_000]), resumedVideo())).toBe(
      80_000_000,
    );
  });

  it("is unknown before the quality's playlist is loaded", () => {
    expect(nextChunkBits({ ...listing(7_700_000), levels: [] }, resumedVideo())).toBeUndefined();
  });

  it("tells the SDK its load: its first byte's wait and its bits over the link its controller names", () => {
    const video = resumedVideo();
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

  it("tells the chunk's film, whether it is of the lowest quality, and the most hls.js loads ahead (D414 §2)", () => {
    const video = resumedVideo();
    const lowest = {
      ...listing(1_782_000),
      config: { maxBufferLength: 30, maxMaxBufferLength: 20 },
    };
    const stopLowest = tellNextChunkLoad(lowest, video, () => ({
      kbps: 1000,
      firstByteMs: 0,
      from: "confident",
    }));
    // 10.4 s at 1.782 Mb/s over 1 Mb/s: 18.5 s, the seek's rule 27.8 s and a minute's run 26.3 s;
    // at the lowest quality, no more than the 20 s hls.js loads ahead.
    expect(refillAfterStallOf(video).aheadMs).toBe(20_000);
    stopLowest();
    const above = { ...lowest, loadLevel: 1, levels: [lowest.levels[0]!, lowest.levels[0]!] };
    const stopAbove = tellNextChunkLoad(above, video, () => ({
      kbps: 1000,
      firstByteMs: 0,
      from: "confident",
    }));
    expect(refillAfterStallOf(video).aheadMs).toBe(readyAheadOf(video).aheadMs);
    stopAbove();
  });

  it("says nothing with nothing remembered and nothing measured, and the player waits refillAheadMs", () => {
    const video = resumedVideo();
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

  it("waits for a heavy next chunk by its size, however fast the link is reckoned (the 11-25 batch's item 5)", () => {
    // Staging, 2026-11-12, the testing Fire TV: it started with 3.2 s loaded, its next chunk 24.4 MB,
    // which took 8.8 s, and stood 1.3 s at 0:03. On a link reckoned at 95 Mb/s the bound's wait is
    // 3.1 s; by its size at heavyChunkLoadKbps, 9.76 s.
    const video = resumedVideo();
    const fast = (): NextChunkLink => ({ kbps: 95_000, firstByteMs: 0, from: "confident" });
    const heavy = tellNextChunkLoad(listing(7_700_000, [0, 24_400_000]), video, fast);
    expect(heavyChunkLoadKbps).toBe(20_000);
    expect(readyAheadOf(video)).toEqual({
      aheadMs: 9760,
      nextChunkLoadMs: 2055,
      nextChunkLoadFrom: "confident",
    });
    heavy();
    // A light one, 2.4 MB, waits as the bound says, its size's 0.96 s under it.
    const light = tellNextChunkLoad(listing(7_700_000, [0, 2_400_000]), video, () => ({
      kbps: 9_600,
      firstByteMs: 300,
      from: "confident",
    }));
    expect(readyAheadOf(video).aheadMs).toBe(Math.round(((300 + 19_200_000 / 9_600) * 150) / 100));
    light();
  });

  it("waits for a heavy next chunk by its size no more than refillAheadMs, nor than hls.js loads ahead", () => {
    const video = resumedVideo();
    const fast = (): NextChunkLink => ({ kbps: 95_000, firstByteMs: 0, from: "confident" });
    // 40 MB: 16 s by its size, 10 s at most, what a player knowing nothing waits.
    const stop = tellNextChunkLoad(listing(7_700_000, [0, 40_000_000]), video, fast);
    expect(readyAheadOf(video).aheadMs).toBe(10_000);
    stop();
    const shortAhead = {
      ...listing(7_700_000, [0, 40_000_000]),
      config: { maxBufferLength: 6, maxMaxBufferLength: 30 },
    };
    const stopShort = tellNextChunkLoad(shortAhead, video, fast);
    expect(readyAheadOf(video).aheadMs).toBe(6000);
    stopShort();
  });
});
