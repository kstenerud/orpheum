// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import Hls, { Events, Fragment, PlaylistLevelType } from "hls.js";

import { orpheumAbr } from "./abr-controller.ts";
import {
  bufferRoomOf,
  chunkBytesAt,
  limitMemoryIn,
  pairLevel,
  PictureRoom,
  roomAheadS,
  StepDown,
  type GridChunk,
  type HeldSpan,
  type LimitStorage,
  type RoomHls,
  type RoomLevel,
} from "./picture-room.ts";

/** A grid of ten 6 s chunks, each saying bitsPerS for its own, 4 Mb/s where it says nothing. */
function gridOf(bitsPerS: Readonly<Record<number, number>> = {}): readonly GridChunk[] {
  return Array.from({ length: 10 }, (_, sn) => ({
    sn,
    start: sn * 6,
    duration: 6,
    bitrate: bitsPerS[sn] ?? 4_000_000,
  }));
}

/**
 * The test ladder's made qualities, and the film's own picture at its top whose chunk 5 is a heavy
 * 20 Mb/s, 15 MB, its others 3 MB.
 */
function ladder(): readonly RoomLevel[] {
  return [
    { bitrate: 696_000, averageBitrate: 496_000 },
    { bitrate: 1_896_000, averageBitrate: 1_296_000 },
    { bitrate: 4_596_000, averageBitrate: 3_096_000 },
    {
      bitrate: 36_208_176,
      averageBitrate: 5_117_192,
      uri: "picture/stream.m3u8",
      details: { fragments: gridOf({ 5: 20_000_000 }) },
    },
  ];
}

/** Chunks 2 to 4 loaded, 12 s to 30 s, chunk 5 the next. */
const loadedTo30: HeldSpan = { start: 12, end: 30 };

/** A RoomHls with the ladder, loaded as span says. */
function hlsAt(span: HeldSpan | null = loadedTo30): RoomHls {
  return { levels: ladder(), minAutoLevel: 0, mainForwardBufferInfo: span };
}

/** A storage of its own, which may refuse every use. */
function storageOf(isRefused = false): LimitStorage & { readonly kept: Map<string, string> } {
  const kept = new Map<string, string>();
  return {
    kept,
    getItem: (key) => {
      if (isRefused) {
        throw new Error("SecurityError");
      }
      return kept.get(key) ?? null;
    },
    setItem: (key, value) => {
      if (isRefused) {
        throw new Error("QuotaExceededError");
      }
      kept.set(key, value);
    },
  };
}

const silk = "Mozilla/5.0 (Linux; Android 9; AFTSSS Build/PS7633) Silk/138";

/** A room of 30 MB, 27 MB of it for the chunks, holding chunks 2 to 4 at megabytes each, and its notes. */
function roomHolding(
  megabytes: readonly [number, number, number],
  options: { readonly storage?: LimitStorage; readonly userAgent?: string } = {},
): { readonly room: PictureRoom; readonly notes: string[] } {
  const notes: string[] = [];
  const room = new PictureRoom({
    assumedBytes: 30_000_000,
    memory: limitMemoryIn(options.storage ?? storageOf(), options.userAgent ?? silk),
    note: (note) => notes.push(note),
  });
  megabytes.forEach((each, index) => {
    const sn = index + 2;
    room.buffered({ sn, startS: sn * 6, endS: sn * 6 + 6, bytes: each * 1e6 }, loadedTo30);
  });
  return { room, notes };
}

describe("a chunk's bytes at each quality (D464 §1, §3)", () => {
  it("are what its playlist says of it, else the film's own average, else a made quality's peak and its encoder's 2 s", () => {
    const levels = ladder();
    const at = (level: number, sn: number): number =>
      chunkBytesAt(levels, level, { sn, duration: 6 });
    expect(at(3, 5)).toBe(15_000_000);
    expect(at(3, 4)).toBe(3_000_000);
    const unsaid: readonly RoomLevel[] = [
      ...levels.slice(0, 3),
      { ...levels[3]!, details: undefined },
    ];
    expect(chunkBytesAt(unsaid, 3, { sn: 5, duration: 6 })).toBe((5_117_192 * 6) / 8);
    expect(at(2, 5)).toBe((4_596_000 * 6 + 3_096_000 * 2) / 8);
    expect(at(7, 5)).toBe(0);
  });
});

describe("the quality a chunk loads at, beside the one held before it (D464 §3)", () => {
  it("is the one chosen where the pair fits in 90% of the buffer", () => {
    const { room } = roomHolding([3, 3, 3]);
    expect(pairLevel(hlsAt(), room, 3)).toEqual({
      level: 3,
      chunk: { sn: 5, chosenBytes: 15_000_000, bytes: 15_000_000, beforeBytes: 3_000_000 },
    });
  });

  it("steps the one chunk down to the highest that fits beside a heavy one held, and the next back up", () => {
    const { room } = roomHolding([3, 3, 15]);
    expect(pairLevel(hlsAt(), room, 3)).toEqual({
      level: 2,
      chunk: { sn: 5, chosenBytes: 15_000_000, bytes: 4_221_000, beforeBytes: 15_000_000 },
    });
    room.buffered({ sn: 5, startS: 30, endS: 36, bytes: 4_221_000 }, { start: 18, end: 36 });
    expect(pairLevel(hlsAt({ start: 18, end: 36 }), room, 3).level).toBe(3);
  });

  it("is the lowest where none fits, and the one chosen where nothing is loaded to place the next", () => {
    const { room } = roomHolding([3, 3, 26.9]);
    expect(pairLevel(hlsAt(), room, 3).level).toBe(0);
    expect(pairLevel(hlsAt(null), room, 3)).toEqual({ level: 3 });
  });
});

describe("how far ahead the picture loads for the next chunk to fit (D464 §4)", () => {
  it("is to the end of what is held from the earliest chunk the next fits beside with all after it", () => {
    // 15 MB beside 5 and 5 fits in 27 MB; with the third 5, not: it loads once 18 s plays.
    expect(roomAheadS(hlsAt(), roomHolding([5, 5, 5]).room, 3)).toBe(12);
    expect(roomAheadS(hlsAt(), roomHolding([5, 5, 13]).room, 3)).toBe(6);
  });

  it("is the one chunk before the next where even it does not fit beside it, and no bound where all fit", () => {
    expect(roomAheadS(hlsAt(), roomHolding([5, 5, 15]).room, 3)).toBe(6);
    expect(roomAheadS(hlsAt(), roomHolding([3, 3, 3]).room, 3)).toBeUndefined();
    expect(roomAheadS(hlsAt(), roomHolding([5, 5, 5]).room, 1)).toBeUndefined();
  });

  it("paces at the quality chosen before a step-down, so a chunk plays at it where waiting makes room", () => {
    const { room } = roomHolding([5, 5, 5]);
    const buffer = bufferRoomOf(hlsAt(), room, () => 3);
    expect([buffer.limitBytes(), buffer.aheadS(1)]).toEqual([30_000_000, 12]);
    expect(bufferRoomOf(hlsAt(), room, () => undefined).aheadS(1)).toBeUndefined();
  });

  it("forgets a chunk played past what is loaded around where it plays", () => {
    const { room } = roomHolding([5, 5, 5]);
    const span = { start: 24, end: 36 };
    room.buffered({ sn: 5, startS: 30, endS: 36, bytes: 2e6 }, span);
    expect(room.heldBefore(6, span).map((chunk) => chunk.sn)).toEqual([5, 4]);
  });
});

describe("the buffer's limit learnt on the device (D464 §2)", () => {
  it("is, at the first refusal, the most held at once, kept for the same user agent's next playback", () => {
    const storage = storageOf();
    const { room, notes } = roomHolding([5, 5, 5], { storage });
    expect(room.limitBytes()).toBe(30_000_000);
    room.refused();
    expect(room.limitBytes()).toBe(15_000_000);
    expect(notes).toEqual([
      "picture buffer refused: limit 30.0 MB lowered to 15.0 MB, the most held",
    ]);
    room.buffered({ sn: 5, startS: 30, endS: 36, bytes: 1e6 }, { start: 24, end: 36 });
    room.refused();
    expect([room.limitBytes(), notes.length]).toEqual([15_000_000, 1]);
    expect(roomHolding([1, 1, 1], { storage }).room.limitBytes()).toBe(15_000_000);
    const updated = roomHolding([1, 1, 1], { storage, userAgent: `${silk}.1` });
    expect(updated.room.limitBytes()).toBe(30_000_000);
  });

  it("stays the device's where it held no less than it, and where the browser keeps nothing", () => {
    const full = roomHolding([10, 10, 10]);
    full.room.refused();
    expect([full.room.limitBytes(), full.notes]).toEqual([30_000_000, []]);
    const refusing = roomHolding([5, 5, 5], { storage: storageOf(true) });
    refusing.room.refused();
    expect(refusing.room.limitBytes()).toBe(15_000_000);
    expect(roomHolding([1, 1, 1], { storage: storageOf(true) }).room.limitBytes()).toBe(30_000_000);
    const odd = storageOf();
    odd.kept.set("orpheum.pictureBufferLimit", "[not json");
    expect(limitMemoryIn(odd, silk).read()).toBeUndefined();
    odd.kept.set("orpheum.pictureBufferLimit", JSON.stringify({ userAgent: silk, bytes: "1" }));
    expect(limitMemoryIn(odd, silk).read()).toBeUndefined();
  });
});

describe("the step-down between hls.js and the quality chosen (D464 §3)", () => {
  it("notes each chunk's step once, keeps why, and knows hls.js's echo of it", () => {
    const { room } = roomHolding([3, 3, 15]);
    const steps: string[] = [];
    const step = new StepDown(hlsAt(), room, (each) => steps.push(each));
    expect([step.level(3), step.level(3), step.chosen()]).toEqual([2, 2, 3]);
    const why =
      "chunk 5 steps down from level 3: 15.0 MB beside 15.0 MB held is over the room's 27.0 MB;" +
      " level 2's 4.2 MB";
    expect(steps).toEqual([why]);
    expect(step.whyChose(2)).toEqual({ why, linkSamples: "" });
    expect([step.isEcho(2), step.isEcho(3)]).toEqual([true, false]);
    expect(step.level(1)).toBe(1);
    expect(step.isEcho(2)).toBe(false);
  });
});

/** A picture chunk of the grid as hls.js buffers it: sn, 6 s, its bytes loaded. */
function bufferedChunk(sn: number, bytes: number): Fragment {
  const frag = new Fragment(PlaylistLevelType.MAIN, "");
  frag.sn = sn;
  frag.level = 3;
  frag.start = sn * 6;
  frag.duration = 6;
  frag.stats.loaded = bytes;
  frag.stats.loading = { start: 0, first: 10, end: 1000 };
  return frag;
}

describe("the bitrate controller's step-down inside hls.js (D464 §3)", () => {
  function playing(): { readonly hls: Hls; readonly abr: ReturnType<typeof orpheumAbr> } {
    const room = new PictureRoom({
      assumedBytes: 30_000_000,
      memory: limitMemoryIn(storageOf(), silk),
      note: () => undefined,
    });
    const abr = orpheumAbr(
      {
        startAtBps: 0,
        monotonicMs: () => 0,
        isVoiceLive: () => false,
        fromCache: () => undefined,
      },
      room,
    );
    const hls = new Hls({ abrController: abr.controller });
    Object.defineProperty(hls, "levels", { value: ladder() });
    Object.defineProperty(hls, "minAutoLevel", { value: 0 });
    Object.defineProperty(hls, "maxAutoLevel", { value: 3 });
    Object.defineProperty(hls, "mainForwardBufferInfo", { value: loadedTo30, writable: true });
    for (const [sn, bytes] of [
      [2, 3e6],
      [3, 3e6],
      [4, 15e6],
    ] as const) {
      const frag = bufferedChunk(sn, bytes);
      hls.trigger(Events.FRAG_BUFFERED, {
        stats: frag.stats,
        frag,
        part: null,
        id: PlaylistLevelType.MAIN,
      });
    }
    return { hls, abr };
  }

  it("gives hls.js the chunk's step-down, and takes its echo for no quality forced", () => {
    const { hls, abr } = playing();
    hls.nextAutoLevel = 3; // hls.js forcing the top, which the choosing obeys
    expect(abr.choosing()?.forcedLevel()).toBe(3);
    expect(hls.nextAutoLevel).toBe(2);
    hls.nextAutoLevel = 2; // its stream controller's echo, at every tick
    expect(abr.choosing()?.forcedLevel()).toBe(3);
    expect([abr.chosenLevel(), abr.whyChose(2)?.why]).toEqual([
      3,
      expect.stringMatching(/^chunk 5 steps down from level 3: /u),
    ]);
    hls.destroy();
  });

  it("learns the buffer's limit from the picture's refusal alone", () => {
    const { hls, abr } = playing();
    hls.trigger(Events.ERROR, {
      type: Hls.ErrorTypes.MEDIA_ERROR,
      details: Hls.ErrorDetails.BUFFER_FULL_ERROR,
      fatal: false,
      sourceBufferName: "audio",
      error: new Error("full"),
    });
    hls.nextAutoLevel = 3;
    expect(hls.nextAutoLevel).toBe(2);
    hls.trigger(Events.ERROR, {
      type: Hls.ErrorTypes.MEDIA_ERROR,
      details: Hls.ErrorDetails.BUFFER_FULL_ERROR,
      fatal: false,
      sourceBufferName: "video",
      error: new Error("full"),
    });
    // 21 MB held is the limit now, 18.9 MB the room: beside chunk 4's 15 MB, only level 1 fits.
    expect(hls.nextAutoLevel).toBe(1);
    expect(abr.whyChose(1)?.why).toMatch(/over the room's 18\.9 MB; level 1's /u);
    hls.destroy();
  });
});
