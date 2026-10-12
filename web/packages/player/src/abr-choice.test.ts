// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  chunksNotHeld,
  chunksWhileStill,
  giveUpFor,
  isUnplayable,
  isWaitingToPlay,
  linkShare,
  linkShareForQuality,
  linkShareWhenAhead,
  measuresTheLink,
  startingLevel,
  standsStillAtMostMs,
  steadyLevel,
  voiceCeiling,
  type ChunkOnItsWay,
  keptOwnPicture,
  ownPictureLevel,
} from "./abr-choice.ts";

/** Bram's ladder on staging: four qualities, then its own picture, its peak 36 Mb/s. */
const levels = [
  { bitrate: 696_000, averageBitrate: 496_000 },
  { bitrate: 1_896_000, averageBitrate: 1_296_000 },
  { bitrate: 4_596_000, averageBitrate: 3_096_000 },
  { bitrate: 36_208_176, averageBitrate: 5_117_192 },
];

describe("where a playback starts (D247 §6, D260 §3)", () => {
  it("is the highest quality within the speed it was offered, by its peak", () => {
    expect(startingLevel(levels, 2_000_000)).toBe(1);
    expect(startingLevel(levels, 40_000_000)).toBe(3);
  });

  it("is the lowest when even it is beyond the speed", () => {
    expect(startingLevel(levels, 500_000)).toBe(0);
  });
});

describe("the share of the link a quality may take as it plays (D265 §4)", () => {
  it("is 70% with ten seconds or less ahead, 95% with 25 or more, and evenly between", () => {
    expect(linkShare(0)).toBe(linkShareForQuality);
    expect(linkShare(10)).toBe(linkShareForQuality);
    expect(linkShare(17.5)).toBeCloseTo((linkShareForQuality + linkShareWhenAhead) / 2);
    expect(linkShare(25)).toBe(linkShareWhenAhead);
    expect(linkShare(60)).toBe(linkShareWhenAhead);
  });
});

/** A ten-second chunk of the film's own picture on its way, nothing ahead, nothing measured. */
const pictureOnItsWay: ChunkOnItsWay = {
  levels,
  loadingLevel: 3,
  lowestLevel: 0,
  durationS: 10,
  loadingMs: 6000,
  firstByteMs: 0,
  loadedBytes: 0,
  totalBytes: 0,
  linkBps: undefined,
  bufferAheadS: 0,
};

describe("giving up a chunk on its way (D265 §3)", () => {
  it("waits for one arriving as fast as it plays, or not loading half its length yet", () => {
    // 5.2 Mb/s, faster than the picture's 5.1 average.
    expect(giveUpFor({ ...pictureOnItsWay, loadedBytes: 3_900_000 })).toBeUndefined();
    // Slow, but only four seconds of a ten-second chunk.
    expect(
      giveUpFor({ ...pictureOnItsWay, loadingMs: 4000, loadedBytes: 100_000 }),
    ).toBeUndefined();
  });

  it("gives a slow one up for the highest quality within 70% of the speed it arrives at", () => {
    // 1 Mb/s: 0.7 Mb/s carries the 360p quality only.
    expect(giveUpFor({ ...pictureOnItsWay, loadedBytes: 750_000 })).toBe(0);
    // 2 Mb/s: 1.4 Mb/s carries 480p.
    expect(giveUpFor({ ...pictureOnItsWay, loadedBytes: 1_500_000 })).toBe(1);
  });

  it("measures the speed from the chunk's first byte, not from asking for it", () => {
    // 1.5 MB over the 3 s since its first byte: 4 Mb/s, within 2.8 Mb/s of 480p, not of 720p.
    expect(giveUpFor({ ...pictureOnItsWay, firstByteMs: 3000, loadedBytes: 1_500_000 })).toBe(1);
  });

  it("gives it up for the lowest when nothing has arrived and nothing is measured, never below", () => {
    expect(giveUpFor({ ...pictureOnItsWay, loadingLevel: 2, firstByteMs: undefined })).toBe(0);
    expect(
      giveUpFor({ ...pictureOnItsWay, loadingLevel: 0, firstByteMs: undefined }),
    ).toBeUndefined();
    expect(
      giveUpFor({ ...pictureOnItsWay, lowestLevel: 1, firstByteMs: undefined, loadingLevel: 1 }),
    ).toBeUndefined();
  });

  it("gives one up for the lowest quality, which hls.js never does", () => {
    expect(
      giveUpFor({ ...pictureOnItsWay, loadingLevel: 1, loadedBytes: 375_000 }), // 0.5 Mb/s
    ).toBe(0);
  });

  it("waits for one the core is still making, on a link measured to carry it (D250 §5)", () => {
    expect(
      giveUpFor({ ...pictureOnItsWay, firstByteMs: undefined, linkBps: 20_000_000 }),
    ).toBeUndefined();
  });

  it("waits while what is loaded ahead outlasts the rest of it", () => {
    // 2 Mb/s, the rest of a 6.4 MB chunk in under 20 s, with 30 s ahead.
    expect(
      giveUpFor({ ...pictureOnItsWay, loadedBytes: 1_500_000, bufferAheadS: 30 }),
    ).toBeUndefined();
  });

  it("waits when the lower quality's whole chunk would take as long as the rest of this one", () => {
    // 4.4 Mb/s and nearly all of it here: 0.1 MB left, 0.2 s; 480p's whole chunk takes 3 s.
    expect(
      giveUpFor({
        ...pictureOnItsWay,
        loadedBytes: 3_300_000,
        totalBytes: 3_400_000,
      }),
    ).toBeUndefined();
  });
});

describe("what the next chunk is of as it plays (D265 §4)", () => {
  it("is the ceiling while nothing is measured", () => {
    expect(steadyLevel({ levels, ceiling: 2, linkBps: undefined, bufferAheadS: 0 })).toBe(2);
  });

  it("is the highest within the buffer's share of the link, never above the ceiling", () => {
    // 5 Mb/s: 3.5 with nothing ahead carries 720p; 4.75 with plenty ahead, still 720p.
    expect(steadyLevel({ levels, ceiling: 3, linkBps: 5_000_000, bufferAheadS: 0 })).toBe(2);
    // 6 Mb/s: 4.2 with nothing ahead, 720p; 5.7 with plenty, the picture.
    expect(steadyLevel({ levels, ceiling: 3, linkBps: 6_000_000, bufferAheadS: 0 })).toBe(2);
    expect(steadyLevel({ levels, ceiling: 3, linkBps: 6_000_000, bufferAheadS: 30 })).toBe(3);
    expect(steadyLevel({ levels, ceiling: 1, linkBps: 50_000_000, bufferAheadS: 30 })).toBe(1);
  });
});

describe("the film giving way to voice (D262 §2, D265 §6)", () => {
  it("is capped at the highest quality within half the link as measured", () => {
    expect(voiceCeiling(levels, 3_000_000)).toBe(1); // 1.5 Mb/s carries 480p
    expect(voiceCeiling(levels, 800_000)).toBe(0); // none within 0.4 Mb/s: the lowest
    expect(voiceCeiling(levels, undefined)).toBe(3); // nothing measured: no cap
  });
});

describe("what measures the link (D265 §7)", () => {
  const loaded = {
    loadMs: 2000,
    arrivingMs: 1900,
    bytes: 1_000_000,
    paceBps: undefined,
    isFromCache: undefined,
  };

  it("is a chunk that came over the link, not from the browser's cache", () => {
    expect(measuresTheLink(loaded)).toBe(true);
    expect(measuresTheLink({ ...loaded, loadMs: 4, arrivingMs: 1 })).toBe(false);
  });

  it("goes by what the browser says of its cache, where it says, however long the load took", () => {
    expect(measuresTheLink({ ...loaded, isFromCache: true })).toBe(false);
    // A fast link on the house's network: over the link, in under cacheHitUnderMs.
    expect(measuresTheLink({ ...loaded, loadMs: 4, arrivingMs: 1, isFromCache: false })).toBe(true);
  });

  it("is a paced chunk only when the link, not its pace, held it back", () => {
    // 4.2 Mb/s arriving: at a 4.4 Mb/s pace, the pace held it; at 6 Mb/s, the link did.
    expect(measuresTheLink({ ...loaded, paceBps: 4_400_000 })).toBe(false);
    expect(measuresTheLink({ ...loaded, paceBps: 6_000_000 })).toBe(true);
  });
});

describe("a quality this device cannot play (D266)", () => {
  it("is one whose video stood still while its chunks kept arriving", () => {
    expect(
      isUnplayable({
        forMs: standsStillAtMostMs,
        chunksLoaded: chunksWhileStill,
        chunksNotHeld: 0,
      }),
    ).toBe(true);
  });

  it("is, at once, one whose chunks the browser took and held nothing of", () => {
    expect(isUnplayable({ forMs: 1000, chunksLoaded: 2, chunksNotHeld })).toBe(true);
    expect(isUnplayable({ forMs: 1000, chunksLoaded: 2, chunksNotHeld: chunksNotHeld - 1 })).toBe(
      false,
    );
  });

  it("is not yet one that stood still briefly, or whose chunks were slow to come", () => {
    expect(
      isUnplayable({ forMs: standsStillAtMostMs - 1, chunksLoaded: 10, chunksNotHeld: 0 }),
    ).toBe(false);
    expect(
      isUnplayable({ forMs: 60_000, chunksLoaded: chunksWhileStill - 1, chunksNotHeld: 0 }),
    ).toBe(false);
  });
});

describe("a video waiting to play (D266)", () => {
  it("is one playing, or paused short of the data to play on, as an autoplaying one is", () => {
    expect(isWaitingToPlay({ paused: false, readyState: 4 })).toBe(true);
    expect(isWaitingToPlay({ paused: true, readyState: 1 })).toBe(true);
    expect(isWaitingToPlay({ paused: true, readyState: 4 })).toBe(false); // a person paused it
  });
});

describe("the film's own picture in a Watch Together group (D286 §2)", () => {
  const ladder = [
    { bitrate: 1_896_000, averageBitrate: 1_296_000, uri: "q1200k480p/stream.m3u8" },
    { bitrate: 36_208_176, averageBitrate: 5_117_192, uri: "picture/stream.m3u8" },
  ];

  it("is the quality whose playlist is picture/, kept while the link carries its average", () => {
    expect(ownPictureLevel(ladder)).toBe(1);
    expect(ownPictureLevel([ladder[0]!])).toBe(-1);
    expect(keptOwnPicture(ladder, 5_117_192)).toBe(1);
    expect(keptOwnPicture(ladder, 5_000_000)).toBeUndefined();
    expect(keptOwnPicture(ladder, undefined)).toBeUndefined();
  });
});
