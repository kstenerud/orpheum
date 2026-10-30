// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { afterFirstPiece } from "./abr-choice.ts";
import { FirstPieces, firstPiecesKept } from "./first-piece.ts";
import {
  chunkAt,
  choosingFrom,
  fireTvPlaying,
  scene,
  topChunkOnItsWay,
  useScene,
} from "./test-choosing.ts";

useScene({ beforeEach, afterEach, vi });

describe("a chunk's first arriving piece measures nothing (D382 §2)", () => {
  it("gives up a chunk that brought only its first piece as stuck, no sample, its next chunk what the link's estimate carries", () => {
    const chosen = fireTvPlaying();
    const chunk = topChunkOnItsWay(chosen, { pieceAtMs: 1500, afterBytes: 0 });
    expect(chosen.givenUp).toEqual([chunk]);
    expect(chosen.fedToHls).toEqual([]);
    expect(scene.linkNotes.filter((note) => note.startsWith("sample "))).toHaveLength(2);
    expect(scene.linkNotes).toContain(
      "gave up a chunk of 4, stuck: 131072 B in 2000 ms, only its first piece; carried WebRTC connection 2 for /c/4.m4s",
    );
    // Measured at 680 kb/s, it took the Fire TV to its lowest quality for the 30 s hold.
    expect(chosen.choosing.nextLevel()).toBeGreaterThanOrEqual(3);
  });

  it("measures a loaded chunk by its bytes after its first piece, over the time after it", () => {
    const { choosing } = choosingFrom(40_000_000);
    choosing.firstLevel();
    const chunk = chunkAt(3);
    choosing.loading(chunk);
    // Its head at 50 ms, its first piece of 128 KiB at 800 ms, then a megabyte in a second.
    scene.firstPieces.set(chunk.url, { atMs: 800, bytes: 131_072 });
    chunk.stats.loaded = 131_072 + 1_000_000;
    chunk.stats.loading = { start: 0, first: 50, end: 1800 };
    choosing.loaded(chunk);
    expect(choosing.measures(chunk)).toBe(true);
    expect(scene.samplesNoted.at(-1)).toBe("samples 3 1000000B 1000ms fb50ms");
  });

  it("measures nothing of a chunk too few of whose bytes came after its first piece", () => {
    const { choosing } = choosingFrom(40_000_000);
    choosing.firstLevel();
    const chunk = chunkAt(3);
    choosing.loading(chunk);
    scene.firstPieces.set(chunk.url, { atMs: 400, bytes: 131_072 });
    chunk.stats.loaded = 131_072 + 100_000;
    chunk.stats.loading = { start: 0, first: 50, end: 1400 };
    choosing.loaded(chunk);
    expect(choosing.measures(chunk)).toBe(false);
    expect(scene.samplesNoted).toEqual([]);
  });

  it("measures from the first byte where the first piece is not known, or is an earlier load's", () => {
    const arrived = { bytes: 500_000, fromMs: 1000, toMs: 2000 };
    expect(afterFirstPiece(arrived, undefined)).toEqual({ bytes: 500_000, ms: 1000 });
    expect(afterFirstPiece(arrived, { atMs: 900, bytes: 131_072 })).toEqual({
      bytes: 500_000,
      ms: 1000,
    });
    expect(afterFirstPiece(arrived, { atMs: 1200, bytes: 131_072 })).toEqual({
      bytes: 500_000 - 131_072,
      ms: 800,
    });
  });
});

describe("the first piece of each load (FirstPieces)", () => {
  /** A request whose progress the test tells. */
  class Request extends EventTarget {
    progress(loaded: number): void {
      this.dispatchEvent(Object.assign(new Event("progress"), { loaded }));
    }
  }

  it("is its first progress with bytes, its time and bytes then, a new load of the same URL forgetting the last", () => {
    let nowMs = 100;
    const pieces = new FirstPieces(() => nowMs);
    const request = new Request();
    pieces.follow(request, "/c/4.m4s");
    request.progress(0);
    expect(pieces.of("/c/4.m4s")).toBeUndefined();
    nowMs = 487;
    request.progress(131_072);
    nowMs = 900;
    request.progress(262_144);
    expect(pieces.of("/c/4.m4s")).toEqual({ atMs: 487, bytes: 131_072 });
    pieces.follow(new Request(), "/c/4.m4s");
    expect(pieces.of("/c/4.m4s")).toBeUndefined();
  });

  it("keeps the latest loads' alone", () => {
    const pieces = new FirstPieces(() => 0);
    for (let index = 0; index <= firstPiecesKept; index += 1) {
      const request = new Request();
      pieces.follow(request, `/c/${String(index)}.m4s`);
      request.progress(1);
    }
    expect(pieces.of("/c/0.m4s")).toBeUndefined();
    expect(pieces.of(`/c/${String(firstPiecesKept)}.m4s`)).toEqual({ atMs: 0, bytes: 1 });
  });
});
