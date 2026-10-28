// SPDX-License-Identifier: MPL-2.0

import {
  loadedAheadMsOf,
  playerLoadedChunkEvent,
  readyAheadOf,
  type ChunkLoaded,
} from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import {
  nextChunkBits,
  nextChunkLinkOf,
  tellNextChunkLoad,
  type ChunkListing,
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

  it("tells the SDK its load over the link measured, or the lower of the remembered link and the first chunk's rate before that", () => {
    const video = marrowgateVideo();
    const link: { kbps?: number } = {};
    const stop = tellNextChunkLoad(listing(7_700_000), video, {
      confidentKbps: () => link.kbps,
      rememberedLinkKbps: 40_040,
    });
    // Before any chunk, the link its device last measured here: 80 Mb at 40 Mb/s.
    expect(readyAheadOf(video)).toMatchObject({
      nextChunkLoadMs: 2000,
      nextChunkLoadFrom: "remembered",
    });
    const first: ChunkLoaded = {
      track: "video",
      loadMs: 2300,
      filmMs: 10_400,
      loadedKbps: 34_800,
      qualityKbps: 7700,
    };
    video.dispatchEvent(new CustomEvent(playerLoadedChunkEvent, { detail: first }));
    expect(readyAheadOf(video).nextChunkLoadMs).toBe(2301); // 80 Mb at 34.8 Mb/s, the lower
    expect(readyAheadOf(video).nextChunkLoadFrom).toBe("firstSample");
    link.kbps = 80_080;
    expect(readyAheadOf(video).nextChunkLoadMs).toBe(1000);
    expect(readyAheadOf(video).nextChunkLoadFrom).toBe("confident");
    stop();
    expect(readyAheadOf(video).nextChunkLoadMs).toBeUndefined();
  });

  it("never rests on a lone first chunk: with nothing remembered it waits refillAheadMs until the link is confident (D352 §1)", () => {
    const video = marrowgateVideo();
    const link: { kbps?: number } = {};
    const stop = tellNextChunkLoad(listing(7_700_000), video, {
      confidentKbps: () => link.kbps,
      rememberedLinkKbps: undefined,
    });
    const first: ChunkLoaded = {
      track: "video",
      loadMs: 740,
      filmMs: 7200,
      loadedKbps: 28_384,
      qualityKbps: 7700,
    };
    video.dispatchEvent(new CustomEvent(playerLoadedChunkEvent, { detail: first }));
    expect(readyAheadOf(video).nextChunkLoadMs).toBeUndefined(); // refillAheadMs
    expect(readyAheadOf(video).nextChunkLoadFrom).toBe("none");
    link.kbps = 9636;
    expect(readyAheadOf(video).nextChunkLoadFrom).toBe("confident");
    stop();
  });

  it("waits on the Fire TV's shape: a first chunk at 28 Mb/s on a link remembered at 9.6 is reckoned at 9.6, and 1.07 s loaded does not start it", () => {
    // Staging, 2026-10-28, device 60, file 5766: its first chunk of picture loaded at 28384 kb/s,
    // 1.07 s loaded past the resume point, nextChunkLoadMs=653 from that chunk alone (980 ms to wait
    // for, which it had), and it played; its next chunk loaded at 9636 kb/s, and it ran dry a
    // second in. Here the next chunk, 173, is 10.4 s at 1.782 Mb/s: 18.5 Mb, 653 ms at 28.4 Mb/s,
    // 1923 ms at 9.6.
    const video = new FakeVideo();
    video.currentTime = 1798.13; // 1.07 s before chunk 172 ends at 1799.2 s
    const stop = tellNextChunkLoad(listing(1_782_000), video, {
      confidentKbps: () => undefined,
      rememberedLinkKbps: 9636,
    });
    video.dispatchEvent(
      new CustomEvent(playerLoadedChunkEvent, {
        detail: {
          track: "video",
          loadMs: 740,
          filmMs: 7200,
          loadedKbps: 28_384,
          qualityKbps: 1782,
        } satisfies ChunkLoaded,
      }),
    );
    const ready = readyAheadOf(video);
    expect(ready.nextChunkLoadFrom).toBe("remembered");
    expect(ready.nextChunkLoadMs).toBe(1923);
    expect(loadedAheadMsOf(video)).toBe(1070);
    expect(ready.aheadMs).toBeGreaterThan(1070); // the 1.07 s it had loaded does not start it
    expect(ready.aheadMs).toBe(2885);
    stop();
  });
});

describe("the link a next chunk's load is reckoned over", () => {
  it("is the confident estimate where there is one, whatever else is known", () => {
    expect(
      nextChunkLinkOf({ confidentKbps: 9000, rememberedKbps: 50_000, firstChunkKbps: 28_000 }),
    ).toEqual({ kbps: 9000, from: "confident" });
  });

  it("is the lower of the remembered link and the first chunk before it is confident", () => {
    expect(
      nextChunkLinkOf({ confidentKbps: undefined, rememberedKbps: 9636, firstChunkKbps: 28_384 }),
    ).toEqual({ kbps: 9636, from: "remembered" });
    expect(
      nextChunkLinkOf({ confidentKbps: undefined, rememberedKbps: 50_000, firstChunkKbps: 28_384 }),
    ).toEqual({ kbps: 28_384, from: "firstSample" });
    expect(
      nextChunkLinkOf({
        confidentKbps: undefined,
        rememberedKbps: 9636,
        firstChunkKbps: undefined,
      }),
    ).toEqual({ kbps: 9636, from: "remembered" });
  });

  it("is nothing with no estimate and nothing remembered, a first chunk alone being no measure", () => {
    expect(
      nextChunkLinkOf({
        confidentKbps: undefined,
        rememberedKbps: undefined,
        firstChunkKbps: 28_384,
      }),
    ).toBeUndefined();
  });
});
