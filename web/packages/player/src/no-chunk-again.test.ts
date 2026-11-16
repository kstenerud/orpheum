// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";
import Hls, { Fragment, LevelDetails, PlaylistLevelType } from "hls.js";

import { loadsNoChunkAgain } from "./no-chunk-again.ts";

/** A quality's playlist of ten 6 s chunks, as hls.js keeps it. */
function playlistOf(level: number): LevelDetails {
  const details = new LevelDetails("");
  const fragments = Array.from({ length: 10 }, (_, sn) =>
    Object.assign(new Fragment(PlaylistLevelType.MAIN, ""), {
      sn,
      level,
      start: sn * 6,
      duration: 6,
    }),
  );
  Reflect.set(details, "fragments", fragments);
  Object.assign(details, { startSN: 0, endSN: 9, live: false });
  return details;
}

/**
 * hls.js's own choice of the next chunk at level 1, the picture buffered to 35.986 s, 14 ms short of
 * chunk 5's end, as a made quality's was (file 1504, 2026-11-15); chunk 5 buffered whole at level 0
 * where isHeld says, by hls.js's fragment tracker.
 */
function nextAtLevel1(isHeld: boolean): { readonly sn: unknown; readonly level: unknown } | null {
  const hls = new Hls();
  const playlists = [playlistOf(0), playlistOf(1)];
  Object.defineProperty(hls, "levels", { value: playlists.map((details) => ({ details })) });
  const controller: object = Reflect.get(hls, "streamController");
  const tracker: object = Reflect.get(controller, "fragmentTracker");
  Reflect.set(tracker, "getState", (chunk: Fragment) =>
    isHeld && chunk.level === 0 && chunk.sn === 5 ? "OK" : "NOT_LOADED",
  );
  loadsNoChunkAgain(hls);
  const next: unknown = Reflect.apply(Reflect.get(controller, "getNextFragment"), controller, [
    35.986,
    playlists[1],
  ]);
  hls.destroy();
  return next === null
    ? null
    : { sn: Reflect.get(next as object, "sn"), level: Reflect.get(next as object, "level") };
}

describe("a switch of quality never loads again a chunk already buffered (D503 §4; hls.js 1.7.3)", () => {
  it("loads, on the quality switched to, the chunk after the one it holds at another, however short its picture ends", () => {
    expect(nextAtLevel1(true)).toEqual({ sn: 6, level: 1 });
  });

  it("loads the chunk the picture ends in where no quality holds it whole, as the browser evicted it", () => {
    expect(nextAtLevel1(false)).toEqual({ sn: 5, level: 1 });
  });
});
