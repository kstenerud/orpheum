// SPDX-License-Identifier: MPL-2.0

import { playerLoadedChunkEvent, readyAheadOf, type ChunkLoaded } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { nextChunkBits, tellNextChunkLoad, type ChunkListing } from "./next-chunk.ts";

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

  it("tells the SDK its load over the link measured, or the first chunk's rate before that", () => {
    const video = marrowgateVideo();
    const link: { kbps?: number } = {};
    const stop = tellNextChunkLoad(listing(7_700_000), video, () => link.kbps);
    expect(readyAheadOf(video).nextChunkLoadMs).toBeUndefined(); // nothing measured: refillAheadMs
    const first: ChunkLoaded = {
      track: "video",
      loadMs: 2300,
      filmMs: 10_400,
      loadedKbps: 34_800,
      qualityKbps: 7700,
    };
    video.dispatchEvent(new CustomEvent(playerLoadedChunkEvent, { detail: first }));
    expect(readyAheadOf(video).nextChunkLoadMs).toBe(2301); // 80 Mb at 34.8 Mb/s
    link.kbps = 80_080;
    expect(readyAheadOf(video).nextChunkLoadMs).toBe(1000);
    stop();
    expect(readyAheadOf(video).nextChunkLoadMs).toBeUndefined();
  });
});
